<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '@/api'
import { useAppStore } from '@/store/app'
import { useIsMobile } from '@/composables/useIsMobile'
import StatCard from '@/components/StatCard.vue'
import JobProgress from '@/components/JobProgress.vue'
import LogDialog from '@/components/LogDialog.vue'
import StatusTag from '@/components/StatusTag.vue'
import KindTag from '@/components/KindTag.vue'
import { formatBytes, formatSpeed, fromNow } from '@/utils/format'

const router = useRouter()
const store = useAppStore()
/* 手机端：最近完成列表由表格换成卡片 */
const isMobile = useIsMobile()

const stats = ref(null)
const loadingStats = ref(false)
const recentDone = ref([])
const loadingRecent = ref(false)
const logVisible = ref(false)
const logJob = ref(null)
let recentTimer = null

const activeJobs = computed(() =>
  store.jobList
    .filter((j) => j.status === 'running' || j.status === 'queued')
    .sort((a, b) => {
      if (a.status === b.status) return (b.id || 0) - (a.id || 0)
      return a.status === 'running' ? -1 : 1
    })
)

const dl = computed(() => stats.value?.downloads || {})
const lib = computed(() => stats.value?.library || {})
const disk = computed(() => stats.value?.disk || {})

const totalSpeed = computed(() => activeJobs.value.reduce((s, j) => s + (Number(j.speedBps) || 0), 0))

async function loadStats() {
  loadingStats.value = true
  try {
    stats.value = await api.stats()
    store.stats = stats.value
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    loadingStats.value = false
  }
}

async function loadRecent() {
  loadingRecent.value = true
  try {
    const res = await api.listDownloads({ status: 'done', page: 1, pageSize: 5 })
    recentDone.value = res?.items || []
  } catch (e) {
    /* 后端未起时静默 */
  } finally {
    loadingRecent.value = false
  }
}

async function onCancel(job) {
  await api.cancelDownload(job.id).then(() => store.loadActiveJobs()).catch(() => {})
}

function openLogs(job) {
  logJob.value = job
  logVisible.value = true
}

async function reload() {
  await Promise.allSettled([loadStats(), loadRecent(), store.loadActiveJobs()])
}

let tickTimer = null

watch(
  () => store.jobTick,
  () => {
    // 任务状态变化后刷新统计与最近完成列表（做轻量节流）
    if (tickTimer) return
    tickTimer = setTimeout(() => {
      tickTimer = null
      loadStats()
      loadRecent()
    }, 1200)
  }
)

onMounted(async () => {
  await reload()
  recentTimer = setInterval(() => {
    loadStats()
  }, 20000)
})

onUnmounted(() => {
  if (recentTimer) clearInterval(recentTimer)
  if (tickTimer) clearTimeout(tickTimer)
})

const shortcuts = [
  { label: '账号管理', icon: 'User', path: '/accounts' },
  { label: '我的收藏', icon: 'Star', path: '/favorites' },
  { label: '搜索漫画', icon: 'Search', path: '/search' },
  { label: '下载任务', icon: 'Download', path: '/downloads' },
  { label: '漫画库', icon: 'Collection', path: '/library' },
  { label: '系统设置', icon: 'Setting', path: '/settings' }
]
</script>

<template>
  <div>
    <div class="ms-panel">
      <div class="ms-panel-title">
        <span>
          总览
          <span class="ms-sub">· 后端 /api/stats</span>
        </span>
        <div class="title-actions">
          <el-tag v-if="store.sseConnected" type="success" size="small" effect="light">实时推送中</el-tag>
          <el-tag v-else type="warning" size="small" effect="light">SSE 断开（已降级轮询）</el-tag>
          <el-button size="small" :loading="loadingStats" @click="reload">刷新数据</el-button>
          <el-button size="small" text type="primary" @click="store.reconnectEvents()">重连推送</el-button>
        </div>
      </div>

      <div v-loading="loadingStats" class="ms-stat-grid">
        <StatCard label="账号数" :value="stats?.accounts ?? '—'" icon="User" sub="已绑定源账号" clickable @click="router.push('/accounts')" />
        <StatCard label="收藏数" :value="stats?.favorites ?? '—'" icon="Star" sub="所有账号收藏合计" clickable @click="router.push('/favorites')" />
        <StatCard label="下载中" :value="dl.running ?? '—'" icon="Loading" color="#2f6fed" :sub="`排队 ${dl.queued ?? 0} 个`" clickable @click="router.push('/downloads')" />
        <StatCard label="已完成" :value="dl.done ?? '—'" icon="CircleCheck" color="#16a34a" sub="累计完成任务" clickable @click="router.push('/downloads')" />
        <StatCard label="失败" :value="dl.failed ?? '—'" icon="CircleClose" color="#dc2626" sub="可重试" clickable @click="router.push('/downloads')" />
        <StatCard
          label="漫画库大小"
          :value="formatBytes(lib.bytes)"
          icon="FolderOpened"
          :sub="`${lib.comics ?? 0} 部 / ${lib.images ?? 0} 图`"
          clickable
          @click="router.push('/library')"
        />
        <StatCard
          label="磁盘剩余"
          :value="formatBytes(disk.freeBytes)"
          icon="Coin"
          :sub="disk.totalBytes ? `共 ${formatBytes(disk.totalBytes)}` : '—'"
        />
        <StatCard label="实时速度" :value="formatSpeed(totalSpeed)" icon="Download" :sub="`${activeJobs.length} 个活动任务`" />
      </div>
    </div>

    <div class="ms-panel">
      <div class="ms-panel-title">
        <span>
          正在下载
          <span class="ms-sub">· 来自 /api/events 实时推送</span>
        </span>
        <span class="ms-dim">{{ activeJobs.length }} 个任务</span>
      </div>

      <div v-if="!activeJobs.length" class="ms-empty">
        <el-icon :size="28"><Select /></el-icon>
        <div>当前没有进行中的下载任务</div>
        <el-button size="small" type="primary" plain @click="router.push('/favorites')">去收藏页挑一本</el-button>
      </div>

      <JobProgress
        v-for="job in activeJobs"
        :key="job.id"
        :job="job"
        compact
        @cancel="onCancel"
        @logs="openLogs"
        @retry="(j) => api.retryDownload(j.id).catch(() => {})"
        @remove="() => {}"
      />
    </div>

    <div class="ms-panel">
      <div class="ms-panel-title">
        <span>
          最近完成
          <span class="ms-sub">· 最近 5 条</span>
        </span>
        <el-button size="small" text type="primary" @click="router.push('/downloads')">查看全部任务</el-button>
      </div>

      <!-- 手机端：卡片列表 -->
      <div v-if="isMobile" v-loading="loadingRecent" class="ms-mlist">
        <div v-for="row in recentDone" :key="row.id" class="ms-mcard">
          <div class="ms-mcard-head">
            <div class="recent-title">
              <KindTag :kind="row.kind" />
              <span class="ms-mcard-title">{{ row.title || row.comicId }}</span>
            </div>
            <StatusTag domain="job" :value="row.status" />
          </div>
          <div class="ms-mcard-rows">
            <div class="ms-mrow">
              <span class="k">章节 / 图片</span>
              <span class="v">
                {{ row.chaptersDone ?? 0 }}/{{ row.chaptersTotal ?? 0 }} 章 ·
                {{ row.imagesDone ?? 0 }}/{{ row.imagesTotal ?? 0 }} 图
              </span>
            </div>
            <div class="ms-mrow">
              <span class="k">大小</span>
              <span class="v">{{ formatBytes(row.bytes) }}</span>
            </div>
            <div class="ms-mrow">
              <span class="k">完成时间</span>
              <span class="v">{{ row.finishedAt ? fromNow(row.finishedAt) : '—' }}</span>
            </div>
          </div>
          <div class="ms-mcard-actions">
            <el-button size="small" type="primary" plain @click="openLogs(row)">查看日志</el-button>
          </div>
        </div>
        <div v-if="!recentDone.length && !loadingRecent" class="ms-empty">还没有完成的任务</div>
      </div>

      <el-table
        v-else
        v-loading="loadingRecent"
        :data="recentDone"
        size="small"
        empty-text="还没有完成的任务"
      >
        <el-table-column label="源" width="76">
          <template #default="{ row }">
            <KindTag :kind="row.kind" />
          </template>
        </el-table-column>
        <el-table-column prop="title" label="标题" min-width="180" show-overflow-tooltip />
        <el-table-column label="章节" width="90">
          <template #default="{ row }">{{ row.chaptersDone ?? 0 }}/{{ row.chaptersTotal ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="图片" width="100">
          <template #default="{ row }">{{ row.imagesDone ?? 0 }}/{{ row.imagesTotal ?? 0 }}</template>
        </el-table-column>
        <el-table-column label="大小" width="90">
          <template #default="{ row }">{{ formatBytes(row.bytes) }}</template>
        </el-table-column>
        <el-table-column label="完成时间" width="120">
          <template #default="{ row }">{{ row.finishedAt ? fromNow(row.finishedAt) : '—' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="86">
          <template #default="{ row }">
            <StatusTag domain="job" :value="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text type="primary" @click="openLogs(row)">日志</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div class="ms-panel">
      <div class="ms-panel-title">快捷入口</div>
      <div class="shortcuts">
        <el-button v-for="s in shortcuts" :key="s.path" @click="router.push(s.path)">
          <el-icon><component :is="s.icon" /></el-icon>
          <span>{{ s.label }}</span>
        </el-button>
      </div>
    </div>

    <LogDialog v-model="logVisible" :job="logJob" />
  </div>
</template>

<style scoped>
.title-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.shortcuts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.shortcuts :deep(.el-button) {
  margin-left: 0;
}

.recent-title {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

/* 手机端：快捷入口两列排布，触摸区更大 */
@media (max-width: 768px) {
  .shortcuts :deep(.el-button) {
    flex: 1 1 44%;
  }
}
</style>
