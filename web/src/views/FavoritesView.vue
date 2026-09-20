<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/api'
import { useAppStore } from '@/store/app'
import { useDownloadActions } from '@/composables/useDownloadActions'
import { useIsMobile } from '@/composables/useIsMobile'
import { useLatestRequest } from '@/composables/useLatestRequest'
import { useViewActive } from '@/composables/useViewActive'
import ComicCard from '@/components/ComicCard.vue'
import CoverImage from '@/components/CoverImage.vue'
import ComicDetailDialog from '@/components/ComicDetailDialog.vue'
import KindTag from '@/components/KindTag.vue'
import PageBar from '@/components/PageBar.vue'
import { downloadStateOf, formatBytes, formatTime } from '@/utils/format'
import { readerPath } from '@/utils/reader'

const store = useAppStore()
/* 手机端：只保留卡片视图 + 底部批量操作栏 */
const isMobile = useIsMobile()
const router = useRouter()

/** 在线阅读：已下载章节走本地文件秒开，未下载的回源并在服务端还原乱序 */
function read(row, order = 1) {
  // 行数据缺 kind 时回退到当前账号的源（原来写成未定义的 kind.value，会抛 ReferenceError）
  const path = readerPath(row, order, currentAccount.value?.kind)
  if (path) router.push(path)
}

const accountId = ref(null)
const keyword = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const items = ref([])
const loading = ref(false)
const viewMode = ref('card') // card | table
const selected = ref([])      // 选中的 comicId 列表
const removingId = ref(null)

const accounts = computed(() => store.accounts)
const currentAccount = computed(() => accounts.value.find((a) => a.id === accountId.value) || null)

const {
  pickerVisible,
  pickerComic,
  detailVisible,
  detailComic,
  busy,
  isBusy,
  downloadWhole,
  downloadMany,
  openPicker,
  openDetail,
  submitPicker
} = useDownloadActions(() => accountId.value)

/** 列表请求：只认最后一次（切账号 / 翻页 / 连点搜索不会被旧响应覆盖） */
const req = useLatestRequest()

async function load() {
  if (!accountId.value) {
    items.value = []
    total.value = 0
    return
  }
  const { my, signal } = req.begin()
  loading.value = true
  try {
    const res = await api.favorites(accountId.value, {
      keyword: keyword.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value,
      signal
    })
    if (!req.isCurrent(my)) return
    items.value = res?.items || []
    total.value = Number(res?.total) || 0
    selected.value = []
  } catch (e) {
    if (e?.name === 'AbortError' || !req.isCurrent(my)) return
    items.value = []
    total.value = 0
  } finally {
    req.end()
    if (req.isCurrent(my)) loading.value = false
  }
}

/**
 * 刷账号列表并保证「首次进入只发一次 /favorites」：
 * 赋值 accountId 会触发下面的 watch（唯一入口），未变更时才手动 load 一次。
 */
async function refreshAccounts() {
  await store.loadAccounts(true).catch(() => {})
  if (accountId.value == null && accounts.value.length) accountId.value = accounts.value[0].id
  else if (accountId.value != null) load()
}

function search() {
  page.value = 1
  load()
}

function onPageChange() {
  load()
}

function toggleSelect(item) {
  const key = String(item.comicId)
  const idx = selected.value.indexOf(key)
  if (idx >= 0) selected.value.splice(idx, 1)
  else selected.value.push(key)
}

const allSelected = computed(
  () => items.value.length > 0 && items.value.every((i) => selected.value.includes(String(i.comicId)))
)

function toggleSelectAll() {
  selected.value = allSelected.value ? [] : items.value.map((i) => String(i.comicId))
}

const selectedItems = computed(() =>
  items.value.filter((i) => selected.value.includes(String(i.comicId)))
)

async function batchDownload() {
  if (!selectedItems.value.length) return
  await downloadMany(selectedItems.value)
  store.loadActiveJobs().catch(() => {})
}

async function download(item) {
  await downloadWhole(item)
  store.loadActiveJobs().catch(() => {})
}

async function removeFavorite(item) {
  try {
    await ElMessageBox.confirm(
      `确定从账号「${currentAccount.value?.label || currentAccount.value?.username}」的收藏中移除《${item.title}》吗？已下载的文件不会被删除。`,
      '取消收藏',
      { type: 'warning', confirmButtonText: '取消收藏', cancelButtonText: '再想想' }
    )
  } catch (_) {
    return
  }
  removingId.value = String(item.comicId)
  try {
    await api.removeFavorite(accountId.value, item.comicId)
    ElMessage.success('已取消收藏')
    await load()
    store.loadAccounts(true).catch(() => {})
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    removingId.value = null
  }
}

function stateTag(item) {
  return downloadStateOf(item)
}

watch(accountId, () => {
  page.value = 1
  keyword.value = ''
  load()
})

/* keep-alive 缓存后 onMounted 只跑一次：切回收藏页重新拉账号与列表 */
const active = useViewActive({ onEnter: refreshAccounts })

/** 顶栏「刷新」 */
watch(
  () => store.refreshTick,
  () => {
    if (active.value) refreshAccounts()
  }
)
</script>

<template>
  <div class="ms-panel" :class="{ 'ms-has-mbar': isMobile }">
    <div class="ms-panel-title">
      <span>
        我的收藏
        <span class="ms-sub">· 共 {{ total }} 部</span>
      </span>
      <div v-if="!isMobile" class="view-switch">
        <el-radio-group v-model="viewMode" size="small">
          <el-radio-button value="card">卡片</el-radio-button>
          <el-radio-button value="table">表格</el-radio-button>
        </el-radio-group>
      </div>
    </div>

    <div class="ms-toolbar">
      <el-select
        v-model="accountId"
        placeholder="选择账号"
        class="acct-select"
        filterable
        :disabled="!accounts.length"
      >
        <el-option
          v-for="a in accounts"
          :key="a.id"
          :label="`${a.kind === 'pica' ? '哔咔' : '禁漫'} · ${a.label || a.username}（${a.favoritesCount ?? 0}）`"
          :value="a.id"
        />
      </el-select>

      <el-input
        v-model="keyword"
        placeholder="搜索收藏标题 / 作者"
        class="grow"
        clearable
        :disabled="!accountId"
        @keyup.enter="search"
        @clear="search"
      >
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>

      <el-button type="primary" :disabled="!accountId" :loading="loading" @click="search">搜索</el-button>
      <el-button :disabled="!accountId" :loading="loading" @click="load">刷新</el-button>

      <template v-if="!isMobile">
        <el-divider direction="vertical" />

        <el-button :disabled="!items.length" @click="toggleSelectAll">
          {{ allSelected ? '取消全选' : '全选本页' }}
        </el-button>
        <el-button
          type="primary"
          plain
          :disabled="!selectedItems.length"
          :loading="busy"
          @click="batchDownload"
        >
          批量下载（{{ selectedItems.length }}）
        </el-button>
      </template>
    </div>

    <el-alert
      v-if="!accounts.length"
      :type="store.accountsError ? 'error' : 'warning'"
      :closable="false"
      show-icon
      :title="
        store.accountsError
          ? `账号列表加载失败：${store.accountsError}`
          : '还没有可用账号，请先到「账号」页添加。'
      "
      class="mb10"
    >
      <el-button
        v-if="store.accountsError"
        size="small"
        text
        type="primary"
        @click="refreshAccounts"
      >
        重试
      </el-button>
    </el-alert>
    <el-alert
      v-else-if="!loading && !items.length"
      type="info"
      :closable="false"
      show-icon
      :title="keyword ? '没有匹配的收藏' : '这个账号暂时没有收藏'"
      class="mb10"
    />

    <!-- 卡片视图（手机端固定卡片；封面 2 列自适应） -->
    <div v-if="isMobile || viewMode === 'card'" v-loading="loading" class="ms-comic-grid">
      <ComicCard
        v-for="item in items"
        :key="`${item.kind}-${item.comicId}`"
        :item="item"
        selectable
        readable
        :selected="selected.includes(String(item.comicId))"
        favorited
        :busy="removingId === String(item.comicId) || isBusy(item)"
        @toggle="toggleSelect"
        @read="read"
        @download="download"
        @queue="openPicker"
        @uncollect="removeFavorite"
        @open="openDetail"
      />
    </div>

    <!-- 表格视图 -->
    <el-table
      v-else
      v-loading="loading"
      :data="items"
      size="small"
      empty-text="没有数据"
      row-key="comicId"
    >
      <el-table-column width="46">
        <template #default="{ row }">
          <el-checkbox
            :model-value="selected.includes(String(row.comicId))"
            @change="toggleSelect(row)"
          />
        </template>
      </el-table-column>
      <el-table-column label="封面" width="66">
        <template #default="{ row }">
          <div class="mini-cover">
            <CoverImage :kind="row.kind" :comic-id="row.comicId" :src="row.cover" :title="row.title" />
          </div>
        </template>
      </el-table-column>
      <el-table-column label="标题" min-width="200">
        <template #default="{ row }">
          <div class="cell-title" @click="openDetail(row)">{{ row.title }}</div>
          <div class="cell-sub ms-mono">{{ row.comicId }}</div>
        </template>
      </el-table-column>
      <el-table-column label="作者" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.author || '—' }}</template>
      </el-table-column>
      <el-table-column label="分类/标签" min-width="150">
        <template #default="{ row }">
          <el-tag
            v-for="c in (row.categories || []).slice(0, 2)"
            :key="c"
            size="small"
            effect="plain"
            class="mr4"
          >
            {{ c }}
          </el-tag>
          <el-tag
            v-for="t in (row.tags || []).slice(0, 2)"
            :key="t"
            size="small"
            type="info"
            effect="plain"
            class="mr4"
          >
            {{ t }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="章节" width="86">
        <template #default="{ row }">{{ row.downloadedChapters ?? 0 }}/{{ row.chapters ?? 0 }}</template>
      </el-table-column>
      <el-table-column label="下载状态" width="112">
        <template #default="{ row }">
          <el-tag :type="stateTag(row).type" size="small" effect="plain">{{ stateTag(row).label }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="本地路径" min-width="200">
        <template #default="{ row }">
          <span class="ms-mono path" :title="row.localPath">{{ row.localPath || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="90">
        <template #default="{ row }">{{ formatBytes(row.bytes) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="success" plain @click="read(row)">阅读</el-button>
          <el-button size="small" type="primary" :loading="isBusy(row)" @click="download(row)">下载</el-button>
          <el-button size="small" @click="openPicker(row)">加入队列</el-button>
          <el-button size="small" type="danger" plain :loading="removingId === String(row.comicId)" @click="removeFavorite(row)">
            取消收藏
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <PageBar
      v-model:page="page"
      v-model:page-size="pageSize"
      :total="total"
      :disabled="loading"
      @change="onPageChange"
    />

    <div v-if="currentAccount" class="foot-note ms-dim">
      当前账号：{{ currentAccount.nickname || currentAccount.username }} ·
      最近登录 {{ formatTime(currentAccount.lastLoginAt) }} ·
      最近同步 {{ currentAccount.lastSyncAt ? formatTime(currentAccount.lastSyncAt) : '从未' }}
    </div>

    <!-- 手机端底部固定操作栏：选择与批量下载 -->
    <div v-if="isMobile" class="ms-mbar">
      <span class="grow">
        已选 <b>{{ selectedItems.length }}</b> / {{ items.length }} 部
      </span>
      <el-button size="small" :disabled="!items.length" @click="toggleSelectAll">
        {{ allSelected ? '取消全选' : '全选本页' }}
      </el-button>
      <el-button
        size="small"
        type="primary"
        :disabled="!selectedItems.length"
        :loading="busy"
        @click="batchDownload"
      >
        批量下载
      </el-button>
    </div>
  </div>

  <ComicDetailDialog
    v-model="pickerVisible"
    :comic="pickerComic"
    @submit="submitPicker"
  />
  <ComicDetailDialog v-model="detailVisible" :comic="detailComic" :pick-chapters="false" />
</template>

<style scoped>
.acct-select {
  width: 260px;
}

.view-switch {
  display: flex;
  align-items: center;
}

.mb10 {
  margin-bottom: 10px;
}

.mini-cover {
  width: 40px;
  height: 54px;
  border-radius: 4px;
  overflow: hidden;
  border: 1px solid var(--ms-border);
}

.cell-title {
  font-weight: 600;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-title:hover {
  color: var(--ms-accent);
}

.cell-sub {
  font-size: 10px;
  color: var(--ms-text-dim);
}

.path {
  font-size: 11px;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.mr4 {
  margin-right: 4px;
}

.foot-note {
  font-size: 12px;
  padding-top: 10px;
}

@media (max-width: 640px) {
  .acct-select {
    width: 100%;
  }
}
</style>
