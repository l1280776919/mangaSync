<script setup>
import CoverImage from '@/components/CoverImage.vue'
import KindTag from '@/components/KindTag.vue'
import { downloadStateOf, formatBytes, formatTime } from '@/utils/format'

/**
 * 漫画卡片（收藏 / 搜索结果通用）。
 * 只负责展示 + 抛事件，具体动作由父级实现。
 */
const props = defineProps({
  item: { type: Object, required: true },
  selectable: { type: Boolean, default: false },
  selected: { type: Boolean, default: false },
  /** 是否显示「取消收藏」按钮（收藏页用） */
  favorited: { type: Boolean, default: false },
  /** 是否显示「加入收藏」按钮（搜索页用） */
  showCollect: { type: Boolean, default: false },
  /** 是否显示下载相关按钮 */
  showDownload: { type: Boolean, default: true },
  busy: { type: Boolean, default: false }
})

const emit = defineEmits(['download', 'queue', 'collect', 'uncollect', 'toggle', 'open'])

const dl = () => downloadStateOf(props.item)
</script>

<template>
  <div class="ms-comic" :class="{ 'is-selected': selected }">
    <div class="ms-comic-check" v-if="selectable">
      <el-checkbox
        :model-value="selected"
        @change="emit('toggle', item)"
        @click.stop
      />
    </div>

    <div class="ms-comic-cover" @click="emit('open', item)">
      <CoverImage :kind="item.kind" :comic-id="item.comicId" :src="item.cover" :title="item.title" />
      <div class="ms-comic-tags">
        <slot name="badge" />
        <KindTag :kind="item.kind" />
      </div>
    </div>

    <div class="ms-comic-body">
      <div class="ms-comic-title" :title="item.title" @click="emit('open', item)">
        {{ item.title || '未命名' }}
      </div>
      <div class="ms-comic-meta" :title="item.author">
        <el-icon :size="11"><EditPen /></el-icon>
        {{ item.author || '未知作者' }}
      </div>

      <div class="tags-row">
        <el-tag
          v-for="c in (item.categories || []).slice(0, 2)"
          :key="`c-${c}`"
          size="small"
          effect="plain"
        >
          {{ c }}
        </el-tag>
        <el-tag
          v-for="t in (item.tags || []).slice(0, 1)"
          :key="`t-${t}`"
          size="small"
          type="info"
          effect="plain"
        >
          {{ t }}
        </el-tag>
        <span v-if="!(item.categories || []).length && !(item.tags || []).length" class="ms-dim">
          无标签
        </span>
      </div>

      <div class="meta-row">
        <span>
          <el-icon :size="11"><Document /></el-icon>
          {{ item.chapters ?? 0 }} 章
        </span>
        <span v-if="item.images">
          <el-icon :size="11"><Picture /></el-icon>
          {{ item.images }} 图
        </span>
        <span v-if="item.bytes">
          <el-icon :size="11"><Coin /></el-icon>
          {{ formatBytes(item.bytes) }}
        </span>
      </div>

      <div class="state-row">
        <el-tag :type="dl().type" size="small" effect="plain">{{ dl().label }}</el-tag>
        <span v-if="item.updatedAt || item.latestEp" class="ms-dim tiny">
          {{ item.latestEp || formatTime(item.updatedAt) }}
        </span>
      </div>

      <div v-if="item.localPath" class="ms-mono ms-dim path" :title="item.localPath">
        {{ item.localPath }}
      </div>

      <div class="ms-comic-actions">
        <el-button
          v-if="showDownload"
          size="small"
          type="primary"
          :loading="busy"
          @click.stop="emit('download', item)"
        >
          下载
        </el-button>
        <el-button
          v-if="showDownload"
          size="small"
          :disabled="busy"
          @click.stop="emit('queue', item)"
        >
          加入队列
        </el-button>
        <el-button
          v-if="showCollect"
          size="small"
          type="success"
          plain
          @click.stop="emit('collect', item)"
        >
          收藏
        </el-button>
        <el-button
          v-if="favorited"
          size="small"
          type="danger"
          plain
          @click.stop="emit('uncollect', item)"
        >
          取消收藏
        </el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tags-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-height: 20px;
  font-size: 11px;
}

.meta-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 11px;
  color: var(--ms-text-dim);
}

.meta-row span {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}

.state-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.tiny {
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.path {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 10px;
}
</style>
