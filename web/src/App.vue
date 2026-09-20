<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/store/app'

const route = useRoute()
const router = useRouter()
const store = useAppStore()

const isMobile = ref(false)
const drawerOpen = ref(false)

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

function syncViewport() {
  isMobile.value = window.innerWidth < 768
  if (!isMobile.value) drawerOpen.value = false
}

function go(path) {
  router.push(path)
  drawerOpen.value = false
}

watch(
  () => route.path,
  () => {
    drawerOpen.value = false
  }
)

onMounted(async () => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
  store.startEvents()
  store.loadAccounts().catch(() => {})
  store.loadStats().catch(() => {})
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewport)
})
</script>

<template>
  <el-container class="app-shell">
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
      <div class="aside-foot ms-dim">
        <div>v0.1.0</div>
        <a :href="'#/settings'">设置</a>
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
          <el-tag :type="sseTag.type" size="small" effect="dark" class="sse-tag">
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
  background: #171a20;
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
  color: #fff;
}

.app-menu {
  flex: 1;
  border-right: none;
  background: transparent;
}

.app-menu :deep(.el-menu-item) {
  height: 46px;
}

.app-menu :deep(.el-menu-item.is-active) {
  background: rgba(64, 158, 255, 0.14);
  border-right: 3px solid var(--el-color-primary);
}

.menu-badge {
  margin-left: auto;
}

.aside-foot {
  padding: 12px 18px;
  font-size: 12px;
  display: flex;
  justify-content: space-between;
  border-top: 1px solid var(--ms-border);
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
  background: #171a20;
  border-bottom: 1px solid var(--ms-border);
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

@media (max-width: 640px) {
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
  background: #171a20;
}
</style>
