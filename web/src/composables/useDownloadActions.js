import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '@/api'

/**
 * 下载相关的复用逻辑：整本下载 / 章节选择下载。
 * @param {() => number|undefined} getAccountId 取当前上下文账号 id（可为空）
 */
export function useDownloadActions(getAccountId) {
  const pickerVisible = ref(false)
  const pickerComic = ref(null)
  const detailVisible = ref(false)
  const detailComic = ref(null)
  /**
   * 正在下载的 comicId 集合：点一张卡的「下载」只让这一张转圈。
   * 以前是一个全局 busy，点任意一张卡所有卡片一起转圈（页面看起来像卡死）。
   */
  const busyIds = ref(new Set())
  /** 批量 / 弹窗提交的整体状态（按钮级别的 loading） */
  const submitting = ref(false)

  const busy = computed(() => submitting.value || busyIds.value.size > 0)

  const keyOf = (item) => String(item?.comicId ?? '')

  function markBusy(id) {
    const s = new Set(busyIds.value)
    s.add(String(id))
    busyIds.value = s
  }

  function unmarkBusy(id) {
    const s = new Set(busyIds.value)
    s.delete(String(id))
    busyIds.value = s
  }

  /** 这张卡是否正在下载（模板里给 ComicCard 的 :busy 用） */
  function isBusy(item) {
    return busyIds.value.has(keyOf(item))
  }

  function accountId(payload) {
    return payload?.accountId ?? getAccountId?.() ?? undefined
  }

  /** 直接整本入队 */
  async function downloadWhole(item, { silent = false } = {}) {
    const id = keyOf(item)
    markBusy(id)
    try {
      const res = await api.createDownload({
        kind: item.kind,
        accountId: accountId(item),
        comicId: String(item.comicId),
        title: item.title || undefined,
        all: true
      })
      if (!silent) {
        ElMessage.success(res?.id ? `已加入下载队列（任务 #${res.id}）` : '已加入下载队列')
      }
      return res
    } catch (e) {
      // api.js 已弹出错误提示
      return null
    } finally {
      unmarkBusy(id)
    }
  }

  /** 打开章节选择弹窗（加入下载队列） */
  function openPicker(item) {
    pickerComic.value = {
      kind: item.kind,
      comicId: String(item.comicId),
      title: item.title,
      accountId: accountId(item)
    }
    pickerVisible.value = true
  }

  /** 打开详情弹窗（只读浏览） */
  function openDetail(item) {
    detailComic.value = { kind: item.kind, comicId: String(item.comicId), title: item.title }
    detailVisible.value = true
  }

  /** 章节选择弹窗提交 */
  async function submitPicker(payload) {
    const c = pickerComic.value
    if (!c) return null
    submitting.value = true
    try {
      const res = await api.createDownload({
        kind: c.kind,
        accountId: c.accountId,
        comicId: String(c.comicId),
        title: c.title || undefined,
        chapters: payload.chapters,
        all: !!payload.all
      })
      ElMessage.success(res?.id ? `已加入下载队列（任务 #${res.id}）` : '已加入下载队列')
      return res
    } catch (e) {
      return null
    } finally {
      submitting.value = false
    }
  }

  /** 批量整本下载 */
  async function downloadMany(items) {
    if (!items?.length) return
    submitting.value = true
    for (const it of items) markBusy(keyOf(it))
    let ok = 0
    let fail = 0
    try {
      for (const it of items) {
        try {
          await api.createDownload({
            kind: it.kind,
            accountId: accountId(it),
            comicId: String(it.comicId),
            title: it.title || undefined,
            all: true
          })
          ok++
        } catch (e) {
          fail++
        } finally {
          // 一个下载完就点亮它自己的卡片，不必等整批结束
          unmarkBusy(keyOf(it))
        }
      }
    } finally {
      submitting.value = false
      for (const it of items) unmarkBusy(keyOf(it))
    }
    if (ok && !fail) ElMessage.success(`已加入下载队列：${ok} 个`)
    else if (ok && fail) ElMessage.warning(`成功 ${ok} 个，失败 ${fail} 个`)
    else if (fail) ElMessage.error(`全部失败（${fail} 个）`)
  }

  return {
    pickerVisible,
    pickerComic,
    detailVisible,
    detailComic,
    busy,
    busyIds,
    isBusy,
    downloadWhole,
    downloadMany,
    openPicker,
    openDetail,
    submitPicker
  }
}

export default useDownloadActions
