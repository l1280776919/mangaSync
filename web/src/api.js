import { ElMessage } from 'element-plus'
import { clearSession, notifyUnauthorized } from '@/store/auth'

/**
 * 统一的 API 封装。
 * - 一律使用相对路径 /api/...（后端 embed 同源托管，开发期由 vite proxy 转发）
 * - 非 2xx 时读取 {"error":"..."} 并用 ElMessage 报错，然后抛出 Error
 * - query 里 undefined / null / '' 的参数自动丢弃
 * - 认证：登录态由 HttpOnly cookie（ms_session）维持，前端不需要手动带 token
 * - 401 → 清登录态并跳登录页（/api/auth/login、/api/auth/me 自身除外，避免死循环）
 * - 403 + mustChangePassword → 跳改密页
 */

const BASE = '/api'

/** 这些接口的 401 不做「会话失效」跳转，由调用方自己处理 */
const AUTH_EXEMPT_PATHS = new Set(['/auth/login', '/auth/me'])

let lastExpiredNoticeAt = 0

function noticeSessionExpired() {
  const now = Date.now()
  if (now - lastExpiredNoticeAt < 3000) return
  lastExpiredNoticeAt = now
  ElMessage.warning('登录状态已失效，请重新登录')
}

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
    this.retryAfter = null
  }
}

/** 把接口错误翻译成人话（登录 401/429、改密 401、未认证 403 各有专门文案） */
export function authErrorMessage(e) {
  const status = e?.status
  const fromServer = e?.payload && typeof e.payload.error === 'string' ? e.payload.error : ''
  if (fromServer) return fromServer
  if (status === 0) return '无法连接到后端服务，请确认服务已启动'
  if (status === 401) return '账号或密码错误'
  if (status === 403) return '没有权限访问该接口'
  if (status === 429) return '尝试过于频繁，请稍后再试'
  return e?.message || '请求失败'
}

async function request(path, { method = 'GET', body, query, silent = false, raw = false, signal } = {}) {
  const url = `${BASE}${path}${buildQuery(query)}`
  let res
  try {
    res = await fetch(url, {
      method,
      credentials: 'same-origin', // 带上 ms_session cookie
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

    const err = new ApiError(msg, res.status, payload)

    if (res.status === 401) {
      // 「原密码不正确」也会返回 401，此时会话仍然有效，不能登出
      const oldPasswordWrong =
        path === '/auth/password' && String(payload?.error || '').includes('原密码')
      if (!oldPasswordWrong) {
        clearSession()
        if (!AUTH_EXEMPT_PATHS.has(path)) {
          // 受保护接口的 401：统一跳登录页（局部错误提示下面照常抛出，由调用方展示）
          if (!silent) noticeSessionExpired()
          notifyUnauthorized()
        }
      }
    } else if (res.status === 403 && payload && payload.mustChangePassword) {
      // 已登录但必须改密：后端拒绝其它接口，前端强制跳改密页（不清会话）
      notifyUnauthorized({ mustChangePassword: true })
    } else if (res.status === 429) {
      const ra = Number(res.headers.get('retry-after'))
      if (Number.isFinite(ra) && ra > 0) err.retryAfter = ra
    }

    if (!silent) ElMessage.error(msg)
    throw err
  }

  if (raw) return res
  const ct = res.headers.get('content-type') || ''
  if (!ct.includes('application/json')) return res.text()
  return await res.json()
}

export const api = {
  request,

  // 健康检查（始终免认证，探活用）
  health: () => request('/health', { silent: true }),
  stats: () => request('/stats'),

  /* ---------------- 认证（登录页 + 服务端会话 + 首次强制改密） ---------------- */

  /** POST /api/auth/login → { username, mustChangePassword, isAdmin, lastLoginAt } */
  login: (username, password) =>
    request('/auth/login', { method: 'POST', body: { username, password }, silent: true }),

  /** POST /api/auth/logout → { ok: true }（未登录也返回 200） */
  logout: () => request('/auth/logout', { method: 'POST', silent: true }),

  /** GET /api/auth/me → { username, mustChangePassword, isAdmin, lastLoginAt, sessionExpiresAt } */
  me: () => request('/auth/me', { silent: true }),

  /** POST /api/auth/password → { ok: true, mustChangePassword: false } */
  changePassword: (oldPassword, newPassword) =>
    request('/auth/password', {
      method: 'POST',
      body: { oldPassword, newPassword },
      silent: true
    }),

  // 设置（authUser / authPass 字段已随旧认证方案移除）
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
