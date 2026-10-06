import { ref, onUnmounted } from 'vue'
import api from '@/api'
export function useSyncStatus(onFinished = () => {}) {
  const runs = ref({})
  const error = ref('')
  let timer, generation = 0
  const isRunning = id => ['queued', 'running'].includes(runs.value[id]?.status)
  function stop() { generation++; clearTimeout(timer) }
  function start() {
    stop()
    const current = generation
    async function poll() {
      try {
        const result = await api.syncStatus()
        if (current !== generation) return
        const next = {}
        for (const run of result || []) {
          if (isRunning(run.accountId) && !['queued', 'running'].includes(run.status)) onFinished(run)
          next[run.accountId] = run
        }
        runs.value = next
        error.value = ''
      } catch (e) {
        if (current === generation) error.value = '同步进度暂时获取失败，正在重试；后台任务继续执行'
      } finally {
        if (current === generation) timer = setTimeout(poll, 2500)
      }
    }
    poll()
  }
  onUnmounted(stop)
  return { runs, error, isRunning, start, stop }
}
