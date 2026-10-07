<script setup>
import { computed, ref, watch, onBeforeUnmount, onDeactivated } from 'vue'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'
import api from '@/api'
const props = defineProps({ modelValue: Boolean, comic: Object })
const emit = defineEmits(['update:modelValue'])
const router = useRouter()
const opened = computed({ get: () => props.modelValue, set: v => emit('update:modelValue', v) })
const run = ref(null)
const busy = ref(false)
const repairing = ref(null)
const error = ref('')
let timer, generation = 0
const running = computed(() => run.value?.status === 'running')
const labels = { checked: '检查完成', issues: '发现异常', skipped: '已跳过', error: '检查失败', stale: '需要重查', canceled: '已停止' }
function stop() { generation++; clearTimeout(timer) }
async function refresh() {
  const g = generation
  clearTimeout(timer)
  try {
    const value = await api.libraryHealth()
    if (g !== generation || !opened.value) return
    run.value = value?.id ? value : null
    error.value = ''
  } catch (e) { if (g === generation) error.value = e.message }
  if (g === generation && opened.value && (running.value || error.value)) timer = setTimeout(refresh, 2000)
}
async function start(deep) {
  if (busy.value) return
  busy.value = true
  stop()
  try {
    run.value = await api.startLibraryHealth({ id: props.comic?.id || 0, deep })
    error.value = ''
    if (opened.value) refresh()
  } catch (e) { error.value = e.message } finally { busy.value = false }
}
async function cancel() {
  try { await api.cancelLibraryHealth(run.value.id); await refresh() } catch (_) {}
}
async function repair(item) {
  if (repairing.value) return
  repairing.value = item.id
  try {
    const result = await api.repairLibrary(item.id, run.value.id)
    item.jobId = result.id
    ElMessage.success('修复已加入下载队列，完成后请重新体检')
  } catch (_) {} finally { repairing.value = null }
}
function tasks() { opened.value = false; router.push('/downloads') }
watch(opened, value => { stop(); if (value) refresh() })
onBeforeUnmount(stop)
onDeactivated(() => { stop(); opened.value = false })
</script>
<template>
  <el-dialog v-model="opened" title="书库体检" align-center width="min(760px, 94vw)" class="health-dialog">
    <p class="scope">{{ comic ? `检查《${comic.title}》` : '检查整个漫画库' }}</p>
    <p class="hint">快速检查缺页、空文件和已知旧版 GIF 异常；深度检查会逐页解码并核对已有校验值。检查不改动图片，也不访问漫画源。</p>
    <div class="health-actions">
      <el-button :disabled="running || busy" @click="start(false)">快速检查</el-button>
      <el-button type="primary" :disabled="running || busy" :loading="busy" @click="start(true)">深度检查</el-button>
      <el-button v-if="running" @click="cancel">停止检查</el-button>
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <section v-if="run" aria-live="polite" class="health-status">
      <strong>{{ running ? '正在检查' : run.status === 'canceled' ? '检查已停止' : '体检完成' }} · {{ run.deep ? '深度' : '快速' }} · {{ run.processed }} / {{ run.total }} 部</strong>
      <p v-if="run.current">{{ run.current }}</p>
      <el-progress :percentage="run.total ? Math.round(run.processed / run.total * 100) : 0" :show-text="false" />
      <p class="hint">关闭窗口不影响后台检查。结果保留至下次体检或服务重启；修复需要漫画源账号，只下载异常章节中不能复用的页面。</p>
    </section>
    <div v-if="run?.results?.length" class="health-results">
      <article v-for="item in run.results" :key="item.id" class="health-result">
        <div class="result-heading"><strong>{{ item.title }}</strong><el-tag :type="item.status === 'issues' ? 'warning' : 'info'">{{ labels[item.status] || item.status }}</el-tag></div>
        <p class="hint">已检查 {{ item.pages }} 页<span v-if="item.issueCount"> · {{ item.issueCount }} 项异常</span></p>
        <p v-if="item.message" class="hint">{{ item.message }}</p>
        <details v-if="item.issues?.length"><summary>查看异常明细</summary><ul><li v-for="(issue, i) in item.issues" :key="i">第 {{ issue.chapter }} 章<span v-if="issue.page"> · 第 {{ issue.page }} 页</span>：{{ issue.reason }}</li></ul><p v-if="item.issueCount > item.issues.length">仅展示前 {{ item.issues.length }} 项；修复范围包含全部已识别异常页。</p></details>
        <el-button v-if="item.status === 'issues' && item.chapters?.length" type="primary" plain :disabled="running || !!item.jobId || !!repairing" :loading="repairing === item.id" @click="repair(item)">{{ item.jobId ? '修复已入队' : `修复 ${item.chapters.length} 个异常章节` }}</el-button>
      </article>
    </div>
    <template #footer><el-button @click="tasks">查看下载任务</el-button><el-button @click="opened = false">关闭</el-button></template>
  </el-dialog>
</template>
<style scoped>
.scope { margin-top: 0; font-weight: 600; overflow-wrap: anywhere; }
.hint { color: var(--ms-text-dim); font-size: 12px; line-height: 1.7; overflow-wrap: anywhere; }
.health-actions { display: flex; gap: 8px; flex-wrap: wrap; margin: 16px 0; }
.health-actions :deep(.el-button) { margin-left: 0; min-height: 40px; }
.health-status { margin: 20px 0; }
.health-status > p { overflow-wrap: anywhere; }
.health-results { max-height: 42dvh; overflow-y: auto; overscroll-behavior: contain; }
.health-result { padding: 16px 0; border-top: 1px solid var(--ms-border); }
.result-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.result-heading strong { overflow-wrap: anywhere; }
.result-heading :deep(.el-tag) { flex-shrink: 0; }
summary { cursor: pointer; padding: 8px 0 14px; }
li { line-height: 1.8; overflow-wrap: anywhere; }
ul { padding-left: 18px; }
</style>

<style>
.health-dialog { display: flex; flex-direction: column; max-height: calc(100dvh - 32px); }
.health-dialog .el-dialog__body { min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
.health-dialog .el-dialog__header, .health-dialog .el-dialog__footer { flex-shrink: 0; }
</style>
