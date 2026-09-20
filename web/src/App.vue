<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/store/app'
import { auth } from '@/store/auth'
import { logout } from '@/composables/useAuth'
import { useIsMobile } from '@/composables/useIsMobile'

const route = useRoute()
const router = useRouter()
const store = useAppStore()

const isMobile = useIsMobile()
const drawerOpen = ref(false)
const loggingOut = ref(false)

/** 登录 / 首改密码页是独立整屏页，不套侧边栏外壳 */
const AUTH_PAGES = ['/login', '/change-password']
/** 裸页面：登录/改密/在线阅读器 —— 整屏渲染，不套侧边栏 */
const isAuthPage = computed(() => route.meta?.bare === true || AUTH_PAGES.includes(route.path))
const username = computed(() => auth.user?.username || '未登录')

// 导航顺序显式声明（router.getRoutes() 的顺序不可靠）
const MENU = [
  { path: '/dashboard', title: '概览', icon: 'Odometer' },
  { path: '/accounts', title: '账号', icon: 'User' },
  { path: '/favorites', title: '收藏', icon: 'Star' },
  { path: '/search', title: '搜索', icon: 'Search' },
  { path: '/downloads', title: '任务', icon: 'Download' },
  { path: '/library', title: '漫画库', icon: 'Collection' },
  { path: '/settings', title: '设置', icon: 'Setting' }
]

const activePath = computed(() => route.path)
const pageTitle = computed(() => route.meta?.title || '概览')

const runningCount = computed(
  () => store.jobList.filter((j) => j.status === 'running' || j.status === 'queued').length
)

const sseTag = computed(() => {
  if (store.sseConnected) return { type: 'success', text: '实时推送' }
  if (store.sseStatus === 'connecting') return { type: 'info', text: '连接中…' }
  if (store.sseStatus === 'offline') return { type: 'warning', text: '轮询模式' }
  return { type: 'info', text: '未连接' }
})

watch(isMobile, (m) => {
  if (!m) drawerOpen.value = false
})

function go(path) {
  router.push(path)
  drawerOpen.value = false
}

async function onLogout() {
  if (loggingOut.value) return
  loggingOut.value = true
  drawerOpen.value = false
  try {
    await logout()
  } finally {
    loggingOut.value = false
  }
}

/** 业务页才需要账号 / 统计 / SSE；登录页不需要（省掉一堆 401 请求） */
function bootstrap() {
  store.startEvents()
  store.loadAccounts().catch(() => {})
  store.loadStats().catch(() => {})
}

watch(
  () => route.path,
  (path) => {
    drawerOpen.value = false
    if (AUTH_PAGES.includes(path)) {
      store.stopEvents()
    } else {
      bootstrap()
    }
  }
)

onMounted(async () => {
  if (!isAuthPage.value) bootstrap()
})
</script>

<template>
  <!-- 登录页 / 强制改密页：整屏独立卡片，不套管理后台外壳 -->
  <router-view v-if="isAuthPage" />

  <el-container v-else class="app-shell">
    <!-- 桌面端侧边导航 -->
    <el-aside v-if="!isMobile" width="196px" class="app-aside">
      <div class="brand">
        <el-icon :size="20"><Files /></el-icon>
        <span>mangaSync</span>
      </div>
      <el-menu :default-active="activePath" class="app-menu" @select="go">
        <el-menu-item v-for="m in MENU" :key="m.path" :index="m.path">
          <el-icon><component :is="m.icon" /></el-icon>
          <span>{{ m.title }}</span>
          <el-badge
            v-if="m.path === '/downloads' && runningCount"
            :value="runningCount"
            class="menu-badge"
          />
        </el-menu-item>
      </el-menu>
      <div class="aside-foot">
        <div class="foot-row">
          <el-icon class="foot-icon"><UserFilled /></el-icon>
          <span class="foot-user ms-ellipsis" :title="username">{{ username }}</span>
          <span class="ms-dim foot-ver">v0.1.0</span>
        </div>
        <div class="foot-row">
          <a href="#/settings">设置</a>
          <el-button
            link
            type="primary"
            size="small"
            :loading="loggingOut"
            @click="onLogout"
          >
            退出登录
          </el-button>
        </div>
      </div>
    </el-aside>

    <!-- 移动端抽屉导航 -->
    <el-drawer
      v-model="drawerOpen"
      direction="ltr"
      size="220px"
      :with-header="false"
      class="nav-drawer"
    >
      <div class="brand">
        <el-icon :size="20"><Files /></el-icon>
        <span>mangaSync</span>
      </div>
      <el-menu :default-active="activePath" class="app-menu" @select="go">
        <el-menu-item v-for="m in MENU" :key="m.path" :index="m.path">
          <el-icon><component :is="m.icon" /></el-icon>
          <span>{{ m.title }}</span>
          <el-badge
            v-if="m.path === '/downloads' && runningCount"
            :value="runningCount"
            class="menu-badge"
          />
        </el-menu-item>
      </el-menu>
      <div class="aside-foot">
        <div class="foot-row">
          <el-icon class="foot-icon"><UserFilled /></el-icon>
          <span class="foot-user ms-ellipsis" :title="username">{{ username }}</span>
          <span class="ms-dim foot-ver">v0.1.0</span>
        </div>
        <div class="foot-row">
          <a href="#/settings">设置</a>
          <el-button
            link
            type="primary"
            size="small"
            :loading="loggingOut"
            @click="onLogout"
          >
            退出登录
          </el-button>
        </div>
      </div>
    </el-drawer>

    <el-container class="app-main-wrap">
      <el-header class="app-header">
        <div class="header-left">
          <el-button
            v-if="isMobile"
            text
            :icon="'Expand'"
            @click="drawerOpen = true"
            aria-label="打开导航"
          />
          <span class="header-title">{{ pageTitle }}</span>
        </div>
        <div class="header-right">
          <el-tag :type="sseTag.type" size="small" effect="light" class="sse-tag">
            {{ sseTag.text }}
          </el-tag>
          <el-tooltip v-if="runningCount" :content="`${runningCount} 个任务进行中`">
            <el-button text @click="go('/downloads')">
              <el-icon>
                <component :is="store.jobList.some((j) => j.status === 'running') ? 'Loading' : 'Clock'" />
              </el-icon>
              <span class="hide-xs">{{ runningCount }} 进行中</span>
            </el-button>
          </el-tooltip>
          <el-button text :icon="'Refresh'" @click="store.loadStats().catch(() => {})">
            <span class="hide-xs">刷新</span>
          </el-button>
        </div>
      </el-header>

      <el-main class="app-main">
        <router-view v-slot="{ Component }">
          <keep-alive :max="3">
            <component :is="Component" />
          </keep-alive>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.app-shell {
  height: 100vh;
  overflow: hidden;
}

.app-aside {
  display: flex;
  flex-direction: column;
  background: var(--ms-panel);
  border-right: 1px solid var(--ms-border);
  padding: 0;
}

.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 18px;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 0.3px;
  color: var(--ms-text);
}

.app-menu {
  flex: 1;
  border-right: none;
  background: transparent;
}

.app-menu :deep(.el-menu-item) {
  height: 46px;
  margin: 2px 8px;
  border-radius: 8px;
}

.app-menu :deep(.el-menu-item.is-active) {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-weight: 600;
}

.menu-badge {
  margin-left: auto;
}

.aside-foot {
  padding: 10px 16px 12px;
  font-size: 12px;
  border-top: 1px solid var(--ms-border);
}

.foot-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 24px;
}

.foot-row + .foot-row {
  justify-content: space-between;
  margin-top: 2px;
}

.foot-icon {
  color: var(--ms-text-dim);
  flex-shrink: 0;
}

.foot-user {
  flex: 1;
  min-width: 0;
  color: var(--ms-text);
  font-weight: 600;
}

.foot-ver {
  flex-shrink: 0;
}

.app-main-wrap {
  overflow: hidden;
}

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  height: 56px;
  padding: 0 16px;
  background: var(--ms-panel);
  border-bottom: 1px solid var(--ms-border);
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.03);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.header-title {
  font-size: 16px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.sse-tag {
  flex-shrink: 0;
}

.app-main {
  padding: 16px;
  overflow-y: auto;
  background: var(--ms-bg);
}

/* 手机端：更紧凑的头部与内容边距，长标题省略而不是撑破 */
@media (max-width: 768px) {
  .app-header {
    height: 52px;
    padding: 0 8px;
    gap: 6px;
  }

  .app-header :deep(.el-button) {
    padding: 0 8px;
  }

  .app-main {
    padding: 10px;
  }

  .hide-xs {
    display: none;
  }
}
</style>

<style>
/* 移动端抽屉里的导航（非 scoped，抽屉挂到 body 下） */
.nav-drawer .el-drawer__body {
  padding: 0;
  display: flex;
  flex-direction: column;
  background: var(--ms-panel);
}

.nav-drawer .el-menu-item.is-active {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
</style>
