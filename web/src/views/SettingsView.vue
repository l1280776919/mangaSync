<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api, { authErrorMessage } from '@/api'
import { useAppStore } from '@/store/app'
import { auth } from '@/store/auth'
import { changePassword, logout } from '@/composables/useAuth'
import { useIsMobile } from '@/composables/useIsMobile'
import StatCard from '@/components/StatCard.vue'
import { QUALITY_OPTIONS, formatBytes, formatTime } from '@/utils/format'

const store = useAppStore()
/* 手机端：表单标签改为顶部对齐，控件撑满屏宽 */
const isMobile = useIsMobile()
const formLabelWidth = computed(() => (isMobile.value ? 'auto' : '132px'))
const formLabelPosition = computed(() => (isMobile.value ? 'top' : 'right'))

const loading = ref(false)
const saving = ref(false)
const formRef = ref(null)
const health = ref(null)

/** 契约里 settings 的全部字段 */
const form = reactive({
  downloadRoot: '',
  picaDir: '',
  jmDir: '',
  picaProxy: '',
  jmProxy: '',
  concurrency: 2,
  imageWorkers: 8,
  quality: 'original',
  schedule: { enabled: false, time: '04:30' },
  serverPort: 8787
})

const rules = {
  downloadRoot: [{ required: true, message: '请填写下载根目录', trigger: 'blur' }],
  concurrency: [
    { required: true, message: '请填写并发数', trigger: 'blur' },
    {
      validator(rule, value, cb) {
        const n = Number(value)
        if (!Number.isInteger(n) || n < 1 || n > 8) cb(new Error('并发数取值范围 1 ～ 8'))
        else cb()
      },
      trigger: 'blur'
    }
  ],
  imageWorkers: [
    {
      validator(rule, value, cb) {
        const n = Number(value)
        if (!Number.isInteger(n) || n < 1 || n > 64) cb(new Error('图片并发数建议 1 ～ 64'))
        else cb()
      },
      trigger: 'blur'
    }
  ],
  scheduleTime: [
    {
      validator(rule, value, cb) {
        if (!form.schedule.enabled) return cb()
        if (!/^\d{2}:\d{2}$/.test(String(form.schedule.time || ''))) cb(new Error('时间格式应为 HH:mm'))
        else cb()
      },
      trigger: 'change'
    }
  ]
}

/* ---------------- 账号安全（登录态 + 改密 + 登出） ---------------- */

const account = computed(() => auth.user || null)
const accountName = computed(() => auth.user?.username || '—')
const lastLoginAt = computed(() => formatTime(auth.user?.lastLoginAt))
const sessionExpiresAt = computed(() => formatTime(auth.user?.sessionExpiresAt))

const pwFormRef = ref(null)
const pwSaving = ref(false)
const loggingOut = ref(false)

const pwForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const pwRules = {
  oldPassword: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '新密码至少 6 位', trigger: 'blur' },
    {
      validator(rule, value, cb) {
        if (value && value === pwForm.oldPassword) cb(new Error('新密码不能与原密码相同'))
        else cb()
      },
      trigger: 'blur'
    }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      validator(rule, value, cb) {
        if (value !== pwForm.newPassword) cb(new Error('两次输入的密码不一致'))
        else cb()
      },
      trigger: ['blur', 'change']
    }
  ]
}

async function loadAccount() {
  try {
    // 让「最近登录时间 / 会话有效期」保持最新
    const me = await api.me()
    auth.user = { ...(auth.user || {}), ...me }
    auth.loaded = true
  } catch (e) {
    /* api.js 已统一处理 401 */
  }
}

async function submitPassword() {
  if (pwSaving.value || !pwFormRef.value) return
  await pwFormRef.value.validate(async (valid) => {
    if (!valid) return
    pwSaving.value = true
    try {
      await changePassword({
        oldPassword: pwForm.oldPassword,
        newPassword: pwForm.newPassword,
        silent: true
      })
      ElMessage.success('密码已修改，请牢记新密码')
      pwForm.oldPassword = pwForm.newPassword = pwForm.confirmPassword = ''
      pwFormRef.value.clearValidate()
    } catch (e) {
      ElMessage.error(authErrorMessage(e))
    } finally {
      pwSaving.value = false
    }
  })
}

async function onLogout() {
  if (loggingOut.value) return
  loggingOut.value = true
  try {
    await logout()
  } finally {
    loggingOut.value = false
  }
}

async function load() {
  loading.value = true
  try {
    const s = await api.getSettings()
    Object.assign(form, {
      downloadRoot: s?.downloadRoot ?? '',
      picaDir: s?.picaDir ?? '',
      jmDir: s?.jmDir ?? '',
      picaProxy: s?.picaProxy ?? '',
      jmProxy: s?.jmProxy ?? '',
      concurrency: s?.concurrency ?? 2,
      imageWorkers: s?.imageWorkers ?? 8,
      quality: s?.quality ?? 'original',
      schedule: {
        enabled: !!s?.schedule?.enabled,
        time: s?.schedule?.time || '04:30'
      },
      serverPort: s?.serverPort ?? 8787
    })
    store.settings = s
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    loading.value = false
  }
}

async function loadHealth() {
  try {
    health.value = await api.health()
  } catch (e) {
    health.value = null
  }
}

async function save() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    saving.value = true
    try {
      const payload = {
        downloadRoot: form.downloadRoot,
        picaDir: form.picaDir,
        jmDir: form.jmDir,
        picaProxy: form.picaProxy,
        jmProxy: form.jmProxy,
        concurrency: Number(form.concurrency),
        imageWorkers: Number(form.imageWorkers),
        quality: form.quality,
        schedule: { enabled: !!form.schedule.enabled, time: form.schedule.time },
        serverPort: Number(form.serverPort)
      }
      const merged = await api.saveSettings(payload)
      store.settings = merged
      Object.assign(form, {
        ...merged,
        schedule: { ...(merged?.schedule || form.schedule) }
      })
      ElMessage.success('设置已保存')
    } catch (e) {
      /* api.js 已提示 */
    } finally {
      saving.value = false
    }
  })
}

function reset() {
  load()
  ElMessage.info('已重新从后端读取设置')
}

onMounted(async () => {
  await Promise.allSettled([load(), loadHealth(), loadAccount(), store.loadStats()])
})
</script>

<template>
  <!-- 账号安全：登录态 + 内嵌改密 + 登出（旧的 authUser/authPass「访问控制」已随 Basic 认证一并移除） -->
  <div class="ms-panel">
    <div class="ms-panel-title">
      <span>
        账号安全
        <span class="ms-sub">· /api/auth/me · /api/auth/password · /api/auth/logout</span>
      </span>
      <div class="head-actions">
        <el-tag size="small" effect="light" type="info">{{ accountName }}</el-tag>
        <el-tag v-if="account?.isAdmin" size="small" effect="light" type="warning">管理员</el-tag>
        <el-button size="small" :loading="loggingOut" @click="onLogout">退出登录</el-button>
      </div>
    </div>

    <el-alert
      v-if="account?.mustChangePassword"
      title="当前仍是初始密码，请先修改密码后再使用其它功能。"
      type="warning"
      :closable="false"
      show-icon
      class="pw-warn"
    />

    <div class="account-grid">
      <div class="account-item">
        <div class="label ms-dim">当前账号</div>
        <div class="value">{{ accountName }}</div>
      </div>
      <div class="account-item">
        <div class="label ms-dim">最近登录时间</div>
        <div class="value">{{ lastLoginAt }}</div>
      </div>
      <div class="account-item">
        <div class="label ms-dim">会话有效期至</div>
        <div class="value">{{ sessionExpiresAt }}</div>
      </div>
    </div>

    <el-divider content-position="left">修改密码</el-divider>

    <el-form
      ref="pwFormRef"
      :model="pwForm"
      :rules="pwRules"
      :label-width="formLabelWidth"
      :label-position="formLabelPosition"
      class="setting-form"
      @submit.prevent="submitPassword"
    >
      <el-form-item label="原密码" prop="oldPassword">
        <el-input
          v-model="pwForm.oldPassword"
          type="password"
          show-password
          autocomplete="current-password"
          placeholder="请输入当前密码"
          @keyup.enter="submitPassword"
        />
      </el-form-item>

      <el-form-item label="新密码" prop="newPassword">
        <el-input
          v-model="pwForm.newPassword"
          type="password"
          show-password
          autocomplete="new-password"
          placeholder="至少 6 位，且不能与原密码相同"
          @keyup.enter="submitPassword"
        />
        <div class="tip">改密后当前会话保留，其它已登录设备会被踢下线。</div>
      </el-form-item>

      <el-form-item label="确认新密码" prop="confirmPassword">
        <el-input
          v-model="pwForm.confirmPassword"
          type="password"
          show-password
          autocomplete="new-password"
          placeholder="请再次输入新密码"
          @keyup.enter="submitPassword"
        />
      </el-form-item>

      <el-form-item>
        <el-button type="primary" :loading="pwSaving" @click="submitPassword">修改密码</el-button>
        <el-button @click="onLogout">退出登录</el-button>
      </el-form-item>
    </el-form>
  </div>

  <div class="ms-panel">
    <div class="ms-panel-title">
      <span>
        系统设置
        <span class="ms-sub">· GET / PUT /api/settings</span>
      </span>
      <div class="head-actions">
        <el-tag v-if="health" type="success" size="small" effect="light">
          后端正常 · v{{ health.version }} · 运行 {{ Math.floor((health.uptimeSec || 0) / 60) }} 分
        </el-tag>
        <el-tag v-else type="info" size="small" effect="light">后端状态未知</el-tag>
        <el-button size="small" :loading="loading" @click="load">重新读取</el-button>
      </div>
    </div>

    <el-form
      ref="formRef"
      v-loading="loading"
      :model="form"
      :rules="rules"
      :label-width="formLabelWidth"
      :label-position="formLabelPosition"
      class="setting-form"
    >
      <el-divider content-position="left">下载路径</el-divider>

      <el-form-item label="下载根目录" prop="downloadRoot">
        <el-input v-model="form.downloadRoot" placeholder="/data/comics">
          <template #prepend><el-icon><FolderOpened /></el-icon></template>
        </el-input>
        <div class="tip">必须是已存在或可创建的目录。</div>
      </el-form-item>

      <el-form-item label="哔咔子目录">
        <el-input v-model="form.picaDir" placeholder="PicaComic" />
        <div class="tip">相对下载根目录，最终路径：根目录 / 子目录 / 作品名</div>
      </el-form-item>

      <el-form-item label="禁漫图片编码">
        <div class="tip" style="margin: 0">
          禁漫为纯 Go 实现（无 Python 依赖）：带乱序的图会还原后<strong>无损</strong>保存，体积约为站点有损版 2~3 倍。
        </div>
      </el-form-item>

      <el-form-item label="禁漫子目录">
        <el-input v-model="form.jmDir" placeholder="18Comic" />
      </el-form-item>

      <el-divider content-position="left">网络与并发</el-divider>

      <el-form-item label="哔咔代理">
        <el-input v-model="form.picaProxy" placeholder="http://127.0.0.1:7890（留空表示直连）" clearable />
      </el-form-item>

      <el-form-item label="禁漫代理">
        <el-input v-model="form.jmProxy" placeholder="http://127.0.0.1:7890（留空表示直连）" clearable />
      </el-form-item>

      <el-form-item label="并发任务数" prop="concurrency">
        <el-input-number v-model="form.concurrency" :min="1" :max="8" :step="1" />
        <span class="tip-inline">同时下载的作品数（1 ～ 8）</span>
      </el-form-item>

      <el-form-item label="图片并发数" prop="imageWorkers">
        <el-input-number v-model="form.imageWorkers" :min="1" :max="64" :step="1" />
        <span class="tip-inline">单本内并行抓图的线程数</span>
      </el-form-item>

      <el-form-item label="图片质量">
        <el-radio-group v-model="form.quality">
          <el-radio-button v-for="q in QUALITY_OPTIONS" :key="q.value" :value="q.value">
            {{ q.label }}
          </el-radio-button>
        </el-radio-group>
        <div class="tip">只有哔咔（pica）支持质量切换。</div>
      </el-form-item>

      <el-divider content-position="left">定时同步</el-divider>

      <el-form-item label="启用定时同步">
        <el-switch v-model="form.schedule.enabled" active-text="开启" inactive-text="关闭" />
        <div class="tip">开启后会按设定时间自动同步所有账号的收藏。</div>
      </el-form-item>

      <el-form-item label="同步时间" prop="scheduleTime">
        <el-time-picker
          v-model="form.schedule.time"
          format="HH:mm"
          value-format="HH:mm"
          placeholder="04:30"
          :disabled="!form.schedule.enabled"
          :style="{ width: isMobile ? '100%' : '160px' }"
        />
      </el-form-item>

      <el-divider content-position="left">服务</el-divider>

      <el-form-item label="服务端口">
        <el-input-number v-model="form.serverPort" :min="1" :max="65535" :step="1" />
        <span class="tip-inline">默认 8787（需重启后端生效）</span>
      </el-form-item>

      <el-form-item>
        <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
        <el-button :disabled="loading" @click="reset">放弃修改</el-button>
      </el-form-item>
    </el-form>
  </div>

  <div class="ms-panel">
    <div class="ms-panel-title">磁盘与缓存</div>
    <div class="ms-stat-grid">
      <StatCard
        label="漫画库占用"
        :value="formatBytes(store.stats?.library?.bytes)"
        icon="FolderOpened"
        :sub="`${store.stats?.library?.comics ?? 0} 部作品`"
      />
      <StatCard
        label="剩余空间"
        :value="formatBytes(store.stats?.disk?.freeBytes)"
        icon="Coin"
        :sub="store.stats?.disk?.totalBytes ? `总容量 ${formatBytes(store.stats.disk.totalBytes)}` : '—'"
      />
      <StatCard
        label="封面缓存"
        value="后端自管"
        icon="Picture"
        sub="<数据目录>/cache/covers/"
      />
    </div>
  </div>
</template>

<style scoped>
.setting-form {
  max-width: 720px;
}

.tip {
  font-size: 12px;
  color: var(--ms-text-dim);
  line-height: 1.5;
  width: 100%;
}

.tip-inline {
  font-size: 12px;
  color: var(--ms-text-dim);
  margin-left: 10px;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.pw-warn {
  margin-bottom: 12px;
}

.account-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 10px;
}

.account-item {
  background: var(--ms-bg-soft);
  border: 1px solid var(--ms-border);
  border-radius: var(--ms-radius);
  padding: 10px 12px;
}

.account-item .label {
  font-size: 12px;
  margin-bottom: 4px;
}

.account-item .value {
  font-size: 14px;
  font-weight: 600;
  word-break: break-all;
}

:deep(.el-form-item__content) {
  flex-wrap: wrap;
}

/* 手机端：标签置顶、数字/时间控件撑满，避免 132px 标签挤掉输入框 */
@media (max-width: 768px) {
  .setting-form {
    max-width: 100%;
  }

  .setting-form :deep(.el-input-number),
  .setting-form :deep(.el-time-picker) {
    width: 100% !important;
  }

  .tip-inline {
    display: block;
    margin-left: 0;
    margin-top: 4px;
  }

  .account-grid {
    grid-template-columns: 1fr;
  }
}
</style>
