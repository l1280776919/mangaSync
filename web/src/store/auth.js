import { reactive } from 'vue'

/**
 * 登录会话状态（**不是 pinia store**）。
 *
 * 路由守卫和 api.js 都需要在组件之外同步读写登录态，而 pinia 实例在
 * `app.use(createPinia())` 之前是不可用的，所以这里用模块级 reactive 对象，
 * 不依赖 pinia 是否已安装。视图里直接 `import { auth }` 用即可（模板中自动解包）。
 *
 * 服务端会话靠 HttpOnly cookie（ms_session）维持，前端只缓存 /api/auth/me 的返回：
 *   { username, mustChangePassword, isAdmin, lastLoginAt, sessionExpiresAt }
 */
export const auth = reactive({
  user: null,
  loaded: false
})

/** 登录成功 / me 成功时写入 */
export function setSession(user) {
  auth.user = user || null
  auth.loaded = !!user
  return auth.user
}

/** 局部更新（如改密成功后 mustChangePassword 变 false） */
export function patchSession(patch = {}) {
  if (!auth.user) return null
  Object.assign(auth.user, patch)
  return auth.user
}

export function setMustChangePassword(flag) {
  if (auth.user) auth.user.mustChangePassword = !!flag
}

/** 401 时清本地状态（服务端 cookie 由后端失效） */
export function clearSession() {
  auth.user = null
  auth.loaded = false
}

export function isLoggedIn() {
  return !!auth.user
}

export function mustChangePassword() {
  return !!auth.user?.mustChangePassword
}

export function currentUsername() {
  return auth.user?.username || ''
}

/* ------------------------------------------------------------------ *
 * 401 / 403 统一处理钩子
 * api.js 只依赖本模块，由 router.js 注册真正的跳转逻辑，
 * 这样 api.js ⇄ router.js 不会形成循环依赖。
 * ------------------------------------------------------------------ */
let unauthorizedHandler = null

export function setUnauthorizedHandler(fn) {
  unauthorizedHandler = typeof fn === 'function' ? fn : null
}

/**
 * @param {{ mustChangePassword?: boolean }} opts
 *   mustChangePassword=true 表示「已登录但必须改密」，应跳改密页而不是登录页。
 */
export function notifyUnauthorized(opts = {}) {
  if (!unauthorizedHandler) return
  try {
    unauthorizedHandler(opts)
  } catch (_) {
    /* 跳转失败不影响原请求的错误抛出 */
  }
}
