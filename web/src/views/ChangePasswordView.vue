<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { authErrorMessage } from '@/api'
import { auth } from '@/store/auth'
import { changePassword, logout } from '@/composables/useAuth'

const router = useRouter()

/** 是「首次登录强制改密」还是用户主动来改密 */
const forced = computed(() => !!auth.user?.mustChangePassword)
const username = computed(() => auth.user?.username || '')

const formRef = ref(null)
const saving = ref(false)
const loggingOut = ref(false)

const form = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const rules = {
  oldPassword: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '新密码至少 6 位', trigger: 'blur' },
    {
      validator(rule, value, cb) {
        if (value && value === form.oldPassword) cb(new Error('新密码不能与原密码相同'))
        else cb()
      },
      trigger: 'blur'
    }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      validator(rule, value, cb) {
        if (value !== form.newPassword) cb(new Error('两次输入的密码不一致'))
        else cb()
      },
      trigger: ['blur', 'change']
    }
  ]
}

async function submit() {
  if (saving.value || !formRef.value) return

  let valid = true
  await formRef.value.validate((ok) => {
    valid = ok
  })
  if (!valid) return

  saving.value = true
  try {
    await changePassword({
      oldPassword: form.oldPassword,
      newPassword: form.newPassword,
      silent: true
    })
    ElMessage.success('密码修改成功，请牢记新密码')
    form.oldPassword = form.newPassword = form.confirmPassword = ''
    router.replace('/dashboard')
  } catch (e) {
    ElMessage.error(authErrorMessage(e))
  } finally {
    saving.value = false
  }
}

async function onLogout() {
  loggingOut.value = true
  try {
    await logout()
  } finally {
    loggingOut.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <el-card class="auth-card" shadow="never">
      <div class="auth-brand">
        <el-icon :size="24" class="auth-logo"><Lock /></el-icon>
        <div class="auth-brand-text">
          <div class="auth-title">修改密码</div>
          <div class="auth-sub ms-dim">
            {{ username ? `当前账号：${username}` : 'mangaSync 管理后台' }}
          </div>
        </div>
      </div>

      <el-alert
        v-if="forced"
        title="首次登录必须修改初始密码，改密后才能使用其它功能。"
        type="warning"
        :closable="false"
        show-icon
        class="auth-error"
      />

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        class="auth-form"
        @submit.prevent="submit"
      >
        <el-form-item label="原密码" prop="oldPassword">
          <el-input
            v-model="form.oldPassword"
            type="password"
            size="large"
            show-password
            autocomplete="current-password"
            placeholder="请输入原密码"
            @keyup.enter="submit"
          />
        </el-form-item>

        <el-form-item label="新密码" prop="newPassword">
          <el-input
            v-model="form.newPassword"
            type="password"
            size="large"
            show-password
            autocomplete="new-password"
            placeholder="至少 6 位，且不能与原密码相同"
            @keyup.enter="submit"
          />
        </el-form-item>

        <el-form-item label="确认新密码" prop="confirmPassword">
          <el-input
            v-model="form.confirmPassword"
            type="password"
            size="large"
            show-password
            autocomplete="new-password"
            placeholder="请再次输入新密码"
            @keyup.enter="submit"
          />
        </el-form-item>

        <el-button
          type="primary"
          size="large"
          class="auth-submit"
          :loading="saving"
          @click="submit"
        >
          {{ saving ? '提交中…' : '确认修改' }}
        </el-button>
      </el-form>

      <div class="auth-foot">
        <el-button link type="info" :loading="loggingOut" @click="onLogout">
          <el-icon><SwitchButton /></el-icon>
          <span>退出登录</span>
        </el-button>
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
  max-width: 400px;
  background: var(--ms-panel);
  border: 1px solid var(--ms-border);
  border-radius: 14px;
}

.auth-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}

.auth-logo {
  color: var(--el-color-warning);
}

.auth-title {
  font-size: 18px;
  font-weight: 700;
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
  margin-bottom: 14px;
}

.auth-submit {
  width: 100%;
}

.auth-foot {
  margin-top: 16px;
  display: flex;
  justify-content: center;
}

.auth-foot :deep(.el-button span) {
  margin-left: 4px;
}
</style>
