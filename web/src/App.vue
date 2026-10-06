<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import api from '@/api'
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

const build = ref(null)
api.health().then(v => { build.value = v }).catch(() => {})
const route = useRoute()
const router = useRouter()
const store = useAppStore()

const isMobile = useIsMobile()
const drawerOpen = ref(false)
const loggingOut = ref(false)

const AUTH_PAGES = ['/login', '/change-password']
const isAuthPage = computed(() => route.meta?.bare === true || AUTH_PAGES.includes(route.path))
const username = computed(() => auth.user?.username || '未登录')

const NAV_PATHS = ['/dashboard', '/library', '/favorites', '/search', '/downloads', '/accounts', '/settings']
const PAGE_DESCRIPTIONS = {
 '/dashboard': '从上次停下的地方继续，或发现下一本想读的漫画。',
 '/library': '整理本地作品，随时开始阅读。',
 '/favorites': '把喜欢的作品留在这里，同步与下载随你安排。',
 '/search': '在不同漫画源中，找到你想读的作品。',
 '/downloads': '查看同步与下载进度，处理需要重试的任务。',
 '/accounts': '管理漫画源账号与收藏同步。',
 '/settings': '按你的设备和阅读习惯调整应用。'
}
const mobilePaths = ['/library', '/favorites', '/search', '/downloads']
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
    <el-aside v-if="!isMobile" width="224px" class="app-aside">
      <div class="brand">
        <div class="brand-logo-badge">
          <el-icon :size="18"><Files /></el-icon>
        </div>
        <div class="brand-text">
          <span class="brand-name">mangaSync</span>
          <span class="brand-tag">个人书房</span>
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
          <span class="ms-dim foot-ver">{{ build?.version || '—' }} · {{ build?.commit?.slice(0, 7) || 'dev' }}</span>
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
          <span class="brand-tag">个人书房</span>
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
          <span class="ms-dim foot-ver">{{ build?.version || '—' }} · {{ build?.commit?.slice(0, 7) || 'dev' }}</span>
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
          <span class="header-context">mangaSync</span><span class="header-slash">/</span><span class="header-title">{{ pageTitle }}</span>
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
          <el-button text aria-label="刷新当前页面" :icon="'Refresh'" @click="store.triggerRefresh()">
            <span class="hide-xs">刷新</span>
          </el-button>
        </div>
      </el-header>

      <el-main class="app-main">
        <div class="page-content">
        <header class="page-intro">
          <div><h1>{{ route.path === '/dashboard' ? '你的漫画书房' : pageTitle }}</h1><p>{{ PAGE_DESCRIPTIONS[route.path] }}</p></div>
        </header>
        <router-view v-slot="{ Component }">
          <keep-alive :max="3">
            <component :is="Component" />
          </keep-alive>
        </router-view>
        </div>
      </el-main>
    </el-container>
    <nav v-if="isMobile" class="mobile-nav" aria-label="常用页面">
      <button v-for="m in MENU.filter(m => mobilePaths.includes(m.path))" :key="m.path" :class="{ active: activePath === m.path }" :aria-current="activePath === m.path ? 'page' : undefined" :aria-label="'前往' + m.title" @click="go(m.path)">
        <el-icon :size="21"><component :is="m.icon" /></el-icon><span>{{ m.title }}</span>
        <span v-if="m.path === '/downloads' && runningCount" class="nav-dot" aria-label="有进行中的任务"></span>
      </button>
    </nav>
  </el-container>
</template>

<style scoped>
.app-shell {
  height: 100dvh;
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
  padding: 28px 22px 26px;
}

.brand-logo-badge {
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: #426a99;
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: none;
  flex-shrink: 0;
}

.brand-text {
  display: flex;
  align-items: flex-start;
  flex-direction: column;
  gap: 3px;
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
  padding: 0;
  border-radius: 4px;
  color: var(--ms-text-dim);
  letter-spacing: 1px;
  font-weight: 400;
}

.app-menu {
  flex: 1;
  border-right: none;
  background: transparent;
  padding: 4px 0;
}

.app-menu :deep(.el-menu-item) {
  height: 44px;
  margin: 5px 14px;
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

.menu-badge { margin-left: auto; display: inline-flex; align-items: center; }
.menu-badge :deep(.el-badge__content) { position: static; transform: none; }

.aside-foot {
  padding: 12px 16px 14px;
  font-size: 12px;
  border-top: 1px solid var(--ms-border);
  background: transparent;
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
  height: 64px;
  padding: 0 32px;
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
  padding: 28px 32px 40px;
  overflow-y: auto;
  background: var(--ms-bg);
}

@media (max-width: 768px) {
  .app-header {
    height: calc(56px + env(safe-area-inset-top));
    padding-top: env(safe-area-inset-top);
    padding: 0 10px;
    gap: 6px;
  }

  .app-header :deep(.el-button) {
    padding: 0 8px;
  }

  .app-main {
    padding: 20px 14px calc(88px + env(safe-area-inset-bottom));
  }

  .hide-xs {
    display: none;
  }
}

.page-content { width: 100%; max-width: 1480px; margin: 0 auto; }
.page-intro { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin-bottom: 28px; }
.page-intro h1 { margin: 0 0 8px; font-size: 28px; font-weight: 650; letter-spacing: -.8px; line-height: 1.3; }
.page-intro p { margin: 0; color: var(--ms-text-dim); font-size: 13px; line-height: 1.7; }
.page-intro-mark { font-size: 10px; letter-spacing: 2px; color: #8794a1; white-space: nowrap; }
.header-context { font-size: 12px; color: var(--ms-text-dim); }
.header-slash { color: var(--ms-border-strong); padding: 0 6px; }
.mobile-nav { position: fixed; bottom: 0; left: 0; right: 0; height: calc(66px + env(safe-area-inset-bottom)); padding: 4px 12px calc(4px + env(safe-area-inset-bottom)); display: grid; grid-template-columns: repeat(4,1fr); background: var(--ms-overlay); border-top: 1px solid var(--ms-border); backdrop-filter: blur(16px); z-index: 1600; }
.mobile-nav button { position: relative; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 5px; border: 0; border-radius: 12px; background: transparent; color: var(--ms-text-dim); font: inherit; font-size: 11px; cursor: pointer; }
.mobile-nav button.active { color: var(--el-color-primary); background: var(--el-color-primary-light-9); font-weight: 600; }
.nav-dot { position: absolute; width: 5px; height: 5px; background: var(--el-color-primary); top: 5px; right: calc(50% - 16px); border-radius: 50%; }
@media (max-width: 768px) { .page-intro { margin-bottom: 20px; } .page-intro h1 { font-size: 24px; } .page-intro-mark, .header-context, .header-slash { display: none; } }
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
