/**
 * 在线阅读器的统一跳转路径（收藏页 / 漫画库共用）。
 *
 * 约定：comicId 必须 encodeURIComponent（可能带 / 等特殊字符），
 * kind / order 由 router 的 /reader/:kind/:comicId/:order 解析。
 *
 * @param {{kind?: string, comicId?: string|number}} row 漫画行数据
 * @param {number} order 章节序号
 * @param {string} fallbackKind 行数据没有 kind 时的兜底源（如当前选中账号的 kind）
 * @returns {string|null} 不可用时返回 null（调用方跳过跳转）
 */
export function readerPath(row, order = 1, fallbackKind = '') {
  const kind = row?.kind || fallbackKind
  const comicId = row?.comicId
  if (!kind || !comicId) return null
  return `/reader/${kind}/${encodeURIComponent(comicId)}/${order}`
}

export default readerPath
