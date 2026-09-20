import { ElMessage } from 'element-plus'
import api from '@/api'
import router, { resetMeCache } from '@/router'
import { clearSession, setMustChangePassword } from '@/store/auth'

/**
 * 退出登录：调后端删除会话 → 清本地登录态 → 回登录页。
 * 后端不可达时也照常本地登出（避免用户被卡住）。
 */
export async function logout({ silent = false } = {}) {
  try {
    await api.logout()
  } catch (_) {
    /* 未登录 / 网络异常都按已登出处理 */
  }
  // 清掉 me 的 TTL 缓存，否则重新登录前的 60s 内可能拿旧结果放行
  resetMeCache()
  clearSession()
  if (router.currentRoute.value.path !== '/login') {
    await router.replace('/login')
  }
  if (!silent) ElMessage.success('已退出登录')
}

/**
 * 修改密码的公共实现（/change-password 页与设置页内嵌表单共用）。
 * @returns {Promise<boolean>} 成功与否
 */
export async function changePassword({ oldPassword, newPassword, silent = false } = {}) {
  const res = await api.changePassword(oldPassword, newPassword)
  // 新密码生效后不再强制改密
  setMustChangePassword(false)
  if (!silent) ElMessage.success('密码已修改')
  return res || { ok: true }
}
