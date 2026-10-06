<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Collection,
  Tickets,
  Picture,
  FolderOpened,
  Search,
  Reading,
  Document,
  Coin,
  Clock,
  MoreFilled,
  Delete,
  InfoFilled
} from '@element-plus/icons-vue'
import api from '@/api'
import { useAppStore } from '@/store/app'
import { useIsMobile } from '@/composables/useIsMobile'
import { useLatestRequest } from '@/composables/useLatestRequest'
import { useViewActive } from '@/composables/useViewActive'
import CoverImage from '@/components/CoverImage.vue'
import ComicDetailDialog from '@/components/ComicDetailDialog.vue'
import KindTag from '@/components/KindTag.vue'
import PageBar from '@/components/PageBar.vue'
import StatCard from '@/components/StatCard.vue'
import { KIND_OPTIONS, formatBytes, formatTime, fromNow } from '@/utils/format'
import { readerPath, openReaderWindow } from '@/utils/reader'

const isMobile = useIsMobile()
const router = useRouter()
const trashVisible = ref(false)
const trashItems = ref([])
async function showTrash() {
  try { trashItems.value = await api.trash(); trashVisible.value = true } catch (_) {}
}
async function restoreItem(item) {
  try { await api.restoreTrash(item.id); ElMessage.success('已恢复'); await showTrash(); load() } catch (_) {}
}
const store = useAppStore()

// 视图切换：卡片 / 表格，默认卡片模式
const viewMode = ref('card')

// 详情弹窗
const detailVisible = ref(false)
const detailComic = ref(null)

function openDetail(row) {
  detailComic.value = {
    kind: row.kind,
    comicId: row.comicId,
    title: row.title
  }
  detailVisible.value = true
}

/** 打开在线阅读器（已下载的章节本地直发） */
function openReader(row, order = 1) {
  // 默认在新窗口打开阅读器
  openReaderWindow(row, order)
}

const kind = ref('')
const keyword = ref('')
const sort = ref('time')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const items = ref([])
const stats = ref(null)
const loading = ref(false)
const scanning = ref(false)
const deletingId = ref(null)
const deletingFiles = ref(false)

const sortOptions = [
  { value: 'time', label: '按更新时间' },
  { value: 'size', label: '按大小' }
]

const req = useLatestRequest()

async function load() {
  const { my, signal } = req.begin()
  loading.value = true
  try {
    const res = await api.library({
      kind: kind.value || undefined,
      keyword: keyword.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value,
      sort: sort.value || undefined,
      signal
    })
    if (!req.isCurrent(my)) return
    items.value = res?.items || []
    total.value = Number(res?.total) || 0
    if (res?.stats) stats.value = res.stats
  } catch (e) {
    if (e?.name === 'AbortError' || !req.isCurrent(my)) return
    items.value = []
    total.value = 0
  } finally {
    req.end(my)
    if (req.isCurrent(my)) loading.value = false
  }
}

function search() {
  page.value = 1
  load()
}

async function deleteItem(row) {
  try {
    await ElMessageBox.confirm(
      `确定从漫画库中移除《${row.title}》的记录吗？（不会删除磁盘文件）`,
      '移除记录',
      { type: 'warning', confirmButtonText: '仅移除记录', cancelButtonText: '取消' }
    )
  } catch (_) {
    return
  }
  deletingId.value = row.id
  try {
    await api.deleteLibrary(row.id, false)
    ElMessage.success('记录已移除')
    load()
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    deletingId.value = null
  }
}

async function deleteWithFiles(row) {
  try {
    await ElMessageBox.confirm(
      `确定将《${row.title}》及其文件移入回收站吗？可在回收站恢复。`,
      '移入回收站',
      {
        type: 'error',
        confirmButtonText: '移入回收站',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger'
      }
    )
  } catch (_) {
    return
  }
  deletingId.value = row.id
  deletingFiles.value = true
  try {
    await api.deleteLibrary(row.id, true)
    ElMessage.success('已移入回收站')
    load()
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    deletingId.value = null
    deletingFiles.value = false
  }
}

function handleCmd(cmd, row) {
  if (cmd === 'deleteItem') deleteItem(row)
  else if (cmd === 'deleteWithFiles') deleteWithFiles(row)
  else if (cmd === 'detail') openDetail(row)
}

async function rescan() {
  scanning.value = true
  try {
    const res = await api.scanLibrary()
    ElMessage.success(`扫描完成：发现 ${res?.found ?? 0} 部，新增 ${res?.added ?? 0} 部`)
    page.value = 1
    await load()
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    scanning.value = false
  }
}

const sizeText = computed(() => formatBytes(stats.value?.bytes))

const active = useViewActive({ onEnter: load })

watch(
  () => store.refreshTick,
  () => {
    if (active.value) load()
  }
)
</script>

<template>
  <el-dialog v-model="trashVisible" title="回收站" width="min(700px, 95vw)">
    <p class="ms-dim">移入回收站的文件仍占用磁盘空间。自动同步会跳过这些漫画，恢复后重新参与同步。</p>
    <div v-if="!trashItems.length" class="ms-empty">回收站为空</div>
    <div v-for="item in trashItems" :key="item.id" style="padding: 12px 0; overflow-wrap: anywhere">
      <span>{{ item.originalPath }}</span>
      <el-button text type="primary" @click="restoreItem(item)">恢复</el-button>
    </div>
  </el-dialog>
  <div class="ms-panel">
    <div class="ms-panel-title">
      <div class="title-left">
        <span>漫画库</span>
        <span class="ms-sub">· 共 {{ total }} 部</span>
      </div>
      <div class="head-actions">
        <el-button size="small" @click="showTrash">回收站</el-button>
        <div v-if="!isMobile" class="view-switch">
          <el-radio-group v-model="viewMode" size="small">
            <el-radio-button value="card">卡片</el-radio-button>
            <el-radio-button value="table">表格</el-radio-button>
          </el-radio-group>
        </div>
        <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        <el-button size="small" type="primary" :loading="scanning" @click="rescan">重新扫描目录</el-button>
      </div>
    </div>

    <!-- 顶部数据概览 -->
    <details class="library-summary" :open="!isMobile">
      <summary>书库概览 <span>{{ stats?.comics ?? total }} 部 · {{ sizeText }}</span></summary>
    <div class="ms-stat-grid mb14">
      <StatCard label="作品数" :value="stats?.comics ?? '—'" icon="Collection" color="#3b82f6" sub="库内漫画总数" />
      <StatCard label="章节数" :value="stats?.chapters ?? '—'" icon="Tickets" color="#10b981" sub="全部章节合计" />
      <StatCard label="图片数" :value="stats?.images ?? '—'" icon="Picture" color="#8b5cf6" sub="已收录图片" />
      <StatCard label="占用空间" :value="sizeText" icon="FolderOpened" color="#f59e0b" sub="本地存储空间" />
    </div>

    </details>

    <!-- 过滤搜索栏 -->
    <div class="ms-toolbar library-toolbar">
      <el-select v-model="kind" class="kind-select" placeholder="全部源" clearable @change="search">
        <el-option label="全部源" value="" />
        <el-option v-for="k in KIND_OPTIONS" :key="k.value" :label="k.label" :value="k.value" />
      </el-select>
      <el-select v-model="sort" class="sort-select" @change="search">
        <el-option v-for="s in sortOptions" :key="s.value" :label="s.label" :value="s.value" />
      </el-select>
      <el-input
        v-model="keyword"
        class="grow"
        placeholder="按标题 / 路径搜索"
        clearable
        @keyup.enter="search"
        @clear="search"
      >
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-button type="primary" :loading="loading" @click="search">搜索</el-button>
    </div>

    <!-- 卡片视图（桌面端卡片模式 或 手机端） -->
    <div v-if="viewMode === 'card' || isMobile" v-loading="loading" class="ms-comic-grid">
      <div v-for="row in items" :key="row.id" class="ms-comic lib-card">
        <!-- 封面区 -->
        <div class="ms-comic-cover" role="button" tabindex="0" :aria-label="'阅读 ' + row.title" @keydown.enter="openReader(row)" @keydown.space.prevent="openReader(row)" @click="openReader(row)">
          <CoverImage :kind="row.kind" :comic-id="row.comicId" :src="row.cover" :title="row.title" />
          <div class="ms-comic-tags">
            <KindTag :kind="row.kind" />
            <el-tag size="small" effect="plain" :type="row.source === 'scan' ? 'warning' : 'success'">
              {{ row.source === 'scan' ? '扫描' : '数据库' }}
            </el-tag>
          </div>
          <div class="cover-quick-read">
            <el-icon :size="16"><Reading /></el-icon>
            <span>立即阅读</span>
          </div>
        </div>

        <!-- 内容区 -->
        <div class="ms-comic-body">
          <div class="ms-comic-title" :title="row.title" @click="openReader(row)">
            {{ row.title || '未命名' }}
          </div>
          <div class="ms-comic-meta ms-mono" :title="row.comicId">
            {{ row.comicId }}
          </div>

          <!-- 章节 / 图片 / 占用空间 药丸栏 -->
          <div class="lib-meta-pills">
            <span class="pill" title="章节数">
              <el-icon :size="11"><Document /></el-icon>
              {{ row.chapters ?? 0 }} 章
            </span>
            <span class="pill" title="图片数">
              <el-icon :size="11"><Picture /></el-icon>
              {{ row.images ?? 0 }} 图
            </span>
            <span class="pill pill-size" title="大小">
              <el-icon :size="11"><Coin /></el-icon>
              {{ formatBytes(row.bytes) }}
            </span>
          </div>

          <!-- 路径小字 -->
          <div class="ms-comic-meta path-row" :title="row.path">
            {{ row.path || '—' }}
          </div>

          <!-- 时间信息 -->
          <div class="lib-time-row" :title="formatTime(row.updatedAt, true)">
            <el-icon :size="11"><Clock /></el-icon>
            <span>{{ fromNow(row.updatedAt) }} 更新</span>
          </div>

          <!-- 底部操作按钮 -->
          <div class="ms-comic-actions lib-card-actions">
            <el-button size="small" type="primary" @click.stop="openReader(row)">
              <el-icon :size="13"><Reading /></el-icon>
              <span>阅读</span>
            </el-button>
            <el-button size="small" plain @click.stop="openDetail(row)">
              详情
            </el-button>
            <el-dropdown trigger="click" @command="(cmd) => handleCmd(cmd, row)">
              <el-button size="small" class="more-btn">
                <el-icon :size="12"><MoreFilled /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="detail">
                    <el-icon><InfoFilled /></el-icon> 查看详情
                  </el-dropdown-item>
                  <el-dropdown-item command="deleteItem">
                    <el-icon><Delete /></el-icon> 移除记录
                  </el-dropdown-item>
                  <el-dropdown-item command="deleteWithFiles" divided style="color: var(--el-color-danger)">
                    <el-icon><Delete /></el-icon> 移入回收站
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </div>

      <!-- 空状态 -->
      <div v-if="!items.length && !loading" class="ms-empty lib-empty">
        <el-icon :size="36" style="margin-bottom: 8px; color: var(--ms-text-dim); opacity: 0.6"><FolderOpened /></el-icon>
        <div>漫画库还是空的，可以先到「收藏」页下载几本，或点「重新扫描目录」</div>
      </div>
    </div>

    <!-- 表格视图（仅桌面端切表格时展示） -->
    <el-table
      v-else
      v-loading="loading"
      :data="items"
      size="small"
      empty-text="漫画库还是空的，可以先到「收藏」页下载几本，或点「重新扫描目录」"
      row-key="id"
      class="lib-table"
    >
      <el-table-column prop="id" label="#" width="58" />
      <el-table-column label="源" width="76">
        <template #default="{ row }"><KindTag :kind="row.kind" /></template>
      </el-table-column>
      <el-table-column label="封面" width="66">
        <template #default="{ row }">
          <div class="table-cover-wrap" @click="openReader(row)">
            <CoverImage :kind="row.kind" :comic-id="row.comicId" :src="row.cover" :title="row.title" />
          </div>
        </template>
      </el-table-column>
      <el-table-column label="标题" min-width="200">
        <template #default="{ row }">
          <div class="cell-title hover-link" @click="openReader(row)">{{ row.title || '未命名' }}</div>
          <div class="cell-sub ms-mono">{{ row.comicId }}</div>
        </template>
      </el-table-column>
      <el-table-column label="章节" width="80">
        <template #default="{ row }">{{ row.chapters ?? 0 }}</template>
      </el-table-column>
      <el-table-column label="图片" width="90">
        <template #default="{ row }">{{ row.images ?? 0 }}</template>
      </el-table-column>
      <el-table-column label="大小" width="94">
        <template #default="{ row }">{{ formatBytes(row.bytes) }}</template>
      </el-table-column>
      <el-table-column label="路径" min-width="220">
        <template #default="{ row }">
          <span class="ms-mono path" :title="row.path">{{ row.path || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="来源" width="80">
        <template #default="{ row }">
          <el-tag size="small" effect="plain" :type="row.source === 'scan' ? 'warning' : 'success'">
            {{ row.source === 'scan' ? '扫描' : '数据库' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="140">
        <template #default="{ row }">
          <span :title="formatTime(row.updatedAt, true)">{{ fromNow(row.updatedAt) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="primary" @click="openReader(row)">阅读</el-button>
          <el-button size="small" plain @click="openDetail(row)">详情</el-button>
          <el-dropdown trigger="click" @command="(cmd) => handleCmd(cmd, row)">
            <el-button size="small" text>
              <el-icon><MoreFilled /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="deleteItem">移除记录</el-dropdown-item>
                <el-dropdown-item command="deleteWithFiles" divided style="color: var(--el-color-danger)">移入回收站</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>

    <PageBar v-model:page="page" v-model:page-size="pageSize" :total="total" :disabled="loading" @change="load" />

    <!-- 漫画详情弹窗 -->
    <ComicDetailDialog v-model="detailVisible" :comic="detailComic" :pick-chapters="false" />
  </div>
</template>

<style scoped>
.title-left {
  display: flex;
  align-items: center;
  gap: 6px;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.view-switch {
  margin-right: 4px;
}

.kind-select {
  width: 150px;
}

.sort-select {
  width: 150px;
}

.mb14 {
  margin-bottom: 14px;
}

.cell-title {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hover-link {
  cursor: pointer;
  transition: color 0.15s ease;
}

.hover-link:hover {
  color: var(--el-color-primary);
}

.cell-sub {
  font-size: 10px;
  color: var(--ms-text-dim);
}

.table-cover-wrap {
  width: 44px;
  height: 58px;
  border-radius: 4px;
  overflow: hidden;
  cursor: pointer;
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

/* 漫画库卡片专属定制 */
.lib-card {
  transition: transform 0.22s cubic-bezier(0.34, 1.56, 0.64, 1), box-shadow 0.22s ease, border-color 0.2s ease;
}

.lib-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--ms-shadow-hover);
  border-color: var(--el-color-primary-light-5);
}

.ms-comic-title {
  cursor: pointer;
  transition: color 0.15s ease;
}

.ms-comic-title:hover {
  color: var(--el-color-primary);
}

.cover-quick-read {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 8px 0;
  background: linear-gradient(to top, rgba(15, 23, 42, 0.88), transparent);
  color: #ffffff;
  font-size: 12px;
  font-weight: 500;
  opacity: 0;
  transform: translateY(6px);
  transition: opacity 0.2s ease, transform 0.2s ease;
  pointer-events: none;
  z-index: 2;
}

.ms-comic:hover .cover-quick-read {
  opacity: 1;
  transform: translateY(0);
}

.lib-meta-pills {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 5px;
  margin: 2px 0;
}

.lib-meta-pills .pill {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  padding: 1px 6px;
  background: var(--ms-bg-soft);
  border: 1px solid var(--ms-border);
  border-radius: 4px;
  color: var(--ms-text-dim);
}

.lib-meta-pills .pill-size {
  color: var(--el-color-primary);
  font-weight: 500;
}

.path-row {
  font-size: 10px;
  color: var(--ms-text-dim);
}

.lib-time-row {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--ms-text-dim);
  margin-top: 1px;
}

.lib-card-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
}

.lib-card-actions :deep(.el-button) {
  flex: 1;
  margin-left: 0;
}

.lib-card-actions :deep(.more-btn) {
  flex: 0 0 32px;
  padding: 0;
}

.lib-empty {
  grid-column: 1 / -1;
}

@media (max-width: 640px) {
  .kind-select,
  .sort-select {
    width: 100%;
  }
}

.library-summary > summary { display: none; }
@media(max-width:768px) {
 .library-summary { margin-bottom: 16px; background: var(--ms-bg-soft); border-radius: 8px; }
 .library-summary > summary { display: list-item; cursor: pointer; padding: 14px; font-size: 12px; }
 .library-summary > summary span { color: var(--ms-text-dim); margin-left: 10px; }
 .library-summary .ms-stat-grid { padding: 0 10px 10px; }
 .library-toolbar { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); }
 .library-toolbar > .grow, .library-toolbar > .el-button { grid-column: 1 / 3; }
 .library-toolbar > * { min-width: 0; }
}
</style>
