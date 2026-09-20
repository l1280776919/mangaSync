import { createRouter, createWebHashHistory } from 'vue-router'
import api from '@/api'
import { auth, clearSession, setSession, setUnauthorizedHandler } from '@/store/auth'

/**
 * 用 hash 路由，后端不需要配 history fallback。
 *
 * 认证模型（见 docs/AUTH.md）：
 * - 登录态 = HttpOnly cookie（ms_session），前端唯一可信来源是 GET /api/auth/me
 * - 进任何页面前先问一次 me：401 → /login；mustChangePassword → /change-password
 * - 已登录访问 /login → /dashboard
 */
const routes = [
  { path: '/', redirect: '/dashboard' },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { title: '登录', bare: true }
  },
  {
    path: '/change-password',
    name: 'change-password',
    component: () => import('@/views/ChangePasswordView.vue'),
    meta: { title: '修改密码', bare: true }
  },
  {
    // 在线阅读器：整屏独立页面（不带侧边栏）
    path: '/reader/:kind/:comicId/:order',
    name: 'reader',
    component: () => import('@/views/ReaderView.vue'),
    meta: { title: '阅读', bare: true }
  },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: () => import('@/views/DashboardView.vue'),
    meta: { title: '概览', icon: 'Odometer' }
  },
  {
    path: '/accounts',
    name: 'accounts',
    component: () => import('@/views/AccountsView.vue'),
    meta: { title: '账号', icon: 'User' }
  },
  {
    path: '/favorites',
    name: 'favorites',
    component: () => import('@/views/FavoritesView.vue'),
    meta: { title: '收藏', icon: 'Star' }
  },
  {
    path: '/search',
    name: 'search',
    component: () => import('@/views/SearchView.vue'),
    meta: { title: '搜索', icon: 'Search' }
  },
  {
    path: '/downloads',
    name: 'downloads',
    component: () => import('@/views/DownloadsView.vue'),
    meta: { title: '任务', icon: 'Download' }
  },
  {
    path: '/library',
    name: 'library',
    component: () => import('@/views/LibraryView.vue'),
    meta: { title: '漫画库', icon: 'Collection' }
  },
  {
    path: '/settings',
    name: 'settings',
    component: () => import('@/views/SettingsView.vue'),
    meta: { title: '设置', icon: 'Setting' }
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

const LOGIN_PATH = '/login'
const CHANGE_PASSWORD_PATH = '/change-password'
const HOME_PATH = '/dashboard'

/* ------------------------------------------------------------------ *
 * 401 / 403 统一处理（api.js 里所有请求都会走这里）
 * ------------------------------------------------------------------ */
setUnauthorizedHandler(({ mustChangePassword: forced } = {}) => {
  const current = router.currentRoute.value

  if (forced) {
    // 已登录但必须改密 → 只能去改密页
    if (current.path !== CHANGE_PASSWORD_PATH) router.replace(CHANGE_PASSWORD_PATH)
    return
  }

  clearSession()
  if (current.path === LOGIN_PATH) return // 已经在登录页，不重复跳
  const redirect = current.fullPath && current.fullPath !== '/' && current.path !== HOME_PATH
    ? current.fullPath
    : undefined
  router.replace({ path: LOGIN_PATH, query: redirect ? { redirect } : undefined })
})

/* ------------------------------------------------------------------ *
 * 全局守卫
 * ------------------------------------------------------------------ */

/** 并发导航（首屏 + 重定向）只发一次 me */
let meInflight = null

function fetchMe() {
  if (!meInflight) {
    meInflight = api.me().finally(() => {
      meInflight = null
    })
  }
  return meInflight
}

function loginRedirect(to) {
  const full = to.fullPath
  const keep = full && full !== '/' && full !== HOME_PATH && !full.startsWith(LOGIN_PATH)
  return { path: LOGIN_PATH, query: keep ? { redirect: full } : undefined }
}

router.beforeEach(async (to) => {
  const isLogin = to.path === LOGIN_PATH
  const isChangePw = to.path === CHANGE_PASSWORD_PATH

  // 已知登录态且不欠改密，访问登录页直接回首页（少打一次 me）
  if (isLogin && auth.user && !auth.user.mustChangePassword) return { path: HOME_PATH }

  let me
  try {
    me = await fetchMe()
  } catch (e) {
    // 401（未登录）或网络错误（后端没起来）都降级到登录页
    clearSession()
    if (isLogin) return true
    return loginRedirect(to)
  }

  setSession(me)

  // 已登录：登录页没意义
  if (isLogin) return { path: me.mustChangePassword ? CHANGE_PASSWORD_PATH : HOME_PATH }

  // 首次登录强制改密：除改密页外一律拦下
  if (me.mustChangePassword && !isChangePw) return { path: CHANGE_PASSWORD_PATH }

  return true
})

router.afterEach((to) => {
  const t = to.meta?.title
  document.title = t ? `${t} · mangaSync` : 'mangaSync'
})

export default router
