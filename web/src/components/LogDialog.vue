<script setup>
import { computed, ref, watch, onUnmounted } from 'vue'
import api from '@/api'
import { formatTime } from '@/utils/format'

/**
 * 任务日志弹窗：拉 GET /api/downloads/{id}/logs
 * 打开时自动每 3 秒刷新一次，关闭即停止。
 */
const props = defineProps({
  modelValue: { type: Boolean, default: false },
  job: { type: Object, default: null }
})

const emit = defineEmits(['update:modelValue'])

const logs = ref([])
const loading = ref(false)
const error = ref('')
const autoRefresh = ref(true)
const showAll = ref(false)
let timer = null

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const shown = computed(() => (showAll.value ? logs.value : logs.value.slice(-200)))

async function load(silent = false) {
  if (!props.job?.id) return
  if (!silent) loading.value = true
  error.value = ''
  try {
    const data = await api.downloadLogs(props.job.id)
    logs.value = Array.isArray(data) ? data : (data?.items ?? [])
  } catch (e) {
    error.value = e.message || '加载日志失败'
  } finally {
    loading.value = false
  }
}

function startTimer() {
  stopTimer()
  timer = setInterval(() => {
    if (autoRefresh.value) load(true)
  }, 3000)
}

function stopTimer() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      logs.value = []
      load()
      startTimer()
    } else {
      stopTimer()
    }
  },
  { immediate: true }
)

onUnmounted(stopTimer)

const levelClass = (lv) => {
  const s = String(lv || '').toLowerCase()
  if (s === 'error' || s === 'fatal') return 'lv-error'
  if (s === 'warn' || s === 'warning') return 'lv-warn'
  if (s === 'debug') return 'lv-debug'
  return 'lv-info'
}
</script>

<template>
  <el-dialog v-model="visible" :title="`任务日志 #${job?.id ?? ''}`" width="min(760px, 94vw)" top="8vh" append-to-body>
    <div class="log-toolbar">
      <span class="ms-dim">共 {{ logs.length }} 条{{ showAll ? '' : '，显示最近 200 条' }}</span>
      <div class="right">
        <el-checkbox v-model="autoRefresh" label="自动刷新" size="small" />
        <el-checkbox v-model="showAll" label="显示全部" size="small" />
        <el-button size="small" :loading="loading" @click="load()">手动刷新</el-button>
      </div>
    </div>

    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon class="mb8" />

    <div v-loading="loading" class="log-box">
      <div v-for="(l, i) in shown" :key="i" class="log-line">
        <span class="ts ms-mono">{{ formatTime(l.ts, true) }}</span>
        <span class="lv ms-mono" :class="levelClass(l.level)">{{ (l.level || 'info').toUpperCase() }}</span>
        <span class="msg">{{ l.msg }}</span>
      </div>
      <div v-if="!shown.length && !loading" class="ms-empty">暂无日志</div>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.log-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
  font-size: 12px;
}

.log-toolbar .right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.log-box {
  background: #0f1116;
  border: 1px solid var(--ms-border);
  border-radius: 8px;
  padding: 8px 10px;
  max-height: 52vh;
  overflow-y: auto;
  font-size: 12px;
}

.log-line {
  display: flex;
  gap: 8px;
  padding: 2px 0;
  border-bottom: 1px dashed rgba(255, 255, 255, 0.04);
  align-items: flex-start;
}

.ts {
  color: #6b7686;
  flex-shrink: 0;
}

.lv {
  flex-shrink: 0;
  width: 52px;
}

.lv-info {
  color: #6da8ff;
}

.lv-warn {
  color: #e6a23c;
}

.lv-error {
  color: #f56c6c;
}

.lv-debug {
  color: #8b949e;
}

.msg {
  word-break: break-all;
  white-space: pre-wrap;
}

.mb8 {
  margin-bottom: 8px;
}
</style>
