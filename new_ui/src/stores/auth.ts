import { defineStore } from 'pinia'
import { ref } from 'vue'
import { http } from '@/api/http'

// 登录态仅存内存；token 在 HttpOnly Cookie 中由浏览器管理
const CHECK_INTERVAL = 5 * 60 * 1000

export const useAuthStore = defineStore('auth', () => {
  const user = ref<string | null>(null)
  const checked = ref(false)
  const showFakeDNS = ref(false)
  // 最近一次 check 是否「明确未授权」；网络错误不算，避免抖动误踢
  const expired = ref(false)
  const loggingOut = ref(false)
  let lastCheckTime = 0
  const lastLoginTime = ref(0)

  function setUser(name: string) {
    user.value = name
    checked.value = true
    lastCheckTime = lastLoginTime.value = Date.now()
    loggingOut.value = false
  }

  const shouldRecheck = () => !checked.value || Date.now() - lastCheckTime > CHECK_INTERVAL

  async function check() {
    try {
      const { data } = await http.get('/api/v1/base/auth_check')
      if (data?.code === 0 && data.data) {
        user.value = data.data.admin_user
        checked.value = true
        expired.value = false
        lastCheckTime = Date.now()
        http.get('/api/v1/set/home').then(r => { showFakeDNS.value = !!r.data?.data?.show_fakedns }).catch(() => {})
        return true
      }
      user.value = null
      checked.value = false
      expired.value = true
      return false
    } catch (e: any) {
      if (e.response?.status === 401) {
        user.value = null
        checked.value = false
        expired.value = true
      } else {
        expired.value = false
      }
      return false
    }
  }

  async function logout() {
    try { await http.post('/api/v1/base/logout') } catch { /* 本地状态仍清除 */ }
    user.value = null
    checked.value = false
    expired.value = false
    lastCheckTime = 0
  }

  return {
    user, checked, showFakeDNS, expired, loggingOut, lastLoginTime,
    setUser, shouldRecheck, check, logout,
  }
})
