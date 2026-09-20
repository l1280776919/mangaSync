<script setup>
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api, { authErrorMessage } from '@/api'
import { setSession } from '@/store/auth'

const router = useRouter()
const route = useRoute()

const formRef = ref(null)
const loading = ref(false)
const errorMsg = ref('')

const form = reactive({
  username: '',
  password: ''
})

const rules = {
  username: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

/** 登录成功后的落点：欠改密先去改密页，否则回跳来源页 */
function afterLoginTarget(user) {
  if (user?.mustChangePassword) return { path: '/change-password' }
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  if (redirect && redirect !== '/' && !redirect.startsWith('/login')) return redirect
  return { path: '/dashboard' }
}

async function submit() {
  errorMsg.value = ''
  if (loading.value || !formRef.value) return

  let valid = true
  await formRef.value.validate((ok) => {
    valid = ok
  })
  if (!valid) return

  loading.value = true
  try {
    const username = form.username.trim()
    const user = await api.login(username, form.password)
    setSession(user)
    ElMessage.success(`欢迎回来，${user?.username || username}`)
    // 登录后不再回填密码
    form.password = ''
    router.replace(afterLoginTarget(user))
  } catch (e) {
    // 401 / 429 分别给一句人话
    if (e?.status === 429) {
      const wait = e.retryAfter ? `请 ${e.retryAfter} 秒后再试` : '请稍后再试'
      errorMsg.value = `登录尝试过于频繁，${wait}`
    } else if (e?.status === 401) {
      errorMsg.value = '账号或密码错误'
    } else {
      errorMsg.value = authErrorMessage(e)
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <el-card class="auth-card" shadow="never">
      <div class="auth-brand">
        <el-icon :size="26" class="auth-logo"><Files /></el-icon>
        <div class="auth-brand-text">
          <div class="auth-title">mangaSync</div>
          <div class="auth-sub ms-dim">漫画同步管理后台</div>
        </div>
      </div>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        class="auth-form"
        @submit.prevent="submit"
      >
        <el-form-item label="账号" prop="username">
          <el-input
            v-model="form.username"
            size="large"
            placeholder="admin"
            autocomplete="username"
            clearable
            @keyup.enter="submit"
          >
            <template #prefix><el-icon><User /></el-icon></template>
          </el-input>
        </el-form-item>

        <el-form-item label="密码" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            size="large"
            show-password
            placeholder="请输入密码"
            autocomplete="current-password"
            @keyup.enter="submit"
          >
            <template #prefix><el-icon><Lock /></el-icon></template>
          </el-input>
        </el-form-item>

        <el-alert
          v-if="errorMsg"
          :title="errorMsg"
          type="error"
          :closable="false"
          show-icon
          class="auth-error"
        />

        <el-button
          type="primary"
          size="large"
          class="auth-submit"
          :loading="loading"
          @click="submit"
        >
          {{ loading ? '登录中…' : '登 录' }}
        </el-button>
      </el-form>

      <div class="auth-tip ms-dim">
        初始账号 admin / admin999，登录后请立即修改密码
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.auth-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  background: radial-gradient(circle at 50% 0%, #1d2331 0%, var(--ms-bg) 60%);
}

.auth-card {
  width: 100%;
  max-width: 380px;
  background: var(--ms-panel);
  border: 1px solid var(--ms-border);
  border-radius: 14px;
}

.auth-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
}

.auth-logo {
  color: var(--el-color-primary);
}

.auth-title {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.4px;
}

.auth-sub {
  font-size: 12px;
  margin-top: 2px;
}

.auth-form :deep(.el-form-item__label) {
  padding-bottom: 2px;
  font-size: 13px;
  color: var(--ms-text-dim);
}

.auth-error {
  margin-bottom: 12px;
}

.auth-submit {
  width: 100%;
}

.auth-tip {
  margin-top: 18px;
  font-size: 12px;
  line-height: 1.6;
  text-align: center;
}
</style>
