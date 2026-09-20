<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import api from '@/api'
import { useAppStore } from '@/store/app'
import { useDownloadActions } from '@/composables/useDownloadActions'
import ComicCard from '@/components/ComicCard.vue'
import ComicDetailDialog from '@/components/ComicDetailDialog.vue'
import PageBar from '@/components/PageBar.vue'
import { KIND_OPTIONS, SORT_OPTIONS } from '@/utils/format'

const store = useAppStore()

const kind = ref('pica')
const accountId = ref(null)
const keyword = ref('')
const sort = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const items = ref([])
const loading = ref(false)
const collectedIds = ref([])
const collectingId = ref(null)
const searched = ref(false)

const {
  pickerVisible,
  pickerComic,
  detailVisible,
  detailComic,
  busy,
  downloadWhole,
  openPicker,
  openDetail,
  submitPicker
} = useDownloadActions(() => accountId.value)

const kindAccounts = computed(() => store.accounts.filter((a) => a.kind === kind.value))
const sortOptions = computed(() => SORT_OPTIONS[kind.value] || [])

async function load() {
  page.value = page.value || 1
  loading.value = true
  try {
    const res = await api.search({
      kind: kind.value,
      keyword: keyword.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value,
      accountId: accountId.value || undefined,
      sort: sort.value || undefined
    })
    items.value = res?.items || []
    total.value = Number(res?.total) || 0
  } catch (e) {
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
    searched.value = true
  }
}

function submit() {
  if (!keyword.value.trim()) {
    ElMessage.warning('请输入关键词')
    return
  }
  page.value = 1
  load()
}

async function onCollect(item) {
  if (!accountId.value) {
    ElMessage.warning('请先选择要收藏到的账号（同源账号）')
    return
  }
  collectingId.value = String(item.comicId)
  try {
    await api.addFavorite(accountId.value, String(item.comicId))
    collectedIds.value = [...new Set([...collectedIds.value, String(item.comicId)])]
    ElMessage.success('已加入收藏')
    store.loadAccounts(true).catch(() => {})
  } catch (e) {
    /* api.js 已提示 */
  } finally {
    collectingId.value = null
  }
}

async function onDownload(item) {
  await downloadWhole(item)
  store.loadActiveJobs().catch(() => {})
}

watch(kind, () => {
  accountId.value = null
  sort.value = ''
  items.value = []
  total.value = 0
  searched.value = false
})

onMounted(async () => {
  await store.loadAccounts().catch(() => {})
})

const quickKeywords = ['同人', '短篇', '中文', '单行本']
</script>

<template>
  <div class="ms-panel">
    <div class="ms-panel-title">
      <span>
        搜索
        <span class="ms-sub">· 走各源官方接口（/api/search）</span>
      </span>
      <span v-if="total" class="ms-dim">共 {{ total }} 条结果</span>
    </div>

    <div class="ms-toolbar">
      <el-radio-group v-model="kind" size="default">
        <el-radio-button v-for="k in KIND_OPTIONS" :key="k.value" :value="k.value">
          {{ k.label }}
        </el-radio-button>
      </el-radio-group>

      <el-select v-model="accountId" placeholder="账号（可选）" clearable class="acct-select">
        <el-option
          v-for="a in kindAccounts"
          :key="a.id"
          :label="`${a.label || a.username}${a.status === 'ok' ? '' : '（异常）'}`"
          :value="a.id"
        />
      </el-select>

      <el-select v-model="sort" placeholder="排序" clearable class="sort-select">
        <el-option v-for="s in sortOptions" :key="s.value" :label="s.label" :value="s.value" />
      </el-select>

      <el-input
        v-model="keyword"
        placeholder="输入关键词，回车搜索"
        class="grow"
        clearable
        @keyup.enter="submit"
        @clear="() => (searched = false)"
      >
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>

      <el-button type="primary" :loading="loading" @click="submit">搜索</el-button>
    </div>

    <div class="quick-row">
      <span class="ms-dim">快捷：</span>
      <el-tag
        v-for="k in quickKeywords"
        :key="k"
        class="quick-tag"
        effect="plain"
        @click="keyword = k; submit()"
      >
        {{ k }}
      </el-tag>
      <span v-if="!kindAccounts.length" class="ms-dim note">
        （该源还没有账号，未选账号时部分源可能搜索失败）
      </span>
    </div>

    <el-empty
      v-if="!searched && !loading"
      description="输入关键词开始搜索"
    />

    <el-empty
      v-else-if="searched && !loading && !items.length"
      description="没有搜索结果，换个关键词试试"
    />

    <div v-else v-loading="loading" class="ms-comic-grid">
      <ComicCard
        v-for="item in items"
        :key="`${item.kind}-${item.comicId}`"
        :item="item"
        show-collect
        :busy="collectingId === String(item.comicId) || busy"
        @download="onDownload"
        @queue="openPicker"
        @collect="onCollect"
        @open="openDetail"
      >
        <template #badge>
          <el-tag
            v-if="collectedIds.includes(String(item.comicId))"
            size="small"
            type="success"
            effect="dark"
          >
            已收藏
          </el-tag>
        </template>
      </ComicCard>
    </div>

    <PageBar v-model:page="page" v-model:page-size="pageSize" :total="total" :disabled="loading" @change="load" />

    <div class="foot-tip ms-dim">
      提示：下载任务入队后可在「任务」页查看进度；结果为已下载状态时显示「已下载」标签。
    </div>
  </div>

  <ComicDetailDialog v-model="pickerVisible" :comic="pickerComic" @submit="submitPicker" />
  <ComicDetailDialog v-model="detailVisible" :comic="detailComic" :pick-chapters="false" />
</template>

<style scoped>
.acct-select {
  width: 190px;
}

.sort-select {
  width: 150px;
}

.quick-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 14px;
  font-size: 12px;
}

.quick-tag {
  cursor: pointer;
}

.note {
  font-size: 12px;
}

.foot-tip {
  font-size: 12px;
  padding-top: 10px;
}

@media (max-width: 640px) {
  .acct-select,
  .sort-select {
    width: 100%;
  }
}
</style>
