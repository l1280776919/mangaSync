/**
 * 列表请求的「竞态 + 超时」保护（每个需要它的视图自己 new 一个）。
 *
 * 解决的问题：
 * - 快速翻页 / 连点两次搜索 / 切账号时，先发的请求可能后返回，把新结果覆盖成旧数据
 *   → 每次发起新请求都作废旧请求（abort），并且只有「最后一次」请求能写回数据；
 * - fetch 没有超时，隧道卡住时 loading 永久转圈
 *   → 超时后 abort，由最后一次请求的 finally 收尾（loading 一定能复位）。
 *
 * 用法：
 *   const req = useLatestRequest()
 *   async function load() {
 *     const { my, signal } = req.begin()
 *     loading.value = true
 *     try {
 *       const res = await api.library({ ..., signal })
 *       if (!req.isCurrent(my)) return
 *       items.value = res?.items || []
 *     } catch (e) {
 *       if (e?.name === 'AbortError' || !req.isCurrent(my)) return
 *       items.value = []
 *     } finally {
 *       req.end()
 *       if (req.isCurrent(my)) loading.value = false
 *     }
 *   }
 */
export function useLatestRequest(timeout = 20000) {
  let seq = 0
  let ctl = null
  let timer = null

  function clearTimer() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  /** 发起一次新请求：作废旧请求，返回 { my, signal } */
  function begin() {
    if (ctl) {
      try {
        ctl.abort()
      } catch (_) {
        /* 已经结束的请求，忽略 */
      }
    }
    clearTimer()
    const my = ++seq
    ctl = new AbortController()
    const ac = ctl
    timer = setTimeout(() => {
      try {
        ac.abort()
      } catch (_) {
        /* ignore */
      }
    }, timeout)
    return { my, signal: ac.signal }
  }

  /** my 还是最新的那次请求吗？ */
  function isCurrent(my) {
    return my === seq
  }

  /** 请求收尾（清掉超时定时器） */
  function end() {
    clearTimer()
  }

  return { begin, isCurrent, end }
}

export default useLatestRequest
