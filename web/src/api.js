import { ElMessage } from 'element-plus'

/**
 * 统一的 API 封装。
 * - 一律使用相对路径 /api/...（后端 embed 同源托管，开发期由 vite proxy 转发）
 * - 非 2xx 时读取 {"error":"..."} 并用 ElMessage 报错，然后抛出 Error
 * - query 里 undefined / null / '' 的参数自动丢弃
 */

const BASE = '/api'

function buildQuery(params) {
  if (!params) return ''
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    sp.append(k, String(v))
  }
  const qs = sp.toString()
  return qs ? `?${qs}` : ''
}

export class ApiError extends Error {
  constructor(message, status, payload) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.payload = payload
  }
}

async function request(path, { method = 'GET', body, query, silent = false, raw = false, signal } = {}) {
  const url = `${BASE}${path}${buildQuery(query)}`
  let res
  try {
    res = await fetch(url, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal
    })
  } catch (e) {
    if (e && e.name === 'AbortError') throw e
    const msg = `网络请求失败：${e.message || '无法连接到后端'}`
    if (!silent) ElMessage.error(msg)
    throw new ApiError(msg, 0, null)
  }

  if (res.status === 204) return null

  if (!res.ok) {
    let msg = `请求失败（HTTP ${res.status}）`
    let payload = null
    try {
      const ct = res.headers.get('content-type') || ''
      if (ct.includes('application/json')) {
        payload = await res.json()
        if (payload && typeof payload.error === 'string' && payload.error) msg = payload.error
      } else {
        const text = (await res.text()).slice(0, 300)
        if (text) msg = text
      }
    } catch (_) {
      /* 忽略解析错误，用默认提示 */
    }
    if (!silent) ElMessage.error(msg)
    throw new ApiError(msg, res.status, payload)
  }

  if (raw) return res
  if (res.status === 204) return null
  const ct = res.headers.get('content-type') || ''
  if (!ct.includes('application/json')) return res.text()
  return await res.json()
}

export const api = {
  request,

  // 健康检查
  health: () => request('/health', { silent: true }),
  stats: () => request('/stats'),

  // 设置
  getSettings: () => request('/settings'),
  saveSettings: (patch) => request('/settings', { method: 'PUT', body: patch }),

  // 账号
  listAccounts: () => request('/accounts'),
  createAccount: (body) => request('/accounts', { method: 'POST', body }),
  updateAccount: (id, patch) => request(`/accounts/${id}`, { method: 'PATCH', body: patch }),
  deleteAccount: (id) => request(`/accounts/${id}`, { method: 'DELETE' }),
  loginAccount: (id) => request(`/accounts/${id}/login`, { method: 'POST' }),
  syncAccount: (id) => request(`/accounts/${id}/sync`, { method: 'POST' }),

  // 收藏
  favorites: (accountId, { keyword, page = 1, pageSize = 20 } = {}) =>
    request(`/accounts/${accountId}/favorites`, { query: { keyword, page, pageSize } }),
  addFavorite: (accountId, comicId) =>
    request(`/accounts/${accountId}/favorites`, { method: 'POST', body: { comicId } }),
  removeFavorite: (accountId, comicId) =>
    request(`/accounts/${accountId}/favorites/${encodeURIComponent(comicId)}`, { method: 'DELETE' }),

  // 漫画
  comic: (kind, comicId) =>
    request(`/comics/${kind}/${encodeURIComponent(comicId)}`),
  coverUrl: (kind, comicId) => `${BASE}/comics/${kind}/${encodeURIComponent(comicId)}/cover`,

  // 搜索
  search: ({ kind, keyword, page = 1, pageSize = 20, accountId, sort } = {}) =>
    request('/search', { query: { kind, keyword, page, pageSize, accountId, sort } }),

  // 下载任务
  listDownloads: ({ status, page = 1, pageSize = 20 } = {}) =>
    request('/downloads', { query: { status, page, pageSize } }),
  createDownload: ({ kind, accountId, comicId, title, chapters, all }) =>
    request('/downloads', { method: 'POST', body: { kind, accountId, comicId, title, chapters, all } }),
  cancelDownload: (id) => request(`/downloads/${id}/cancel`, { method: 'POST' }),
  retryDownload: (id) => request(`/downloads/${id}/retry`, { method: 'POST' }),
  deleteDownload: (id) => request(`/downloads/${id}`, { method: 'DELETE' }),
  downloadLogs: (id) => request(`/downloads/${id}/logs`, { silent: true }),

  // 漫画库
  library: ({ kind, keyword, page = 1, pageSize = 20, sort } = {}) =>
    request('/library', { query: { kind, keyword, page, pageSize, sort } }),
  deleteLibrary: (id, files = false) =>
    request(`/library/${id}`, { method: 'DELETE', query: { files } }),
  scanLibrary: () => request('/library/scan', { method: 'POST' })
}

export default api
