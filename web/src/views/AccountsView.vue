<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/api'
import { useAppStore } from '@/store/app'
import { useIsMobile } from '@/composables/useIsMobile'
import KindTag from '@/components/KindTag.vue'
import StatusTag from '@/components/StatusTag.vue'
import { KIND_OPTIONS, formatTime, fromNow } from '@/utils/format'

const store = useAppStore()
/* 手机端：表格转卡片，对话框接近全屏 */
const isMobile = useIsMobile()
const dialogWidth = computed(() => (isMobile.value ? '94%' : '460px'))

const loading = ref(false)
const rows = ref([])
const rowBusy = ref({}) // id -> 'login' | 'sync'

/* ---------- 新增 / 编辑 ---------- */
const dialogVisible = ref(false)
const dialogMode = ref('create') // create | edit
const saving = ref(false)
const formRef = ref(null)
const form = reactive({
  id: null,
  kind: 'pica',
  username: '',
  password: '',
  label: '',
  note: ''
})

const rules = {
  kind: [{ required: true, message: '请选择源', trigger: 'change' }],
  username: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [
    {
      validator(rule, value, cb) {
        if (dialogMode.value === 'create' && !value) cb(new Error('请输入密码'))
        else cb()
      },
      trigger: 'blur'
    }
  ]
}

const kindOptions = KIND_OPTIONS

async function load() {
  loading.value = true
  try {
    const data = await api.listAccounts()
    rows.value = Array.isArray(data) ? data : (data?.items ?? [])
    store.accounts = rows.value
    store.accountsLoaded = true
  } catch (e) {
    rows.value = []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  dialogMode.value = 'create'
  Object.assign(form, { id: null, kind: 'pica', username: '', password: '', label: '', note: '' })
  dialogVisible.value = true
}

function openEdit(row) {
  dialogMode.value = 'edit'
  Object.assign(form, {
    id: row.id,
    kind: row.kind,
    username: row.username,
    password: '',
    label: row.label || '',
    note: row.note || ''
  })
  dialogVisible.value = true
}

async function submitForm() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    saving.value = true
    try {
      if (dialogMode.value === 'create') {
        await api.createAccount({
          kind: form.kind,
          username: form.username,
          password: form.password,
          label: form.label
        })
        ElMessage.success('账号已添加并登录')
      } else {
        const patch = { label: form.label, note: form.note }
        if (form.username && form.username !== originalUsername(form.id)) patch.username = form.username
        if (form.password) patch.password = form.password
        await api.updateAccount(form.id, patch)
        ElMessage.success('账号已更新')
      }
      dialogVisible.value = false
      await load()
    } catch (e) {
      /* api.js 已提示 */
    } finally {
      saving.value = false
    }
  })
}

function originalUsername(id) {
  return rows.value.find((r) => r.id === id)?.username
}

async function testLogin(row) {
  rowBusy.value = { ...rowBusy.value, [row.id]: 'login' }
  try {
    await api.loginAccount(row.id)
    ElMessage.success(`「${row.label || row.username}」登录正常`)
    await load()
  } catch (e) {
    /* api.js 已提示 */
    await load()
  } finally {
    const m = { ...rowBusy.value }
    delete m[row.id]
    rowBusy.value = m
  }
}

async function syncNow(row) {
  rowBusy.value = { ...rowBusy.value, [row.id]: 'sync' }
  try {
    const res = await api.syncAccount(row.id)
    ElMessage.success(`同步完成：入队 ${res?.enqueued ?? 0} 个，跳过 ${res?.skipped ?? 0} 个`)
    await load()
    store.loadActiveJobs().catch(() => {})
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    const m = { ...rowBusy.value }
    delete m[row.id]
    rowBusy.value = m
  }
}

async function removeAccount(row) {
  try {
    await ElMessageBox.confirm(
      `确定删除账号「${row.label || row.username}」吗？删除后需重新登录，已下载的文件不受影响。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch (_) {
    return
  }
  try {
    await api.deleteAccount(row.id)
    ElMessage.success('账号已删除')
    await load()
  } catch (e) {
    /* api.js 已提示 */
  }
}

function statusOf(row) {
  return row.status || 'error'
}

onMounted(load)
</script>

<template>
  <div class="ms-panel">
    <div class="ms-panel-title">
      <span>
        账号列表
        <span class="ms-sub">· 共 {{ rows.length }} 个</span>
      </span>
      <div class="head-actions">
        <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        <el-button size="small" type="primary" @click="openCreate">新增账号</el-button>
      </div>
    </div>

    <!-- 宽屏：表格 -->
    <el-table
      v-if="!isMobile"
      v-loading="loading"
      :data="rows"
      size="small"
      empty-text="还没有账号，点右上角「新增账号」添加"
      row-key="id"
    >
      <el-table-column label="源" width="80">
        <template #default="{ row }">
          <KindTag :kind="row.kind" />
        </template>
      </el-table-column>
      <el-table-column prop="username" label="账号" min-width="150" show-overflow-tooltip />
      <el-table-column label="昵称" min-width="120">
        <template #default="{ row }">
          <span>{{ row.nickname || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="等级" width="70">
        <template #default="{ row }">{{ row.level ?? '—' }}</template>
      </el-table-column>
      <el-table-column label="收藏数" width="110">
        <template #default="{ row }">
          {{ row.favoritesCount ?? 0 }}
          <span v-if="row.favoritesMax" class="ms-dim">/{{ row.favoritesMax }}</span>
        </template>
      </el-table-column>
      <el-table-column label="标签" min-width="110">
        <template #default="{ row }">
          <el-tag v-if="row.label" size="small" effect="plain">{{ row.label }}</el-tag>
          <span v-else class="ms-dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="96">
        <template #default="{ row }">
          <StatusTag domain="account" :value="statusOf(row)" />
        </template>
      </el-table-column>
      <el-table-column label="最近登录" width="130">
        <template #default="{ row }">
          <span :title="formatTime(row.lastLoginAt, true)">{{ fromNow(row.lastLoginAt) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="最近同步" width="130">
        <template #default="{ row }">
          <span :title="formatTime(row.lastSyncAt, true)">{{ fromNow(row.lastSyncAt) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button size="small" :loading="rowBusy[row.id] === 'login'" @click="testLogin(row)">测试登录</el-button>
          <el-button size="small" type="primary" plain :loading="rowBusy[row.id] === 'sync'" @click="syncNow(row)">
            立即同步
          </el-button>
          <el-dropdown trigger="click" class="more-dd">
            <el-button size="small" text>更多<el-icon><ArrowDown /></el-icon></el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="openEdit(row)">编辑</el-dropdown-item>
                <el-dropdown-item divided @click="removeAccount(row)">
                  <span class="danger-text">删除</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>

    <!-- 手机端：卡片列表（大触摸区，信息分层） -->
    <div v-else v-loading="loading" class="ms-mlist">
      <div v-for="row in rows" :key="row.id" class="ms-mcard">
        <div class="ms-mcard-head">
          <div class="acct-title">
            <KindTag :kind="row.kind" />
            <span class="ms-mcard-title">{{ row.label || row.username }}</span>
          </div>
          <StatusTag domain="account" :value="statusOf(row)" />
        </div>

        <div class="ms-mcard-rows">
          <div class="ms-mrow">
            <span class="k">账号</span>
            <span class="v">{{ row.username }}</span>
          </div>
          <div class="ms-mrow">
            <span class="k">昵称</span>
            <span class="v">{{ row.nickname || '—' }}</span>
          </div>
          <div class="ms-mrow">
            <span class="k">等级 / 收藏</span>
            <span class="v">
              {{ row.level ?? '—' }} 级 ·
              {{ row.favoritesCount ?? 0 }}<span v-if="row.favoritesMax" class="ms-dim">/{{ row.favoritesMax }}</span>
            </span>
          </div>
          <div class="ms-mrow">
            <span class="k">最近登录</span>
            <span class="v">{{ fromNow(row.lastLoginAt) }}</span>
          </div>
          <div class="ms-mrow">
            <span class="k">最近同步</span>
            <span class="v">{{ fromNow(row.lastSyncAt) }}</span>
          </div>
          <div v-if="row.error" class="ms-mrow">
            <span class="k">错误</span>
            <span class="v danger-text">{{ row.error }}</span>
          </div>
          <div v-if="row.note" class="ms-mrow">
            <span class="k">备注</span>
            <span class="v ms-dim">{{ row.note }}</span>
          </div>
        </div>

        <div class="ms-mcard-actions">
          <el-button size="small" :loading="rowBusy[row.id] === 'login'" @click="testLogin(row)">测试登录</el-button>
          <el-button
            size="small"
            type="primary"
            plain
            :loading="rowBusy[row.id] === 'sync'"
            @click="syncNow(row)"
          >
            立即同步
          </el-button>
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" plain @click="removeAccount(row)">删除</el-button>
        </div>
      </div>

      <div v-if="!rows.length && !loading" class="ms-empty">
        还没有账号，点上方「新增账号」添加
      </div>
    </div>

    <div v-if="!isMobile && !rows.length && !loading" class="foot-tip">
      <el-button type="primary" plain @click="openCreate">新增第一个账号</el-button>
    </div>
  </div>

  <el-dialog
    v-model="dialogVisible"
    :title="dialogMode === 'create' ? '新增账号' : '编辑账号'"
    :width="dialogWidth"
    append-to-body
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      :label-width="isMobile ? 'auto' : '76px'"
      :label-position="isMobile ? 'top' : 'right'"
    >
      <el-form-item label="源" prop="kind">
        <el-radio-group v-model="form.kind" :disabled="dialogMode === 'edit'">
          <el-radio-button v-for="k in kindOptions" :key="k.value" :value="k.value">
            {{ k.label }}
          </el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="账号" prop="username">
        <el-input v-model="form.username" placeholder="登录用户名 / 邮箱" clearable />
      </el-form-item>
      <el-form-item label="密码" prop="password">
        <el-input
          v-model="form.password"
          type="password"
          show-password
          :placeholder="dialogMode === 'edit' ? '留空表示不修改密码' : '登录密码'"
        />
      </el-form-item>
      <el-form-item label="备注标签">
        <el-input v-model="form.label" maxlength="40" placeholder="例如：哔咔主号" clearable />
      </el-form-item>
      <el-form-item label="备注">
        <el-input v-model="form.note" type="textarea" :rows="2" maxlength="200" show-word-limit />
      </el-form-item>
      <el-alert
        v-if="dialogMode === 'create'"
        type="info"
        :closable="false"
        show-icon
        title="添加后会立即尝试登录，密码错误会直接报错。"
      />
    </el-form>
    <template #footer>
      <el-button @click="dialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submitForm">
        {{ dialogMode === 'create' ? '添加并登录' : '保存' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.head-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.more-dd {
  margin-left: 8px;
  vertical-align: middle;
}

.danger-text {
  color: var(--el-color-danger);
}

.foot-tip {
  text-align: center;
  padding: 16px 0 4px;
}

.acct-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}
</style>
