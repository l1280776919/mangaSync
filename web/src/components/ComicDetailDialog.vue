<script setup>
import { computed, ref, watch } from 'vue'
import api from '@/api'
import CoverImage from '@/components/CoverImage.vue'
import KindTag from '@/components/KindTag.vue'

/**
 * 漫画详情 / 章节选择弹窗。
 * - 打开时拉 GET /api/comics/{kind}/{comicId} 拿章节列表
 * - 「下载整本」→ emit submit { all: true }
 * - 「下载选中章节」→ emit submit { chapters: [order...] }
 */
const props = defineProps({
  modelValue: { type: Boolean, default: false },
  comic: { type: Object, default: null }, // { kind, comicId, title }
  pickChapters: { type: Boolean, default: true }
})

const emit = defineEmits(['update:modelValue', 'submit'])

const loading = ref(false)
const detail = ref(null)
const error = ref('')
const selected = ref([])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const chapters = computed(() => detail.value?.chapters || [])
const selectedCount = computed(() => selected.value.length)
const allSelected = computed(
  () => chapters.value.length > 0 && selected.value.length === chapters.value.length
)

async function load() {
  if (!props.comic?.kind || !props.comic?.comicId) return
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    detail.value = await api.comic(props.comic.kind, props.comic.comicId)
    // 默认勾选未下载的章节
    const undownloaded = (detail.value?.chapters || [])
      .filter((c) => !c.downloaded)
      .map((c) => c.order)
    selected.value = undownloaded.length ? undownloaded : (detail.value?.chapters || []).map((c) => c.order)
  } catch (e) {
    error.value = e.message || '加载详情失败'
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.modelValue, props.comic?.kind, props.comic?.comicId],
  ([open]) => {
    if (open) load()
    else {
      detail.value = null
      error.value = ''
      selected.value = []
    }
  },
  { immediate: true }
)

function toggleAll() {
  selected.value = allSelected.value ? [] : chapters.value.map((c) => c.order)
}

function submitAll() {
  emit('submit', { all: true })
  visible.value = false
}

function submitSelected() {
  if (!selectedCount.value) return
  emit('submit', { chapters: [...selected.value].sort((a, b) => a - b) })
  visible.value = false
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="detail?.title || comic?.title || '漫画详情'"
    width="min(720px, 94vw)"
    top="6vh"
    append-to-body
  >
    <div v-loading="loading" class="detail-wrap">
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />

      <template v-if="detail">
        <div class="detail-head">
          <div class="detail-cover">
            <CoverImage
              :kind="detail.kind"
              :comic-id="detail.comicId"
              :src="detail.cover"
              :title="detail.title"
            />
          </div>
          <div class="detail-meta">
            <div class="row">
              <KindTag :kind="detail.kind" />
              <span class="ms-mono ms-dim">{{ detail.comicId }}</span>
            </div>
            <div class="row">
              <span class="k">作者</span>
              <span>{{ detail.author || '未知' }}</span>
            </div>
            <div class="row">
              <span class="k">分类</span>
              <span>
                <el-tag
                  v-for="c in detail.categories || []"
                  :key="c"
                  size="small"
                  effect="plain"
                  class="mr4"
                >
                  {{ c }}
                </el-tag>
                <span v-if="!(detail.categories || []).length" class="ms-dim">—</span>
              </span>
            </div>
            <div v-if="(detail.tags || []).length" class="row">
              <span class="k">标签</span>
              <span>
                <el-tag
                  v-for="t in detail.tags"
                  :key="t"
                  size="small"
                  type="info"
                  effect="plain"
                  class="mr4"
                >
                  {{ t }}
                </el-tag>
              </span>
            </div>
            <div class="row">
              <span class="k">章节</span>
              <span>共 {{ detail.chapters?.length || 0 }} 章，已下载 {{ detail.downloadedChapters ?? 0 }}</span>
            </div>
            <div v-if="detail.localPath" class="row">
              <span class="k">本地</span>
              <span class="ms-mono path">{{ detail.localPath }}</span>
            </div>
            <div v-if="detail.description" class="row">
              <span class="k">简介</span>
              <span class="desc">{{ detail.description }}</span>
            </div>
          </div>
        </div>

        <div v-if="pickChapters" class="chapter-box">
          <div class="chapter-head">
            <span>选择章节（已选 {{ selectedCount }}/{{ chapters.length }}）</span>
            <el-button size="small" text @click="toggleAll">
              {{ allSelected ? '取消全选' : '全选' }}
            </el-button>
          </div>
          <el-checkbox-group v-model="selected" class="chapter-list">
            <el-checkbox
              v-for="c in chapters"
              :key="c.order"
              :value="c.order"
              :label="c.order"
              class="chapter-item"
            >
              <span class="chapter-title">{{ c.title || `第 ${c.order} 话` }}</span>
              <span class="ms-dim chapter-info">
                {{ c.images ?? '?' }} 图
                <el-tag v-if="c.downloaded" size="small" type="success" effect="plain">已下载</el-tag>
              </span>
            </el-checkbox>
          </el-checkbox-group>
          <div v-if="!chapters.length" class="ms-empty">该作品没有返回章节列表</div>
        </div>
      </template>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
      <el-button
        v-if="pickChapters"
        :disabled="!selectedCount"
        @click="submitSelected"
      >
        下载选中 {{ selectedCount }} 章
      </el-button>
      <el-button type="primary" :loading="loading" @click="submitAll">下载整本</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.detail-wrap {
  min-height: 120px;
}

.detail-head {
  display: flex;
  gap: 14px;
  margin-bottom: 14px;
}

.detail-cover {
  width: 132px;
  height: 176px;
  flex-shrink: 0;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid var(--ms-border);
}

.detail-meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
}

.row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.row .k {
  color: var(--ms-text-dim);
  width: 34px;
  flex-shrink: 0;
}

.path {
  font-size: 11px;
  word-break: break-all;
}

.desc {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.mr4 {
  margin-right: 4px;
}

.chapter-box {
  border-top: 1px solid var(--ms-border);
  padding-top: 10px;
}

.chapter-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  margin-bottom: 6px;
}

.chapter-list {
  display: flex;
  flex-direction: column;
  max-height: 260px;
  overflow-y: auto;
  gap: 2px;
}

.chapter-item {
  display: flex;
  width: 100%;
  height: auto;
  padding: 4px 0;
  margin: 0;
}

.chapter-item :deep(.el-checkbox__label) {
  display: flex;
  justify-content: space-between;
  width: 100%;
  gap: 10px;
}

.chapter-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chapter-info {
  font-size: 11px;
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

@media (max-width: 640px) {
  .detail-head {
    flex-direction: column;
  }
  .detail-cover {
    width: 110px;
    height: 146px;
  }
}
</style>
