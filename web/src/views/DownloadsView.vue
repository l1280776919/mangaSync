<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/api'
import { useAppStore } from '@/store/app'
import { useIsMobile } from '@/composables/useIsMobile'
import { useLatestRequest } from '@/composables/useLatestRequest'
import { useViewActive } from '@/composables/useViewActive'
import JobProgress from '@/components/JobProgress.vue'
import LogDialog from '@/components/LogDialog.vue'
import KindTag from '@/components/KindTag.vue'
import PageBar from '@/components/PageBar.vue'
import StatusTag from '@/components/StatusTag.vue'
import {
  JOB_STATUS,
  formatBytes,
  formatElapsed,
  formatSpeed,
  jobProgress
} from '@/utils/format'

const store = useAppStore()
/* 手机端：表格视图退化为卡片列表（宽表格在手机上无法阅读） */
const isMobile = useIsMobile()

const status = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const items = ref([])
const loading = ref(false)
const viewMode = ref('card')
const logVisible = ref(false)
const logJob = ref(null)
const autoRefresh = ref(true)

const statusOptions = [
  { value: '', label: '全部状态' },
  ...Object.entries(JOB_STATUS).map(([value, v]) => ({ value, label: v.label }))
]

/** 进行中的任务：优先用 SSE 实时缓冲里的数据（比接口轮询更实时） */
const liveItems = computed(() => {
  const map = new Map()
  for (const j of items.value) map.set(j.id, j)
  // SSE 推来的任务对象覆盖同 id 的记录（实时进度更准）
  for (const j of store.jobList) {
    if (map.has(j.id)) map.set(j.id, { ...map.get(j.id), ...j })
  }
  return [...map.values()].sort((a, b) => (b.id || 0) - (a.id || 0))
})

/** 列表请求：只认最后一次（连点刷新 / 改状态筛选不会被旧响应覆盖） */
const req = useLatestRequest()

async function load() {
  const { my, signal } = req.begin()
  loading.value = true
  try {
    const res = await api.listDownloads({
      status: status.value || undefined,
      page: page.value,
      pageSize: pageSize.value,
      signal
    })
    if (!req.isCurrent(my)) return
    items.value = res?.items || []
    total.value = Number(res?.total) || Number(items.value.length) || 0
  } catch (e) {
    if (e?.name === 'AbortError' || !req.isCurrent(my)) return
    items.value = []
    total.value = 0
  } finally {
    req.end()
    if (req.isCurrent(my)) loading.value = false
  }
}

async function onCancel(job) {
  try {
    await api.cancelDownload(job.id)
    ElMessage.success('已请求取消')
  } catch (e) {
    /* api.js 已提示 */
  }
  load()
}

async function onRetry(job) {
  try {
    await api.retryDownload(job.id)
    ElMessage.success('已重新入队')
  } catch (e) {
    /* api.js 已提示 */
  }
  load()
}

async function onRemove(job) {
  try {
    await ElMessageBox.confirm(
      `确定删除任务记录 #${job.id}《${job.title || job.comicId}》吗？仅删除记录，不影响已下载文件。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除记录', cancelButtonText: '取消' }
    )
  } catch (_) {
    return
  }
  try {
    await api.deleteDownload(job.id)
    ElMessage.success('记录已删除')
    const m = { ...store.jobs }
    delete m[job.id]
    store.jobs = m
    load()
  } catch (e) {
    /* api.js 已提示 */
  }
}

function openLogs(job) {
  logJob.value = job
  logVisible.value = true
}

function onStatusChange() {
  page.value = 1
  load()
}

function onPageChange() {
  load()
}

/* keep-alive 缓存后 onMounted 只跑一次：切回任务页重新拉一次列表 */
const active = useViewActive({
  onEnter: () => {
    load()
    store.loadActiveJobs().catch(() => {})
  },
  onLeave: () => {
    if (refreshTimer) {
      clearTimeout(refreshTimer)
      refreshTimer = null
    }
  }
})

// SSE 推送到达时，如果当前看的是实时状态（或全部），做节流刷新列表补齐总数
let refreshTimer = null
watch(
  () => store.jobTick,
  () => {
    // 页面被 keep-alive 缓存时也要停：人不在这里就不该每 2.5s 拉一次 /downloads
    if (!active.value || !autoRefresh.value) return
    if (refreshTimer) return
    refreshTimer = setTimeout(() => {
      refreshTimer = null
      if (!active.value) return
      load()
    }, 2500)
  }
)

/** 顶栏「刷新」 */
watch(
  () => store.refreshTick,
  () => {
    if (active.value) load()
  }
)

const activeCount = computed(
  () => liveItems.value.filter((j) => j.status === 'running' || j.status === 'queued').length
)
const totalSpeed = computed(() =>
  liveItems.value.reduce((s, j) => s + (j.status === 'running' ? Number(j.speedBps) || 0 : 0), 0)
)

</script>

<template>
  <div class="ms-panel">
    <div class="ms-panel-title">
      <span>
        下载任务
        <span class="ms-sub">· 共 {{ total }} 条，进行中 {{ activeCount }} 个</span>
      </span>
      <div class="head-actions">
        <el-tag v-if="store.sseConnected" type="success" size="small" effect="light">
          实时推送（EventSource /api/events）
        </el-tag>
        <el-tag v-else type="warning" size="small" effect="light">SSE 断开，已降级为轮询</el-tag>
        <el-tooltip content="SSE 掉线时用轮询兜底，重连成功后自动恢复推送">
          <el-switch v-model="autoRefresh" size="small" active-text="自动刷新" />
        </el-tooltip>
        <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        <el-button size="small" text type="primary" @click="store.reconnectEvents()">重连推送</el-button>
      </div>
    </div>

    <div class="ms-toolbar">
      <el-select v-model="status" class="status-select" placeholder="任务状态" @change="onStatusChange">
        <el-option v-for="o in statusOptions" :key="o.value" :label="o.label" :value="o.value" />
      </el-select>
      <span class="ms-dim speed-note">当前总速度：{{ formatSpeed(totalSpeed) }}</span>
      <div v-if="!isMobile" class="spacer" />
      <el-radio-group v-if="!isMobile" v-model="viewMode" size="small">
        <el-radio-button value="card">卡片</el-radio-button>
        <el-radio-button value="table">表格</el-radio-button>
      </el-radio-group>
    </div>

    <!-- 卡片视图（手机端固定用卡片） -->
    <div v-if="isMobile || viewMode === 'card'" v-loading="loading" class="job-list">
      <JobProgress
        v-for="job in liveItems"
        :key="job.id"
        :job="job"
        @cancel="onCancel"
        @retry="onRetry"
        @remove="onRemove"
        @logs="openLogs"
      />
      <div v-if="!liveItems.length && !loading" class="ms-empty">
        <el-icon :size="28"><Tickets /></el-icon>
        <div>暂无下载任务</div>
      </div>
    </div>

    <!-- 表格视图 -->
    <el-table
      v-else
      v-loading="loading"
      :data="liveItems"
      size="small"
      empty-text="暂无下载任务"
      row-key="id"
    >
      <el-table-column prop="id" label="#" width="58" />
      <el-table-column label="源" width="76">
        <template #default="{ row }"><KindTag :kind="row.kind" /></template>
      </el-table-column>
      <el-table-column label="标题" min-width="180">
        <template #default="{ row }">
          <div class="cell-title">{{ row.title || row.comicId }}</div>
          <div class="cell-sub ms-mono">{{ row.comicId }}</div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="96">
        <template #default="{ row }"><StatusTag domain="job" :value="row.status" /></template>
      </el-table-column>
      <el-table-column label="进度" width="150">
        <template #default="{ row }">
          <el-progress
            :percentage="jobProgress(row)"
            :stroke-width="6"
            :status="row.status === 'failed' ? 'exception' : row.status === 'done' ? 'success' : undefined"
          />
        </template>
      </el-table-column>
      <el-table-column label="章节进度" width="90">
        <template #default="{ row }">{{ row.chaptersDone ?? 0 }}/{{ row.chaptersTotal ?? 0 }}</template>
      </el-table-column>
      <el-table-column label="图片进度" width="110">
        <template #default="{ row }">{{ row.imagesDone ?? 0 }}/{{ row.imagesTotal ?? 0 }}</template>
      </el-table-column>
      <el-table-column label="速度" width="96">
        <template #default="{ row }">{{ row.status === 'running' ? formatSpeed(row.speedBps) : '—' }}</template>
      </el-table-column>
      <el-table-column label="当前章节" min-width="120" show-overflow-tooltip>
        <template #default="{ row }">{{ row.currentChapter || '—' }}</template>
      </el-table-column>
      <el-table-column label="大小" width="92">
        <template #default="{ row }">{{ formatBytes(row.bytes) }}</template>
      </el-table-column>
      <el-table-column label="耗时" width="110">
        <template #default="{ row }">
          <span v-if="!row.startedAt">—</span>
          <span v-else>{{ formatElapsed(row.startedAt, row.finishedAt) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="错误信息" min-width="160">
        <template #default="{ row }">
          <span :class="row.error ? 'err' : 'ms-dim'">{{ row.error || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="row.status === 'running' || row.status === 'queued'"
            size="small"
            type="warning"
            plain
            @click="onCancel(row)"
          >
            取消
          </el-button>
          <el-button
            v-if="row.status === 'failed' || row.status === 'canceled'"
            size="small"
            type="primary"
            plain
            @click="onRetry(row)"
          >
            重试
          </el-button>
          <el-button size="small" @click="openLogs(row)">日志</el-button>
          <el-button size="small" type="danger" plain @click="onRemove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <PageBar v-model:page="page" v-model:page-size="pageSize" :total="total" :disabled="loading" @change="onPageChange" />
  </div>

  <LogDialog v-model="logVisible" :job="logJob" />
</template>

<style scoped>
.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.status-select {
  width: 160px;
}

.spacer {
  flex: 1;
}

.speed-note {
  font-size: 12px;
}

.job-list :deep(.ms-job-row) {
  padding: 12px 0;
}

.cell-title {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-sub {
  font-size: 10px;
  color: var(--ms-text-dim);
}

.err {
  color: var(--el-color-danger);
}

@media (max-width: 640px) {
  .status-select {
    width: 100%;
  }
}
</style>
