import { onActivated, onDeactivated, ref } from 'vue'

/**
 * keep-alive 视图的「当前可见」标记 + 进出钩子。
 *
 * 视图被 <keep-alive> 缓存后 onUnmounted 不会再触发，定时器 / 轮询必须在
 * onDeactivated 里停掉，否则人已经离开页面，后台还在一直请求；
 * 反过来 onActivated 是「切回页面」的唯一时机，需要新数据的页面在这里重新加载。
 *
 * 用法：
 *   const active = useViewActive({ onEnter: () => { startTimer(); load() }, onLeave: stopTimer })
 *   watch(() => store.jobTick, () => { if (active.value) load() })
 *
 * 注意：不在 keep-alive 里的组件拿不到 onActivated/onDeactivated，本 hook 只给
 * 被缓存的业务页（App.vue 的 router-view）使用，登录 / 阅读器等裸页面不要用。
 */
export function useViewActive({ onEnter, onLeave } = {}) {
  const active = ref(false)

  onActivated(() => {
    active.value = true
    if (onEnter) onEnter()
  })

  onDeactivated(() => {
    active.value = false
    if (onLeave) onLeave()
  })

  return active
}

export default useViewActive
