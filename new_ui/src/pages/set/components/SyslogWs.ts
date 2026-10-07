// 系统日志 WebSocket 实时推送：断线指数退避重连（3s 起，×1.5，上限 30s）
import { onBeforeUnmount, ref } from 'vue'

export interface SyslogEntry { level: string; time: string; msg: string }

export function useSyslogWs(onEntry: (e: SyslogEntry) => void) {
  const live = ref(true)
  const connected = ref(false)
  let ws: WebSocket | null = null
  let timer: ReturnType<typeof setTimeout> | null = null
  let delay = 3000

  function connect() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    try {
      ws = new WebSocket(`${proto}//${location.host}/api/v1/set/syslog/ws`)
    } catch {
      return scheduleReconnect()
    }
    ws.onopen = () => {
      connected.value = true
      delay = 3000
    }
    ws.onmessage = ev => {
      if (!live.value) return
      try {
        const msg = JSON.parse(ev.data)
        if (msg?.type === 'syslog' && msg.data) onEntry(msg.data)
      } catch { /* 忽略无法解析的消息 */ }
    }
    ws.onclose = () => {
      connected.value = false
      ws = null
      scheduleReconnect()
    }
  }

  function disconnect() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
    if (ws) {
      ws.onclose = null
      ws.close()
      ws = null
    }
    connected.value = false
  }

  function scheduleReconnect() {
    if (timer) return
    timer = setTimeout(() => {
      timer = null
      if (live.value) connect()
    }, delay)
    delay = Math.min(delay * 1.5, 30000)
  }

  // 暂停 = 断开连接；继续 = 重连
  function setLive(v: boolean) {
    live.value = v
    if (v) connect()
    else disconnect()
  }

  onBeforeUnmount(disconnect)
  return { live, connected, connect, disconnect, setLive }
}
