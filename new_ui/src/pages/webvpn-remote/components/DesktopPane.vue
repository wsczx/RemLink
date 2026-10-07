<script setup lang="ts">
// 远程桌面（RDP/VNC）：guacamole-common-js Client + WebSocketTunnel（/webvpn/ws/desktop，子协议 guacamole）
// - 显示：按容器尺寸 × devicePixelRatio 请求远端分辨率，再按「适配 / 原始」缩放；窗口变化时 sendSize
// - 输入：鼠标（含滚轮）、触摸屏、键盘（仅视口聚焦时捕获，失焦释放全部按键）
// - 关闭原因：截获网关自定义指令 x-remlink-closed，其次 error 指令状态码，最后按隧道错误自动重连
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { NButton, NInput, NSpin, NTooltip } from 'naive-ui'
import Guacamole from 'guacamole-common-js'
import FullScreen from '~icons/fluent/full-screen-maximize-20-regular'
import ExitFullScreen from '~icons/fluent/full-screen-minimize-20-regular'
import Keyboard from '~icons/fluent/keyboard-20-regular'
import FitIcon from '~icons/fluent/arrow-fit-20-regular'
import OriginalIcon from '~icons/fluent/ratio-one-to-one-20-regular'
import ClipboardIcon from '~icons/fluent/clipboard-paste-20-regular'
import Copy from '~icons/fluent/copy-20-regular'
import Send from '~icons/fluent/send-20-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import Plug from '~icons/fluent/plug-disconnected-20-regular'
import { RemoteError, remoteApi } from '../api'

export type DeskState = 'connecting' | 'connected' | 'reconnecting' | 'disconnected' | 'ended'

const props = defineProps<{
  sid: string
  ticket: string
  wsPath?: string
  clipboard: boolean
  // prompt 凭据模式下 guacd 追加索要凭据时可在浏览器补充（网关仅此模式放行 argv）
  credPrompt?: boolean
}>()
const emit = defineEmits<{
  state: [s: DeskState]
  // 会话已在服务端结束（被踢 / 空闲 / 吊销 / 用户结束 / 服务重启）
  ended: [reason: string, message: string]
  // 目标主机认证失败：需回到凭据步骤
  authFailed: [message: string]
  disconnect: []
}>()

const root = ref<HTMLDivElement>()
const viewport = ref<HTMLDivElement>()
const host = ref<HTMLDivElement>()

const state = ref<DeskState>('connecting')
const everConnected = ref(false)
const overlay = ref<{ title: string; desc: string; action?: 'reconnect' } | null>(null)
const retryIn = ref(0)
const focused = ref(false)
const fullscreen = ref(false)
const mode = ref<'fit' | 'original'>('fit')
const clipOpen = ref(false)
const clipText = ref('')
const clipNew = ref(false)
const required = ref<string[] | null>(null)
const reqForm = reactive<Record<string, string>>({})

let client: Guacamole.Client | null = null
let tunnel: Guacamole.Tunnel | null = null
let keyboard: Guacamole.Keyboard | null = null
let ro: ResizeObserver | null = null
let gen = 0 // 每次附着递增，忽略旧连接的迟到回调
let attempts = 0
let retryTimer = 0
let sizeTimer = 0
let disposed = false
let lastSize = ''

// 单次附着期间收集到的关闭信息
let closedReason = ''
let closedMsg = ''
let guacStatus: Guacamole.Status | null = null // guacd error 指令

function setState(s: DeskState) {
  state.value = s
  emit('state', s)
}

const dpr = () => Math.min(Math.max(window.devicePixelRatio || 1, 1), 2)

// 期望的远端分辨率（设备像素）
function wantSize() {
  const v = viewport.value
  const w = Math.max(64, Math.round((v?.clientWidth || 1024) * dpr()))
  const h = Math.max(64, Math.round((v?.clientHeight || 768) * dpr()))
  return { w: Math.min(w, 8192), h: Math.min(h, 8192), dpi: Math.round(96 * dpr()) }
}

function rescale() {
  const v = viewport.value
  const d = client?.getDisplay()
  if (!v || !d) return
  const dw = d.getWidth()
  const dh = d.getHeight()
  if (!dw || !dh) return
  const s = mode.value === 'fit' ? Math.min(v.clientWidth / dw, v.clientHeight / dh) : 1 / dpr()
  if (s > 0 && Number.isFinite(s)) d.scale(s)
}

function onViewportResize() {
  rescale()
  clearTimeout(sizeTimer)
  sizeTimer = window.setTimeout(() => {
    if (!client || state.value !== 'connected') return
    const { w, h } = wantSize()
    const key = `${w}x${h}`
    if (key === lastSize) return
    lastSize = key
    client.sendSize(w, h)
  }, 250)
}

function connectData(ticket: string) {
  const { w, h, dpi } = wantSize()
  lastSize = `${w}x${h}`
  const q = new URLSearchParams({ ticket, width: String(w), height: String(h), dpi: String(dpi) })
  for (const a of Guacamole.AudioPlayer.getSupportedTypes()) q.append('audio', a)
  try {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
    if (tz) q.set('timezone', tz)
  } catch { /* 忽略 */ }
  return q.toString()
}

function teardown() {
  keyboard?.reset()
  const c = client
  const t = tunnel
  client = null
  tunnel = null
  if (t) {
    t.onstatechange = null
    t.onerror = null
  }
  if (c) {
    c.onstatechange = null
    c.onerror = null
    c.onclipboard = null
    c.onrequired = null
    try { c.disconnect() } catch { /* 忽略 */ }
    const el = c.getDisplay().getElement()
    el.parentNode?.removeChild(el)
  }
  required.value = null
}

function open(ticket: string) {
  clearTimeout(retryTimer)
  teardown()
  overlay.value = null
  closedReason = ''
  closedMsg = ''
  guacStatus = null
  const my = ++gen

  const t = new Guacamole.WebSocketTunnel(props.wsPath || remoteApi.desktopPath())
  const c = new Guacamole.Client(t)
  tunnel = t
  client = c

  // 截获网关自定义关闭通知（Client 会忽略未知指令，但需在其处理前取出原因）
  const orig = t.oninstruction
  t.oninstruction = (op, args) => {
    if (op === 'x-remlink-closed') {
      if (my === gen) { closedReason = args[0] || ''; closedMsg = args[1] || '' }
      return
    }
    orig?.(op, args)
  }
  t.onstatechange = s => {
    if (my !== gen) return
    // 隧道关闭是所有断开路径的汇合点（error 指令 → client.disconnect → 隧道关闭）
    if (s === Guacamole.Tunnel.State.CLOSED) window.setTimeout(() => { if (my === gen) handleClosed() }, 0)
  }

  c.onerror = st => { if (my === gen) guacStatus = st }
  c.onstatechange = s => {
    if (my !== gen) return
    if (s === Guacamole.Client.State.CONNECTED) {
      attempts = 0
      everConnected.value = true
      setState('connected')
      rescale()
      // 连接期间窗口可能变化，补发一次尺寸
      onViewportResize()
      if (!clipOpen.value) viewport.value?.focus()
    }
  }
  c.onclipboard = (stream, mimetype) => {
    if (!props.clipboard || !/^text\//.test(mimetype)) return
    const r = new Guacamole.StringReader(stream)
    let buf = ''
    r.ontext = s => { buf += s }
    r.onend = () => {
      clipText.value = buf
      if (!clipOpen.value) clipNew.value = true
    }
  }
  c.onrequired = params => {
    if (!props.credPrompt) return
    for (const k of params) reqForm[k] = ''
    required.value = params.filter(p => ['username', 'password', 'domain'].includes(p))
  }

  const d = c.getDisplay()
  const el = d.getElement()
  host.value!.appendChild(el)
  d.onresize = () => rescale()

  // 鼠标与触摸（坐标按显示缩放换算）
  const mouse = new Guacamole.Mouse(el)
  const touch = new Guacamole.Mouse.Touchscreen(el)
  const sendMouse = (e: Guacamole.Mouse.Event) => { if (my === gen) c.sendMouseState(e.state, true) }
  mouse.onEach(['mousedown', 'mousemove', 'mouseup'], sendMouse)
  touch.onEach(['mousedown', 'mousemove', 'mouseup'], sendMouse)
  // 本地光标：远端光标图像设置为 CSS 光标，隐藏显示层中的软件光标
  d.oncursor = (canvas, x, y) => { d.showCursor(!mouse.setCursor(canvas, x, y)) }

  setState(everConnected.value ? 'reconnecting' : 'connecting')
  try {
    c.connect(connectData(ticket))
  } catch {
    handleClosed()
  }
}

const reasonText: Record<string, string> = {
  kicked: '管理员已断开此会话',
  revoked: '你的访问权限已被吊销',
  unauthorized: '你已无权访问该应用',
  app_changed: '应用配置已变更，会话已结束',
  idle: '长时间无操作，会话已自动断开',
  user_close: '会话已结束',
  shutdown: '服务正在重启，会话已结束',
  exit: '远程桌面已断开（远程端已注销或关闭了连接）',
  remote_closed: '远程桌面连接已断开',
  auth_failed: '目标主机认证失败，请检查账号、密码或域',
  target_unreachable: '无法连接目标主机，请稍后再试或联系管理员',
  guacd_unavailable: '远程桌面服务（guacd）未部署或不可达，请联系管理员',
  conflict: '该会话已在其它浏览器窗口中打开，请先关闭那个窗口再重新连接',
  error: '远程桌面连接错误',
}
// 会话已在服务端结束
const ENDED = new Set(['kicked', 'revoked', 'unauthorized', 'app_changed', 'idle', 'user_close', 'shutdown'])
// 会话仍在，但自动重连无意义：交给用户决定
const MANUAL = new Set(['exit', 'remote_closed', 'target_unreachable', 'guacd_unavailable', 'conflict'])

// guacd error 指令状态码 → 关闭原因（无 x-remlink-closed 时兜底）
function reasonFromStatus(code: number) {
  switch (code) {
    case 0x0301: return 'auth_failed'
    case 0x0202: case 0x0203: case 0x0207: case 0x0208: return 'target_unreachable'
    case 0x0209: return 'conflict'
    case 0x020A: return 'idle'
    case 0x020B: return 'remote_closed'
    case 0x0303: return 'kicked'
  }
  return 'error'
}

function handleClosed() {
  if (disposed || !client) return
  keyboard?.reset()
  // 原因优先级：网关通知 x-remlink-closed > guacd error 指令状态码；仅有隧道错误（网络中断）时自动重连
  const reason = closedReason || (guacStatus ? reasonFromStatus(guacStatus.code) : '')
  const msg = closedMsg || reasonText[reason] || guacStatus?.message || '远程桌面连接已断开'
  teardown()

  if (ENDED.has(reason)) {
    setState('ended')
    emit('ended', reason, reasonText[reason] || msg)
    return
  }
  if (reason === 'auth_failed') {
    setState('disconnected')
    emit('authFailed', reasonText.auth_failed)
    return
  }
  if (MANUAL.has(reason)) {
    setState('disconnected')
    overlay.value = { title: reason === 'conflict' ? '已在其它窗口打开' : '远程桌面已断开', desc: reasonText[reason] || msg, action: 'reconnect' }
    return
  }
  // 网络中断 / 未知错误：凭新票据自动重连（会话仍在服务端保留）
  scheduleReconnect()
}

function scheduleReconnect() {
  if (attempts >= 6) {
    setState('disconnected')
    overlay.value = { title: '远程桌面连接已断开', desc: '多次重连失败，请检查网络后重试。', action: 'reconnect' }
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
  setState(everConnected.value ? 'reconnecting' : 'connecting')
  try {
    const t = await remoteApi.ticket(props.sid)
    if (disposed) return
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

// ---------- 工具栏 ----------
function sendCAD() {
  if (!client) return
  const keys = [0xFFE3, 0xFFE9, 0xFFFF] // Control_L, Alt_L, Delete
  for (const k of keys) client.sendKeyEvent(1, k)
  for (const k of [...keys].reverse()) client.sendKeyEvent(0, k)
  viewport.value?.focus()
}

function toggleMode() {
  mode.value = mode.value === 'fit' ? 'original' : 'fit'
  nextTick(rescale)
}

async function toggleFullscreen() {
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await root.value?.requestFullscreen()
  } catch { /* 浏览器拒绝 */ }
}
function onFsChange() {
  fullscreen.value = document.fullscreenElement === root.value
  // 全屏时尝试锁定系统快捷键（Esc / Win 等直接发往远端），支持的浏览器才生效
  const kb = (navigator as any).keyboard
  try {
    if (fullscreen.value) kb?.lock?.()
    else kb?.unlock?.()
  } catch { /* 忽略 */ }
  nextTick(onViewportResize)
}

function toggleClip() {
  clipOpen.value = !clipOpen.value
  clipNew.value = false
  if (!clipOpen.value) viewport.value?.focus()
}
function sendClip() {
  if (!client || !props.clipboard) return
  const w = new Guacamole.StringWriter(client.createClipboardStream('text/plain'))
  w.sendText(clipText.value)
  w.sendEnd()
  ui('已发送到远程剪贴板，可在远程桌面中粘贴')
}
async function copyClip() {
  try {
    await navigator.clipboard.writeText(clipText.value)
    ui('已复制到本机剪贴板')
  } catch {
    ui('浏览器不允许写入剪贴板，请手动选择文本复制')
  }
}
const toast = ref('')
let toastTimer = 0
function ui(msg: string) {
  toast.value = msg
  clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toast.value = '' }, 2200)
}

function submitRequired() {
  if (!client || !required.value) return
  for (const name of required.value) {
    const w = new Guacamole.StringWriter(client.createArgumentValueStream('text/plain', name))
    w.sendText(reqForm[name] || '')
    w.sendEnd()
  }
  required.value = null
  viewport.value?.focus()
}
const reqLabel: Record<string, string> = { username: '账号', password: '密码', domain: '域' }

// ---------- 键盘焦点 ----------
function onFocus() { focused.value = true }
function onBlur() {
  focused.value = false
  keyboard?.reset() // 释放所有按下的键，避免远端「粘键」
}
function onWinBlur() { keyboard?.reset() }

const scaleTip = computed(() => (mode.value === 'fit' ? '切换为原始尺寸（1:1，可滚动）' : '切换为适配窗口'))

onMounted(async () => {
  keyboard = new Guacamole.Keyboard(viewport.value!)
  keyboard.onkeydown = keysym => { client?.sendKeyEvent(1, keysym) }
  keyboard.onkeyup = keysym => { client?.sendKeyEvent(0, keysym) }
  ro = new ResizeObserver(onViewportResize)
  ro.observe(viewport.value!)
  document.addEventListener('fullscreenchange', onFsChange)
  window.addEventListener('blur', onWinBlur)
  await nextTick()
  open(props.ticket)
})

onBeforeUnmount(() => {
  disposed = true
  gen++
  clearTimeout(retryTimer)
  clearTimeout(sizeTimer)
  clearTimeout(toastTimer)
  ro?.disconnect()
  document.removeEventListener('fullscreenchange', onFsChange)
  window.removeEventListener('blur', onWinBlur)
  teardown()
  if (keyboard) { keyboard.onkeydown = null; keyboard.onkeyup = null }
})

defineExpose({ reconnect, focus: () => viewport.value?.focus() })
</script>

<template>
  <div ref="root" class="dp">
    <!-- 工具栏 -->
    <div class="dp__bar">
      <span class="dp__hint" :class="focused && 'dp__hint--on'">
        <Keyboard />{{ focused ? '键盘已捕获' : '点击画面以捕获键盘' }}
      </span>
      <div class="flex-1" />
      <NTooltip>
        <template #trigger>
          <NButton size="tiny" quaternary :disabled="state !== 'connected'" @click="sendCAD">Ctrl+Alt+Del</NButton>
        </template>
        向远程桌面发送 Ctrl+Alt+Del
      </NTooltip>
      <NTooltip>
        <template #trigger>
          <button class="rl-icon-btn" @click="toggleMode"><FitIcon v-if="mode === 'original'" /><OriginalIcon v-else /></button>
        </template>
        {{ scaleTip }}
      </NTooltip>
      <NTooltip v-if="clipboard">
        <template #trigger>
          <button class="rl-icon-btn dp__clipbtn" :class="clipOpen && 'dp__on'" @click="toggleClip">
            <ClipboardIcon /><span v-if="clipNew" class="dp__dot" />
          </button>
        </template>
        剪贴板
      </NTooltip>
      <NTooltip v-else>
        <template #trigger><span class="ax-badge text-secondary">剪贴板已禁用</span></template>
        管理员已禁用该应用的剪贴板同步
      </NTooltip>
      <NTooltip>
        <template #trigger>
          <button class="rl-icon-btn" @click="toggleFullscreen"><ExitFullScreen v-if="fullscreen" /><FullScreen v-else /></button>
        </template>
        {{ fullscreen ? '退出全屏' : '全屏' }}
      </NTooltip>
      <NButton size="tiny" secondary class="ml-4px" @click="emit('disconnect')">
        <template #icon><Plug /></template>断开
      </NButton>
    </div>

    <div class="dp__body">
      <div
        ref="viewport" class="dp__viewport" :class="mode === 'original' && 'dp__viewport--scroll'" tabindex="0"
        @focus="onFocus" @blur="onBlur" @mousedown="viewport?.focus()"
      >
        <div ref="host" class="dp__display" />
      </div>

      <!-- 剪贴板面板 -->
      <div v-if="clipboard && clipOpen" class="dp__clip">
        <div class="mb-8px flex items-center">
          <span class="text-md font-semibold">剪贴板</span>
          <div class="flex-1" />
          <button class="rl-icon-btn h-24px! w-24px!" title="关闭" @click="toggleClip"><Dismiss /></button>
        </div>
        <div class="mb-8px text-sm text-secondary">远程复制的文本会显示在这里；在此粘贴或输入文本后发送到远程，再在远程桌面中粘贴。</div>
        <NInput v-model:value="clipText" type="textarea" :rows="8" placeholder="剪贴板内容" :input-props="{ spellcheck: false }" />
        <div class="mt-8px flex gap-8px">
          <NButton size="small" type="primary" :disabled="state !== 'connected'" @click="sendClip">
            <template #icon><Send /></template>发送到远程
          </NButton>
          <NButton size="small" secondary :disabled="!clipText" @click="copyClip">
            <template #icon><Copy /></template>复制到本机
          </NButton>
        </div>
      </div>

      <!-- 首次连接 -->
      <div v-if="!everConnected && state === 'connecting' && !overlay" class="dp__overlay">
        <div class="text-center">
          <NSpin size="medium" />
          <div class="mt-12px text-md text-secondary">正在连接远程桌面…</div>
        </div>
      </div>

      <div v-if="state === 'reconnecting'" class="dp__banner">
        <span class="ax-badge text-warning">连接中断，{{ retryIn > 0 ? `${retryIn} 秒后` : '正在' }}重连…</span>
        <NButton size="tiny" secondary @click="reconnect">立即重连</NButton>
      </div>

      <!-- guacd 追加索要凭据（仅 prompt 模式） -->
      <div v-if="required && required.length" class="dp__overlay">
        <form class="dp__card" @submit.prevent="submitRequired">
          <div class="text-lg font-semibold">目标主机需要更多凭据</div>
          <div class="mb-12px mt-4px text-sm text-secondary">仅用于本次连接，不会被保存。</div>
          <div v-for="(name, i) in required" :key="name" class="mb-10px">
            <div class="mb-4px text-sm text-secondary">{{ reqLabel[name] || name }}</div>
            <NInput
              v-model:value="reqForm[name]" :type="name === 'password' ? 'password' : 'text'" show-password-on="click"
              :autofocus="i === 0" :input-props="{ autocomplete: name === 'password' ? 'current-password' : 'off' }"
            />
          </div>
          <NButton type="primary" block attr-type="submit">继续连接</NButton>
        </form>
      </div>

      <div v-if="overlay" class="dp__overlay">
        <div class="max-w-380px text-center">
          <div class="text-lg font-semibold">{{ overlay.title }}</div>
          <div class="mt-4px text-md text-secondary">{{ overlay.desc }}</div>
          <div class="mt-16px flex justify-center gap-8px">
            <NButton type="primary" @click="reconnect">重新连接</NButton>
            <NButton secondary @click="emit('disconnect')">结束会话</NButton>
          </div>
        </div>
      </div>

      <div v-if="toast" class="dp__toast">{{ toast }}</div>
    </div>
  </div>
</template>

<style scoped>
.dp { display: flex; flex-direction: column; height: 100%; min-height: 0; background: var(--color-body); }
.dp__bar {
  display: flex; align-items: center; gap: 4px; flex-shrink: 0; height: 40px; padding: 0 8px 0 12px;
  border-bottom: 1px solid var(--color-border-default); background: var(--color-primary);
}
.dp__hint { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: var(--color-text-tertiary); }
.dp__hint--on { color: var(--color-text-success); }
.dp__on { color: var(--color-text-primary); background: var(--color-action-hover); }
.dp__clipbtn { position: relative; }
.dp__dot { position: absolute; top: 6px; right: 6px; width: 6px; height: 6px; border-radius: 50%; background: var(--color-text-info); }
.dp__body { position: relative; flex: 1; min-height: 0; }
/* 远程画面区域固定深色底，与远端桌面边缘过渡自然（功能性底色，非界面配色） */
.dp__viewport { position: absolute; inset: 0; display: flex; overflow: hidden; outline: none; background: #000; }
.dp__viewport--scroll { overflow: auto; }
.dp__display { margin: auto; flex-shrink: 0; }
.dp__display :deep(> div) { touch-action: none; }
.dp__clip {
  position: absolute; top: 10px; right: 10px; width: min(360px, calc(100% - 20px)); padding: 12px; z-index: 4;
  border: 1px solid var(--color-border-default); border-radius: 12px; background: var(--color-primary);
  box-shadow: 0 8px 24px var(--color-overlay);
}
.dp__overlay {
  position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; z-index: 3; padding: 16px;
  background: color-mix(in srgb, var(--color-primary) 86%, transparent); backdrop-filter: blur(2px);
}
.dp__card {
  width: min(340px, 100%); padding: 20px; border: 1px solid var(--color-border-default); border-radius: 12px; background: var(--color-primary);
}
.dp__banner {
  position: absolute; top: 10px; left: 50%; transform: translateX(-50%); display: flex; gap: 8px; align-items: center; z-index: 2;
  padding: 6px 10px; border-radius: 10px; background: var(--color-primary); border: 1px solid var(--color-border-default);
  box-shadow: 0 4px 16px var(--color-overlay);
}
.dp__toast {
  position: absolute; bottom: 16px; left: 50%; transform: translateX(-50%); z-index: 5; padding: 6px 12px; border-radius: 8px;
  background: var(--color-primary); border: 1px solid var(--color-border-default); font-size: 13px; box-shadow: 0 4px 16px var(--color-overlay);
}
</style>
