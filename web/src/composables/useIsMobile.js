import { onMounted, onUnmounted, ref } from 'vue'

/**
 * 全站手机端断点（与 App.vue 的侧边栏抽屉保持一致）：
 * ≤768px 视为手机端，视图层用它在「表格」与「卡片列表」之间切换。
 *
 * 复用一个模块级 ref + 一份 resize 监听，避免每个组件都挂监听。
 */
export const MOBILE_MAX_WIDTH = 768

const isMobile = ref(
  typeof window !== 'undefined' ? window.innerWidth <= MOBILE_MAX_WIDTH : false
)

let refCount = 0

function onResize() {
  isMobile.value = window.innerWidth <= MOBILE_MAX_WIDTH
}

export function useIsMobile() {
  onMounted(() => {
    if (refCount === 0) {
      window.addEventListener('resize', onResize, { passive: true })
    }
    refCount += 1
    onResize()
  })

  onUnmounted(() => {
    refCount -= 1
    if (refCount <= 0) {
      refCount = 0
      window.removeEventListener('resize', onResize)
    }
  })

  return isMobile
}

export default useIsMobile
