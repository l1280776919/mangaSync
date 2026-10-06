<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '@/api'
import { openReaderWindow } from '@/utils/reader'
import { useAppStore } from '@/store/app'
import { useIsMobile } from '@/composables/useIsMobile'
import { useViewActive } from '@/composables/useViewActive'
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

/* 统计口径统一走 store（bootstrap / 设置页 / 本页共用一份，带 15s 复用窗口） */
const stats = computed(() => store.stats)
const loadingStats = ref(false)
const recentDone = ref([])
const reading = ref([])
const syncRuns = ref([])
const loadingRecent = ref(false)
const logVisible = ref(false)
const logJob = ref(null)
let recentTimer = null

/** 页面是否可见（keep-alive 缓存后 onUnmounted 不再触发，靠这个开关停掉轮询） */
const active = useViewActive({
  onEnter: () => {
    startTimer()
    reload()
  },
  onLeave: () => {
    stopTimer()
    clearTickTimer()
  }
})

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

async function loadStats(force = false) {
  loadingStats.value = true
  try {
    await store.loadStats(force ? 0 : undefined)
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

/** 失败任务重试：原来没有提示也不刷新列表，点了像没反应 */
async function onRetry(job) {
  try {
    await api.retryDownload(job.id)
    ElMessage.success('已重新入队')
    store.loadActiveJobs().catch(() => {})
    reload()
  } catch (e) {
    /* api.js 已提示 */
  }
}

function openLogs(job) {
  logJob.value = job
  logVisible.value = true
}

async function loadActivity() {
  const [r, s] = await Promise.allSettled([api.recentReading(), api.syncHistory()])
  if (r.status === 'fulfilled') reading.value = r.value || []
  if (s.status === 'fulfilled') syncRuns.value = s.value || []
}
async function reload(force = false) {
  await Promise.allSettled([loadStats(force), loadRecent(), store.loadActiveJobs(), loadActivity()])
}

let tickTimer = null

watch(
  () => store.jobTick,
  () => {
    if (!active.value) return // 已经离开概览页，别再刷
    // 任务状态变化后刷新统计与最近完成列表（做轻量节流）
    if (tickTimer) return
    tickTimer = setTimeout(() => {
      tickTimer = null
      if (!active.value) return
      loadStats(true)
      loadRecent()
      loadActivity()
    }, 1200)
  }
)

/** 顶栏「刷新」：重载本页数据 */
watch(
  () => store.refreshTick,
  () => {
    if (active.value) reload(true)
  }
)

function startTimer() {
  if (recentTimer) return
  recentTimer = setInterval(() => loadStats(), 20000)
}

function stopTimer() {
  if (recentTimer) {
    clearInterval(recentTimer)
    recentTimer = null
  }
}

function clearTickTimer() {
  if (tickTimer) {
    clearTimeout(tickTimer)
    tickTimer = null
  }
}

onUnmounted(() => {
  stopTimer()
  clearTickTimer()
})

import { ArrowRight } from '@element-plus/icons-vue'

const shortcuts = [
  { label: '账号管理', desc: '源账号与状态', icon: 'User', path: '/accounts', color: '#3b82f6' },
  { label: '我的收藏', desc: '收藏列表与批量下载', icon: 'Star', path: '/favorites', color: '#f59e0b' },
  { label: '搜索漫画', desc: '跨平台检索书源', icon: 'Search', path: '/search', color: '#8b5cf6' },
  { label: '下载任务', desc: '队列监控与进度', icon: 'Download', path: '/downloads', color: '#10b981' },
  { label: '漫画库', desc: '海报墙与在线阅读', icon: 'Collection', path: '/library', color: '#ec4899' },
  { label: '系统设置', desc: '存储与核心参数', icon: 'Setting', path: '/settings', color: '#64748b' }
]
</script>

<template>
  <div>
    <el-alert v-if="stats?.downloadPause" :title="stats.downloadPause" type="warning" :closable="false" />
    <div class="ms-panel">
      <div class="ms-panel-title">继续阅读</div>
      <div v-if="!reading.length" class="ms-empty">开始阅读后，这里会保存你的阅读位置。</div>
      <div v-for="book in reading" :key="book.kind + book.comicId" class="reading-row">
        <img :src="api.coverUrl(book.kind, book.comicId)" alt="" loading="lazy" />
        <div class="reading-info"><strong>{{ book.title || book.comicId }}</strong><div class="ms-dim">第 {{ book.order }} 章 · 第 {{ book.page }} 页</div></div>
        <el-button type="primary" @click="openReaderWindow(book)">继续阅读</el-button>
        <el-button text @click="openReaderWindow(book, 1, '', false)">从头阅读</el-button>
      </div>
    </div>
    <div class="ms-panel">
      <div class="ms-panel-title">
        <span>
          总览

        </span>
        <div class="title-actions">
          <el-tag size="small" effect="light">{{ store.polling ? '自动刷新中' : '刷新已暂停' }}</el-tag>
          <el-button size="small" :loading="loadingStats" @click="reload(true)">刷新数据</el-button>

        </div>
      </div>

      <div v-loading="loadingStats" class="ms-stat-grid">
        <StatCard label="账号数" :value="stats?.accounts ?? '—'" icon="User" color="#3b82f6" sub="已绑定源账号" clickable @click="router.push('/accounts')" />
        <StatCard label="收藏数" :value="stats?.favorites ?? '—'" icon="Star" color="#f59e0b" sub="所有账号收藏合计" clickable @click="router.push('/favorites')" />
        <StatCard label="下载中" :value="dl.running ?? '—'" icon="Loading" color="#2563eb" :sub="`排队 ${dl.queued ?? 0} 个`" clickable @click="router.push('/downloads')" />
        <StatCard label="已完成" :value="dl.done ?? '—'" icon="CircleCheck" color="#10b981" sub="累计完成任务" clickable @click="router.push('/downloads')" />
        <StatCard label="失败" :value="dl.failed ?? '—'" icon="CircleClose" color="#ef4444" sub="可重试" clickable @click="router.push('/downloads')" />
        <StatCard
          label="漫画库大小"
          :value="formatBytes(lib.bytes)"
          icon="FolderOpened"
          color="#8b5cf6"
          :sub="`${lib.comics ?? 0} 部 / ${lib.images ?? 0} 图`"
          clickable
          @click="router.push('/library')"
        />
        <StatCard
          label="磁盘剩余"
          :value="formatBytes(disk.freeBytes)"
          icon="Coin"
          color="#64748b"
          :sub="disk.totalBytes ? `共 ${formatBytes(disk.totalBytes)}` : '—'"
        />
        <StatCard label="实时速度" :value="formatSpeed(totalSpeed)" icon="Download" color="#06b6d4" :sub="`${activeJobs.length} 个活动任务`" />
      </div>
    </div>

    <div class="ms-panel">
      <div class="ms-panel-title">
        <span>
          正在下载

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
        :removable="false"
        @cancel="onCancel"
        @logs="openLogs"
        @retry="onRetry"
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
      <div class="shortcut-grid">
        <div
          v-for="s in shortcuts"
          :key="s.path"
          class="shortcut-card"
          @click="router.push(s.path)"
        >
          <div class="shortcut-icon-wrap" :style="{ backgroundColor: `${s.color}14`, color: s.color }">
            <el-icon :size="20"><component :is="s.icon" /></el-icon>
          </div>
          <div class="shortcut-info">
            <div class="shortcut-title">{{ s.label }}</div>
            <div class="shortcut-desc">{{ s.desc }}</div>
          </div>
          <el-icon class="shortcut-arrow"><ArrowRight /></el-icon>
        </div>
      </div>
    </div>

    <div class="ms-panel">
      <div class="ms-panel-title"><span>同步历史</span><span class="ms-dim">失败的定时同步将在 15 分钟后重试</span></div>
      <div v-if="!syncRuns.length" class="ms-empty">还没有同步记录</div>
      <div v-for="run in syncRuns.slice(0, 10)" :key="run.id" class="sync-row">
        <strong>{{ store.accountMap[run.accountId]?.label || store.accountMap[run.accountId]?.username || ('账号 ' + run.accountId) }}</strong>
        <el-tag :type="run.status === 'success' ? 'success' : run.status === 'running' ? 'info' : 'warning'">{{ ({success:'成功',partial:'部分成功',failed:'失败',running:'进行中'})[run.status] }}</el-tag>
        <span>入队 {{ run.enqueued }} · 跳过 {{ run.skipped }} · {{ fromNow(run.startedAt) }}</span>
        <div v-if="run.error" class="sync-error">{{ run.error }}</div>
      </div>
    </div>
    <LogDialog v-model="logVisible" :job="logJob" />
  </div>
</template>

<style scoped>
.reading-row { display: flex; gap: 12px; align-items: center; padding: 12px 0; flex-wrap: wrap; border-bottom: 1px solid var(--ms-border); }
.reading-row img { width: 44px; height: 60px; object-fit: cover; border-radius: 4px; }
.reading-info { flex: 1; min-width: 120px; }
.sync-row { display: flex; flex-wrap: wrap; gap: 8px; padding: 12px 0; border-bottom: 1px solid var(--ms-border); }
.sync-error { width: 100%; color: var(--el-color-warning); overflow-wrap: anywhere; }

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

.shortcut-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
}

.shortcut-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  background: var(--ms-bg-soft);
  border: 1px solid var(--ms-border);
  border-radius: var(--ms-radius);
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.shortcut-card:hover {
  background: #ffffff;
  border-color: var(--el-color-primary-light-7);
  box-shadow: 0 4px 14px rgba(15, 23, 42, 0.06);
  transform: translateY(-2px);
}

.shortcut-icon-wrap {
  width: 36px;
  height: 36px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.shortcut-info {
  flex: 1;
  min-width: 0;
}

.shortcut-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ms-text);
  line-height: 1.25;
}

.shortcut-desc {
  font-size: 11px;
  color: var(--ms-text-dim);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.shortcut-arrow {
  font-size: 14px;
  color: var(--ms-border-strong);
  transition: transform 0.15s ease, color 0.15s ease;
  flex-shrink: 0;
}

.shortcut-card:hover .shortcut-arrow {
  color: var(--el-color-primary);
  transform: translateX(2px);
}

</style>
