<script setup>
import { computed } from 'vue'
import KindTag from '@/components/KindTag.vue'
import StatusTag from '@/components/StatusTag.vue'
import {
  formatBytes,
  formatElapsed,
  formatSpeed,
  formatTime,
  fromNow,
  jobProgress
} from '@/utils/format'

/**
 * 任务进度行（概览页与任务页共用）。
 */
const props = defineProps({
  job: { type: Object, required: true },
  compact: { type: Boolean, default: false },
  /** 是否显示「删除记录」（概览页只看进行中的任务，没有删除入口） */
  removable: { type: Boolean, default: true }
})

defineEmits(['cancel', 'retry', 'remove', 'logs'])

const progress = computed(() => jobProgress(props.job))
const pctText = computed(() => {
  const j = props.job
  const it = Number(j.imagesTotal) || 0
  const id = Number(j.imagesDone) || 0
  if (it > 0) return `${id}/${it} 张`
  const ct = Number(j.chaptersTotal) || 0
  const cd = Number(j.chaptersDone) || 0
  if (ct > 0) return `${cd}/${ct} 章`
  return '—'
})

const barStatus = computed(() => {
  if (props.job.status === 'failed') return 'exception'
  if (props.job.status === 'done') return 'success'
  return undefined
})

const elapsed = computed(() => {
  const j = props.job
  // 与任务页表格共用同一份耗时格式化（原来两处各写了一遍）
  return formatElapsed(j.startedAt, j.finishedAt)
})

const active = computed(() => props.job.status === 'queued' || props.job.status === 'running')
</script>

<template>
  <div class="ms-job-row">
    <div class="ms-job-head">
      <div class="job-title-wrap">
        <KindTag :kind="job.kind" />
        <span class="job-title ms-ellipsis" :title="job.title">{{ job.title || job.comicId }}</span>
        <span class="ms-mono ms-dim job-id">#{{ job.id }}</span>
      </div>
      <div class="job-head-right">
        <StatusTag domain="job" :value="job.status" />
      </div>
    </div>

    <el-progress
      :percentage="progress"
      :status="barStatus"
      :stroke-width="compact ? 6 : 8"
      :text-inside="false"
    />

    <div class="job-meta">
      <span>进度 {{ pctText }}</span>
      <span>章节 {{ job.chaptersDone ?? 0 }}/{{ job.chaptersTotal ?? 0 }}</span>
      <span v-if="active">速度 {{ formatSpeed(job.speedBps) }}</span>
      <span v-if="job.bytes">已下载 {{ formatBytes(job.bytes) }}</span>
      <span v-if="job.currentChapter">当前：{{ job.currentChapter }}</span>
      <span>耗时 {{ elapsed }}</span>
      <span v-if="job.createdAt">创建 {{ fromNow(job.createdAt) }}</span>
      <span v-if="job.finishedAt">完成 {{ formatTime(job.finishedAt) }}</span>
    </div>

    <div v-if="job.error" class="job-error">
      <el-icon :size="13"><WarningFilled /></el-icon>
      <span>{{ job.error }}</span>
    </div>

    <div v-if="job.localPath" class="ms-mono ms-dim job-path" :title="job.localPath">
      {{ job.localPath }}
    </div>

    <div class="job-actions">
      <el-button v-if="active" size="small" type="warning" plain @click="$emit('cancel', job)">
        取消
      </el-button>
      <el-button
        v-if="job.status === 'failed' || job.status === 'canceled'"
        size="small"
        type="primary"
        plain
        @click="$emit('retry', job)"
      >
        重试
      </el-button>
      <el-button size="small" @click="$emit('logs', job)">日志</el-button>
      <el-button
        v-if="removable"
        size="small"
        type="danger"
        plain
        @click="$emit('remove', job)"
      >
        删除记录
      </el-button>
    </div>
  </div>
</template>

<style scoped>
.job-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.job-title {
  font-weight: 600;
  font-size: 13px;
  max-width: 100%;
}

.job-id {
  flex-shrink: 0;
}

.job-head-right {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-shrink: 0;
}

.job-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 11px;
  color: var(--ms-text-dim);
}

.job-error {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 12px;
  color: var(--el-color-danger);
  word-break: break-all;
}

.job-path {
  font-size: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.job-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
</style>
