<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/api'
import { useIsMobile } from '@/composables/useIsMobile'
import CoverImage from '@/components/CoverImage.vue'
import KindTag from '@/components/KindTag.vue'
import PageBar from '@/components/PageBar.vue'
import StatCard from '@/components/StatCard.vue'
import { KIND_OPTIONS, formatBytes, formatTime, fromNow } from '@/utils/format'

/* 手机端：宽表格换成 2 列封面卡片网格 */
const isMobile = useIsMobile()

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

async function load() {
  loading.value = true
  try {
    const res = await api.library({
      kind: kind.value || undefined,
      keyword: keyword.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value,
      sort: sort.value || undefined
    })
    items.value = res?.items || []
    total.value = Number(res?.total) || 0
    if (res?.stats) stats.value = res.stats
  } catch (e) {
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  load()
}

async function deleteItem(row) {
  try {
    await ElMessageBox.confirm(
      `确定从漫画库中移除《${row.title}》的记录吗？`,
      '移除记录',
      { type: 'warning', confirmButtonText: '仅移除记录', cancelButtonText: '取消' }
    )
  } catch (_) {
    return
  }
  try {
    await api.deleteLibrary(row.id, false)
    ElMessage.success('记录已移除')
    load()
  } catch (e) {
    /* api.js 已提示 */
  }
}

async function deleteWithFiles(row) {
  try {
    await ElMessageBox.confirm(
      `确定删除《${row.title}》并同时删除磁盘目录吗？\n${row.path || ''}\n此操作不可恢复！`,
      '危险操作：删除文件',
      {
        type: 'error',
        confirmButtonText: '删除记录 + 文件',
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
    ElMessage.success('已删除记录与文件')
    load()
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    deletingId.value = null
    deletingFiles.value = false
  }
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

onMounted(load)
</script>

<template>
  <div class="ms-panel">
    <div class="ms-panel-title">
      <span>
        漫画库
        <span class="ms-sub">· 共 {{ total }} 部</span>
      </span>
      <div class="head-actions">
        <el-button size="small" :loading="loading" @click="load">刷新</el-button>
        <el-button size="small" type="primary" :loading="scanning" @click="rescan">重新扫描目录</el-button>
      </div>
    </div>

    <div class="ms-stat-grid mb14">
      <StatCard label="作品数" :value="stats?.comics ?? '—'" icon="Collection" sub="库内漫画总数" />
      <StatCard label="章节数" :value="stats?.chapters ?? '—'" icon="Tickets" />
      <StatCard label="图片数" :value="stats?.images ?? '—'" icon="Picture" />
      <StatCard label="占用空间" :value="sizeText" icon="FolderOpened" />
    </div>

    <div class="ms-toolbar">
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

    <!-- 手机端：2 列封面卡片（宽屏仍是表格） -->
    <div v-if="isMobile" v-loading="loading" class="ms-comic-grid">
      <div v-for="row in items" :key="row.id" class="ms-comic">
        <div class="ms-comic-cover">
          <CoverImage :kind="row.kind" :comic-id="row.comicId" :src="row.cover" :title="row.title" />
          <div class="ms-comic-tags">
            <KindTag :kind="row.kind" />
            <el-tag size="small" effect="plain" :type="row.source === 'scan' ? 'warning' : 'success'">
              {{ row.source === 'scan' ? '扫描' : '数据库' }}
            </el-tag>
          </div>
        </div>
        <div class="ms-comic-body">
          <div class="ms-comic-title" :title="row.title">{{ row.title || '未命名' }}</div>
          <div class="ms-comic-meta ms-mono" :title="row.comicId">{{ row.comicId }}</div>
          <div class="lib-meta">
            <span>{{ row.chapters ?? 0 }} 章</span>
            <span>{{ row.images ?? 0 }} 图</span>
            <span>{{ formatBytes(row.bytes) }}</span>
          </div>
          <div class="ms-comic-meta" :title="row.path">{{ row.path || '—' }}</div>
          <div class="ms-comic-meta" :title="formatTime(row.updatedAt, true)">
            {{ fromNow(row.updatedAt) }} 更新
          </div>
          <div class="lib-actions">
            <el-button size="small" :loading="deletingId === row.id" @click="deleteItem(row)">移除记录</el-button>
            <el-button
              size="small"
              type="danger"
              plain
              :loading="deletingId === row.id && deletingFiles"
              @click="deleteWithFiles(row)"
            >
              删除文件
            </el-button>
          </div>
        </div>
      </div>
      <div v-if="!items.length && !loading" class="ms-empty lib-empty">
        漫画库还是空的，可以先到「收藏」页下载几本，或点「重新扫描目录」
      </div>
    </div>

    <el-table
      v-else
      v-loading="loading"
      :data="items"
      size="small"
      empty-text="漫画库还是空的，可以先到「收藏」页下载几本，或点「重新扫描目录」"
      row-key="id"
    >
      <el-table-column prop="id" label="#" width="58" />
      <el-table-column label="源" width="76">
        <template #default="{ row }"><KindTag :kind="row.kind" /></template>
      </el-table-column>
      <el-table-column label="标题" min-width="200">
        <template #default="{ row }">
          <div class="cell-title">{{ row.title || '未命名' }}</div>
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
      <el-table-column label="操作" width="180" fixed="right">
        <template #default="{ row }">
          <el-button size="small" :loading="deletingId === row.id" @click="deleteItem(row)">移除记录</el-button>
          <el-button
            size="small"
            type="danger"
            plain
            :loading="deletingId === row.id && deletingFiles"
            @click="deleteWithFiles(row)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <PageBar v-model:page="page" v-model:page-size="pageSize" :total="total" :disabled="loading" @change="load" />
  </div>
</template>

<style scoped>
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

@media (max-width: 640px) {
  .kind-select,
  .sort-select {
    width: 100%;
  }
}

/* 手机端卡片网格里的信息行与操作按钮 */
.lib-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 11px;
  color: var(--ms-text-dim);
}

.lib-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 4px;
}

.lib-actions :deep(.el-button) {
  width: 100%;
  margin-left: 0;
}

.lib-empty {
  grid-column: 1 / -1;
}
</style>
