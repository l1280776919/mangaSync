/**
 * 在线阅读器的统一跳转路径与打开逻辑（收藏页 / 漫画库共用）。
 */
export function readerPath(row, order = 1, fallbackKind = '') {
  const kind = row?.kind || fallbackKind
  const comicId = row?.comicId || row?.id
  if (!kind || !comicId) return null
  return `/reader/${kind}/${encodeURIComponent(comicId)}/${order}`
}

/**
 * 默认在新窗口 / 新标签页打开阅读器，不打断用户当前的浏览上下文
 */
export function openReaderWindow(row, order = 1, fallbackKind = '') {
  const path = readerPath(row, order, fallbackKind)
  if (!path) return
  const fullUrl = window.location.origin + window.location.pathname + '#' + path
  window.open(fullUrl, '_blank')
}

export default readerPath
