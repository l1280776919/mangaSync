<script setup>
import {
  Reading,
  Download,
  MoreFilled,
  Tickets,
  Star,
  Delete,
  Document,
  Picture,
  Coin,
  EditPen
} from '@element-plus/icons-vue'
import CoverImage from '@/components/CoverImage.vue'
import KindTag from '@/components/KindTag.vue'
import { downloadStateOf, formatBytes, formatTime } from '@/utils/format'

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
  /** 是否显示「阅读」按钮（收藏 / 漫画库卡片用） */
  readable: { type: Boolean, default: false },
  busy: { type: Boolean, default: false }
})

const emit = defineEmits(['download', 'queue', 'collect', 'uncollect', 'toggle', 'open', 'read'])

const dl = () => downloadStateOf(props.item)

function handleCmd(cmd) {
  if (cmd === 'queue') emit('queue', props.item)
  else if (cmd === 'collect') emit('collect', props.item)
  else if (cmd === 'uncollect') emit('uncollect', props.item)
  else if (cmd === 'read') emit('read', props.item)
  else if (cmd === 'download') emit('download', props.item)
}
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
      <div v-if="readable" class="cover-quick-read" @click.stop="emit('read', item)">
        <el-icon :size="15"><Reading /></el-icon>
        <span>立即阅读</span>
      </div>
    </div>

    <div class="ms-comic-body">
      <div class="ms-comic-title" :title="item.title" @click="emit('open', item)">
        {{ item.title || '未命名' }}
      </div>
      <div class="ms-comic-meta author-meta" :title="item.author">
        <el-icon :size="11"><EditPen /></el-icon>
        <span>{{ item.author || '未知作者' }}</span>
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

      <!-- 优化后的按钮栏：主次分明、绝对底端平齐、绝不换行错位 -->
      <div class="ms-comic-actions card-action-bar">
        <!-- 按钮 1：若可读显示阅读，否则若可收藏显示收藏，否则显示整本下载 -->
        <el-button
          v-if="readable"
          size="small"
          type="primary"
          class="flex-btn"
          @click.stop="emit('read', item)"
        >
          <el-icon :size="12"><Reading /></el-icon>
          <span>阅读</span>
        </el-button>

        <!-- 按钮 2：下载整本 -->
        <el-button
          v-if="showDownload"
          size="small"
          :type="readable ? 'default' : 'primary'"
          class="flex-btn"
          :loading="busy"
          @click.stop="emit('download', item)"
        >
          <el-icon :size="12"><Download /></el-icon>
          <span>下载</span>
        </el-button>

        <!-- 搜索页专用的收藏按钮 -->
        <el-button
          v-if="showCollect && !showDownload && !readable"
          size="small"
          type="success"
          class="flex-btn"
          @click.stop="emit('collect', item)"
        >
          <el-icon :size="12"><Star /></el-icon>
          <span>收藏</span>
        </el-button>

        <!-- 更多操作下拉（选章、取消收藏等） -->
        <el-dropdown
          v-if="showDownload || favorited || showCollect"
          trigger="click"
          @command="handleCmd"
        >
          <el-button size="small" class="more-icon-btn">
            <el-icon :size="13"><MoreFilled /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-if="showDownload" command="queue">
                <el-icon><Tickets /></el-icon> 选章加入队列
              </el-dropdown-item>
              <el-dropdown-item v-if="showCollect" command="collect">
                <el-icon><Star /></el-icon> 加入收藏
              </el-dropdown-item>
              <el-dropdown-item
                v-if="favorited"
                command="uncollect"
                divided
                style="color: var(--el-color-danger)"
              >
                <el-icon><Delete /></el-icon> 取消收藏
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ms-comic {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.ms-comic-body {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.ms-comic-title {
  cursor: pointer;
  transition: color 0.15s ease;
}

.ms-comic-title:hover {
  color: var(--el-color-primary);
}

.author-meta {
  display: flex;
  align-items: center;
  gap: 4px;
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

/* 按钮栏底端对齐与防错位 */
.card-action-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: auto;
  padding-top: 8px;
}

.flex-btn {
  flex: 1;
  min-width: 0;
  margin-left: 0 !important;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
  padding: 0 4px;
}

.more-icon-btn {
  flex: 0 0 32px !important;
  width: 32px;
  padding: 0;
  margin-left: 0 !important;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
</style>