<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Odometer, User, Setting, ArrowRight, UserFilled } from '@element-plus/icons-vue'
import { auth } from '@/store/auth'
import { logout } from '@/composables/useAuth'
const router = useRouter()
const busy = ref(false)
const entries = [
  { path: '/dashboard', title: '概览', description: '继续阅读与书房动态', icon: Odometer },
  { path: '/accounts', title: '漫画源账号', description: '登录账号、同步收藏', icon: User },
  { path: '/settings', title: '设置', description: '下载、存储与阅读偏好', icon: Setting }
]
async function signOut() {
  if (busy.value) return
  busy.value = true
  try { await logout() } finally { busy.value = false }
}
</script>
<template>
  <section class="more-panel">
    <div class="more-profile"><el-icon :size="24"><UserFilled /></el-icon><div><strong>{{ auth.user?.username }}</strong><p>你的个人书房</p></div></div>
    <button v-for="entry in entries" :key="entry.path" class="more-entry" @click="router.push(entry.path)">
      <el-icon :size="22"><component :is="entry.icon" /></el-icon>
      <span><strong>{{ entry.title }}</strong><small>{{ entry.description }}</small></span>
      <el-icon><ArrowRight /></el-icon>
    </button>
    <el-button class="logout" :loading="busy" @click="signOut">退出登录</el-button>
  </section>
</template>
<style scoped>
.more-panel { max-width: 680px; margin: 0 auto; }
.more-profile { display: flex; align-items: center; gap: 16px; padding: 8px 8px 28px; color: var(--el-color-primary); }
.more-profile strong { color: var(--ms-text); font-size: 18px; }
.more-profile p { margin: 6px 0 0; font-size: 12px; color: var(--ms-text-dim); }
.more-entry { display: flex; align-items: center; width: 100%; gap: 16px; padding: 20px; margin-bottom: 12px; background: var(--ms-bg-soft); border: 1px solid var(--ms-border); border-radius: 12px; color: var(--ms-text); text-align: left; font: inherit; cursor: pointer; }
.more-entry span { flex: 1; }
.more-entry strong { display: block; font-size: 14px; font-weight: 600; }
.more-entry small { display: block; margin-top: 6px; color: var(--ms-text-dim); }
.more-entry:hover { border-color: var(--el-color-primary); }
.logout { width: 100%; min-height: 44px; margin-top: 18px; }
</style>
