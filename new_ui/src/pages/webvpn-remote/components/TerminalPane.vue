<script setup lang="ts">
// 终端：xterm + WebSocket（二进制帧 stdin/stdout，文本帧控制）；自适应尺寸、心跳、按 close code 断线重连
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton } from 'naive-ui'
import { Terminal, type ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { RemoteError, remoteApi } from '../api'

export type TermState = 'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'exited' | 'ended'

const props = defineProps<{ sid: string; ticket: string; clipboard: boolean; dark: boolean }>()
const emit = defineEmits<{
  state: [s: TermState]
  // 会话已在服务端结束（被踢 / 空闲 / 目标断开 / 用户结束 / 服务重启）
  ended: [reason: string, message: string]
}>()

const el = ref<HTMLDivElement>()
const state = ref<TermState>('connecting')
const overlay = ref<{ title: string; desc: string; action?: 'reconnect' | 'reopen' } | null>(null)
const retryIn = ref(0)

let term: Terminal | null = null
let fit: FitAddon | null = null
let ws: WebSocket | null = null
let ro: ResizeObserver | null = null
let pingTimer = 0
let retryTimer = 0
let fitTimer = 0
let attempts = 0
let disposed = false
let lastReason = ''
const enc = new TextEncoder()

function setState(s: TermState) {
  state.value = s
  emit('state', s)
}

// 终端配色跟随主题；ANSI 16 色使用 xterm 默认，浅色主题下调整亮色以保证可读
function themeOf(dark: boolean): ITheme {
  const css = getComputedStyle(document.documentElement)
  const v = (n: string) => css.getPropertyValue(n).trim()
  const base: ITheme = {
    background: v('--color-primary'),
    foreground: v('--color-text-primary'),
    cursor: v('--color-text-primary'),
    cursorAccent: v('--color-primary'),
    selectionBackground: v('--color-action-selection'),
  }
  if (dark) return base
  // 浅色背景下 xterm 默认的白 / 黄等前景几乎不可见（终端调色板为功能性颜色，非界面配色）
  return {
    ...base,
    black: '#1f2328', red: '#cf222e', green: '#116329', yellow: '#7d4e00', blue: '#0969da', magenta: '#8250df', cyan: '#1b7c83', white: '#6e7781',
    brightBlack: '#57606a', brightRed: '#a40e26', brightGreen: '#1a7f37', brightYellow: '#633c01', brightBlue: '#218bff', brightMagenta: '#a475f9', brightCyan: '#3192aa', brightWhite: '#8c959f',
  }
}

function send(obj: any) {
  if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj))
}

function doFit() {
  if (!term || !fit || !el.value || !el.value.offsetWidth || !el.value.offsetHeight) return
  try { fit.fit() } catch { /* 容器不可见 */ }
}
function scheduleFit() {
  clearTimeout(fitTimer)
  fitTimer = window.setTimeout(doFit, 60)
}

function open(ticket: string) {
  clearTimeout(retryTimer)
  overlay.value = null
  lastReason = ''
  const cols = term?.cols || 80
  const rows = term?.rows || 24
  const sock = new WebSocket(remoteApi.termUrl(ticket, cols, rows))
  sock.binaryType = 'arraybuffer'
  ws = sock
  sock.onopen = () => {
    clearInterval(pingTimer)
    pingTimer = window.setInterval(() => send({ type: 'ping' }), 30000)
  }
  sock.onmessage = ev => {
    if (ev.data instanceof ArrayBuffer) {
      term?.write(new Uint8Array(ev.data))
      return
    }
    let m: any
    try { m = JSON.parse(ev.data) } catch { return }
    switch (m.type) {
      case 'ready':
        attempts = 0
        setState('connected')
        // 服务端按 URL 尺寸建 PTY，再同步一次以防期间窗口变化
        if (term && (term.cols !== m.cols || term.rows !== m.rows)) send({ type: 'resize', cols: term.cols, rows: term.rows })
        term?.focus()
        break
      case 'closed':
        lastReason = m.reason || ''
        break
      case 'exit':
        lastReason = 'exit:' + (m.code ?? 0)
        break
      case 'error':
        term?.write(`\r\n\x1b[31m${m.message || '终端错误'}\x1b[0m\r\n`)
        break
    }
  }
  sock.onclose = ev => {
    if (ws !== sock) return
    clearInterval(pingTimer)
    ws = null
    if (disposed) return
    handleClose(ev.code, ev.reason)
  }
}

const reasonText: Record<string, string> = {
  kicked: '管理员已断开此会话',
  revoked: '你的访问权限已被吊销',
  unauthorized: '你已无权访问该应用',
  app_changed: '应用配置已变更，会话已结束',
  idle: '会话空闲超时，已自动断开',
  remote_closed: '目标主机已断开连接',
  user_close: '会话已结束',
  shutdown: '服务正在重启，会话已结束',
}

function handleClose(code: number, reason: string) {
  const r = lastReason.startsWith('exit:') ? '' : (lastReason || reason)
  switch (code) {
    case 4000: { // shell 正常退出：SSH 连接仍在，可新开一个 shell
      const exitCode = lastReason.startsWith('exit:') ? lastReason.slice(5) : '0'
      term?.write(`\r\n\x1b[2m[进程已退出，代码 ${exitCode}]\x1b[0m\r\n`)
      setState('exited')
      overlay.value = { title: 'Shell 已退出', desc: `退出代码 ${exitCode}。可以重新打开一个终端，文件功能不受影响。`, action: 'reopen' }
      return
    }
    case 4001:
    case 4002:
    case 4003:
    case 4005:
    case 1001: {
      const msg = reasonText[r] || (code === 4001 ? '会话已被管理员断开' : code === 4002 ? reasonText.idle : code === 4003 ? reasonText.remote_closed : code === 1001 ? reasonText.shutdown : '会话已结束')
      term?.write(`\r\n\x1b[33m${msg}\x1b[0m\r\n`)
      setState('ended')
      emit('ended', r || String(code), msg)
      return
    }
  }
  // 其余（网络中断 1006、错误 4004 等）：凭新票据自动重连，会话仍在服务端保留
  scheduleReconnect()
}

function scheduleReconnect() {
  if (attempts >= 6) {
    setState('disconnected')
    overlay.value = { title: '终端连接已断开', desc: '多次重连失败，请检查网络后重试。', action: 'reconnect' }
    return
  }
  const delay = [1, 2, 3, 5, 8, 13][attempts] || 13
  attempts++
  setState('reconnecting')
  retryIn.value = delay
  const tick = () => {
    retryIn.value--
    if (retryIn.value <= 0) reconnect()
    else retryTimer = window.setTimeout(tick, 1000)
  }
  retryTimer = window.setTimeout(tick, 1000)
}

async function reconnect() {
  clearTimeout(retryTimer)
  overlay.value = null
  setState(state.value === 'exited' ? 'connecting' : 'reconnecting')
  try {
    const t = await remoteApi.ticket(props.sid)
    open(t.ticket)
  } catch (e) {
    if (e instanceof RemoteError && (e.code === 'session_not_found' || e.status === 401 || e.status === 403)) {
      const msg = e.code === 'session_not_found' ? '会话已在服务端结束' : e.message
      setState('ended')
      emit('ended', e.code, msg)
      return
    }
    scheduleReconnect()
  }
}

// 剪贴板禁用：拦截复制 / 剪切 / 粘贴（SSH 终端剪贴板策略由前端执行）
function blockClipboard(e: Event) {
  if (props.clipboard) return
  e.preventDefault()
  e.stopImmediatePropagation()
}

onMounted(async () => {
  term = new Terminal({
    fontFamily: 'v-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
    fontSize: 13,
    lineHeight: 1.15,
    cursorBlink: true,
    scrollback: 5000,
    allowProposedApi: false,
    theme: themeOf(props.dark),
    rightClickSelectsWord: props.clipboard,
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.loadAddon(new WebLinksAddon())
  term.open(el.value!)
  term.onData(d => { if (ws?.readyState === WebSocket.OPEN) ws.send(enc.encode(d)) })
  term.onBinary(d => {
    if (ws?.readyState !== WebSocket.OPEN) return
    const buf = new Uint8Array(d.length)
    for (let i = 0; i < d.length; i++) buf[i] = d.charCodeAt(i) & 0xff
    ws.send(buf)
  })
  term.onResize(({ cols, rows }) => send({ type: 'resize', cols, rows }))
  // Ctrl+Shift+C / V 复制粘贴；禁用剪贴板时一并拦截
  term.attachCustomKeyEventHandler(ev => {
    if (ev.type !== 'keydown') return true
    const k = ev.key.toLowerCase()
    const mod = ev.ctrlKey || ev.metaKey
    if (mod && ev.shiftKey && (k === 'c' || k === 'v')) {
      if (props.clipboard) {
        if (k === 'c') { const s = term!.getSelection(); if (s) navigator.clipboard?.writeText(s).catch(() => {}) }
        else navigator.clipboard?.readText().then(t => term!.paste(t)).catch(() => {})
      }
      ev.preventDefault()
      return false
    }
    // 禁用剪贴板时，Ctrl/Cmd+V 不触发浏览器粘贴，但仍作为控制字符发送给终端
    if (!props.clipboard && ev.metaKey && (k === 'v' || k === 'c')) { ev.preventDefault(); return false }
    return true
  })
  for (const t of ['copy', 'cut', 'paste', 'drop']) el.value!.addEventListener(t, blockClipboard, true)
  el.value!.addEventListener('contextmenu', blockClipboard, true)

  try { await document.fonts?.ready } catch { /* 忽略 */ }
  doFit()
  ro = new ResizeObserver(scheduleFit)
  ro.observe(el.value!)
  setState('connecting')
  open(props.ticket)
})

watch(() => props.dark, d => { if (term) term.options.theme = themeOf(d) })

onBeforeUnmount(() => {
  disposed = true
  clearInterval(pingTimer)
  clearTimeout(retryTimer)
  clearTimeout(fitTimer)
  ro?.disconnect()
  try { ws?.close(1000) } catch { /* 忽略 */ }
  term?.dispose()
})

defineExpose({ focus: () => term?.focus(), fit: doFit, reconnect })
</script>

<template>
  <div class="term-wrap" :class="!clipboard && 'term-wrap--noclip'">
    <div ref="el" class="term-host" />
    <div v-if="state === 'reconnecting'" class="term-banner">
      <span class="ax-badge text-warning">连接中断，{{ retryIn > 0 ? `${retryIn} 秒后` : '正在' }}重连…</span>
      <NButton size="tiny" secondary @click="reconnect">立即重连</NButton>
    </div>
    <div v-if="overlay" class="term-overlay">
      <div class="max-w-360px text-center">
        <div class="text-lg font-semibold">{{ overlay.title }}</div>
        <div class="mt-4px text-md text-secondary">{{ overlay.desc }}</div>
        <NButton class="mt-16px" type="primary" @click="reconnect">{{ overlay.action === 'reopen' ? '新开终端' : '重新连接' }}</NButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.term-wrap { position: relative; height: 100%; min-height: 0; background: var(--color-primary); }
.term-host { position: absolute; inset: 8px 4px 4px 12px; }
.term-host :deep(.xterm-viewport) { background: transparent !important; }
.term-wrap--noclip :deep(.xterm-screen) { user-select: none; }
.term-banner {
  position: absolute; top: 10px; left: 50%; transform: translateX(-50%); display: flex; gap: 8px; align-items: center;
  padding: 6px 10px; border-radius: 10px; background: var(--color-primary); border: 1px solid var(--color-border-default);
  box-shadow: 0 4px 16px var(--color-overlay); z-index: 2;
}
.term-overlay {
  position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; z-index: 3;
  background: color-mix(in srgb, var(--color-primary) 82%, transparent); backdrop-filter: blur(2px);
}
</style>
