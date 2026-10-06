<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Files,
  UserFilled,
  Expand,
  Loading,
  Clock,
  Refresh
} from '@element-plus/icons-vue'
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

const AUTH_PAGES = ['/login', '/change-password']
const isAuthPage = computed(() => route.meta?.bare === true || AUTH_PAGES.includes(route.path))
const username = computed(() => auth.user?.username || '未登录')

const NAV_PATHS = [
  '/dashboard',
  '/accounts',
  '/favorites',
  '/search',
  '/downloads',
  '/library',
  '/settings'
]
const MENU = NAV_PATHS.map((path) => {
  const { meta } = router.resolve(path)
  return { path, title: meta.title || path, icon: meta.icon }
})

const activePath = computed(() => route.path)
const pageTitle = computed(() => route.meta?.title || '概览')

const runningCount = computed(
  () => store.jobList.filter((j) => j.status === 'running' || j.status === 'queued').length
)



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

function bootstrap() {
  store.startEvents()
  store.loadAccounts().catch(() => {})
  store.loadStats().catch(() => {})
}

watch(
  () => route.path,
  () => {
    drawerOpen.value = false
    if (isAuthPage.value) {
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
  <router-view v-if="isAuthPage" />

  <el-container v-else class="app-shell">
    <!-- 桌面端侧边导航 -->
    <el-aside v-if="!isMobile" width="206px" class="app-aside">
      <div class="brand">
        <div class="brand-logo-badge">
          <el-icon :size="18"><Files /></el-icon>
        </div>
        <div class="brand-text">
          <span class="brand-name">mangaSync</span>
          <span class="brand-tag">PRO</span>
        </div>
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
      size="230px"
      :with-header="false"
      class="nav-drawer"
    >
      <div class="brand">
        <div class="brand-logo-badge">
          <el-icon :size="18"><Files /></el-icon>
        </div>
        <div class="brand-text">
          <span class="brand-name">mangaSync</span>
          <span class="brand-tag">PRO</span>
        </div>
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

          <el-tooltip v-if="runningCount" :content="`${runningCount} 个任务进行中`">
            <el-button text @click="go('/downloads')">
              <el-icon>
                <component :is="store.jobList.some((j) => j.status === 'running') ? 'Loading' : 'Clock'" />
              </el-icon>
              <span class="hide-xs">{{ runningCount }} 进行中</span>
            </el-button>
          </el-tooltip>
          <el-button text :icon="'Refresh'" @click="store.triggerRefresh()">
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
  background: #ffffff;
  border-right: 1px solid var(--ms-border);
  padding: 0;
  box-shadow: 1px 0 3px rgba(15, 23, 42, 0.02);
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px 18px 14px;
}

.brand-logo-badge {
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: linear-gradient(135deg, #3b82f6 0%, #1d4ed8 100%);
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.3);
  flex-shrink: 0;
}

.brand-text {
  display: flex;
  align-items: center;
  gap: 6px;
}

.brand-name {
  font-size: 16px;
  font-weight: 700;
  letter-spacing: -0.3px;
  color: var(--ms-text);
}

.brand-tag {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  letter-spacing: 0.5px;
}

.app-menu {
  flex: 1;
  border-right: none;
  background: transparent;
  padding: 4px 0;
}

.app-menu :deep(.el-menu-item) {
  height: 44px;
  margin: 3px 10px;
  border-radius: 9px;
  font-weight: 500;
  transition: all 0.18s ease;
}

.app-menu :deep(.el-menu-item:hover) {
  background: var(--ms-bg-soft);
  color: var(--el-color-primary);
}

.app-menu :deep(.el-menu-item.is-active) {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(37, 99, 235, 0.06);
}

.menu-badge {
  margin-left: auto;
}

.aside-foot {
  padding: 12px 16px 14px;
  font-size: 12px;
  border-top: 1px solid var(--ms-border);
  background: #fafbfc;
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
  font-size: 11px;
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
  padding: 0 20px;
  background: rgba(255, 255, 255, 0.88);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
  border-bottom: 1px solid var(--ms-border);
  position: relative;
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.header-title {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.sse-tag {
  flex-shrink: 0;
  border-radius: 6px;
}

.app-main {
  padding: 16px 20px 24px;
  overflow-y: auto;
  background: var(--ms-bg);
}

@media (max-width: 768px) {
  .app-header {
    height: 52px;
    padding: 0 10px;
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
.nav-drawer .el-drawer__body {
  padding: 0;
  display: flex;
  flex-direction: column;
  background: #ffffff;
}

.nav-drawer .el-menu-item.is-active {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  font-weight: 600;
}
</style>