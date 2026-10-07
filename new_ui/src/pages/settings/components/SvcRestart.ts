// 服务重启：发送重启指令 / 轮询等待服务恢复后刷新页面（服务配置、数据库切换共用）
import { onBeforeUnmount, ref } from 'vue'
import { http, post, errMsg } from '@/api/http'
import { ui, confirm } from '@/utils/ui'

export function useServiceRestart() {
  const restarting = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null

  const stop = () => { if (timer) { clearInterval(timer); timer = null } }

  // 每 2 秒探测一次，恢复即刷新；最多等待 2 分钟
  function waitForRestart() {
    let retries = 0
    restarting.value = true
    stop()
    timer = setInterval(async () => {
      retries++
      try {
        await http.get('/api/v1/set/soft/status', { timeout: 3000 })
        stop()
        location.reload()
      } catch {
        if (retries >= 60) {
          stop()
          restarting.value = false
          ui.message.warning('服务恢复超时，请稍后手动刷新页面')
        }
      }
    }, 2000)
  }

  async function restartService() {
    if (!(await confirm('服务将立即重启，所有在线 VPN 连接会短暂中断，重启完成后页面自动刷新。', '重启服务'))) return
    restarting.value = true
    try {
      await post('/api/v1/set/restart')
      ui.message.success('重启指令已发送，等待服务恢复…')
      waitForRestart()
    } catch (e) {
      restarting.value = false
      ui.message.error(errMsg(e, '重启失败'))
    }
  }

  onBeforeUnmount(stop)
  return { restarting, restartService, waitForRestart }
}

// 系统告警（/api/v1/set/soft/status）：字符串或 {code, level, message, action_path}
export type SysWarning = string | { code?: string; level?: string; message?: string; action_path?: string }
export const warnMsg = (w: SysWarning) => (typeof w === 'string' ? w : w?.message || '')
export const warnCode = (w: SysWarning) => (typeof w === 'string' ? '' : w?.code || '')
export const warnPath = (w: SysWarning) => (typeof w === 'string' ? '' : w?.action_path || '')
export function warnLevel(w: SysWarning): 'error' | 'warning' | 'info' | 'success' {
  const l = typeof w === 'string' ? 'error' : w?.level || 'error'
  return (['error', 'warning', 'info', 'success'].includes(l) ? l : 'error') as any
}
