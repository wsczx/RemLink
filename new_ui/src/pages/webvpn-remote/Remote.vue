<script setup lang="ts">
// WebVPN 远程访问（SSH 终端 + 文件 / SFTP·SMB·FTP 文件 / RDP·VNC 远程桌面）。两种访问方式：
//   - 子域名：https://<app>.<WebVpnDomain>/ui/#/webvpn/remote，接口 /webvpn/*（webvpn_session）
//   - 路径：https://<VPN 主域名>/ui/#/webvpn/remote?app=&protocol=&mode=path，接口 /web/<协议>/<应用>/webvpn/*（门户会话）
// 独立外壳
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NSpin, NSplit, NTooltip } from 'naive-ui'
import Console from '~icons/fluent/window-console-20-regular'
import DesktopIcon from '~icons/fluent/desktop-20-regular'
import FolderOpen from '~icons/fluent/folder-open-20-regular'
import Moon from '~icons/fluent/weather-moon-20-regular'
import Sun from '~icons/fluent/weather-sunny-20-regular'
import SplitV from '~icons/fluent/split-vertical-20-regular'
import SplitH from '~icons/fluent/split-horizontal-20-regular'
import PanelFiles from '~icons/fluent/folder-20-regular'
import Plug from '~icons/fluent/plug-disconnected-20-regular'
import Record from '~icons/fluent/record-20-regular'
import ShieldError from '~icons/fluent/shield-error-20-regular'
import StatusDot from '@/components/StatusDot.vue'
import { useThemeStore } from '@/stores/theme'
import { confirm } from '@/utils/ui'
import { formatDate } from '@/utils/format'
import { useRoute } from 'vue-router'
import { RemoteError, configureRemote, friendlyError, remoteApi, remoteEntry, type AppInfo, type CredInput, type LiveSession, type SessionInfo } from './api'
import CredForm from './components/CredForm.vue'
import TerminalPane, { type TermState } from './components/TerminalPane.vue'
import FileBrowser from './components/FileBrowser.vue'
import type { DeskState } from './components/DesktopPane.vue'
// 远程桌面依赖 guacamole-common-js（约 400KB），按需加载
const DesktopPane = defineAsyncComponent(() => import('./components/DesktopPane.vue'))

type Phase = 'loading' | 'fatal' | 'cred' | 'connecting' | 'connected' | 'ended'

const theme = useThemeStore()
// 按页面参数确定访问方式（子域名 / 路径），须在任何接口调用之前完成
configureRemote(useRoute().query)
const phase = ref<Phase>('loading')
const info = ref<AppInfo | null>(null)
const fatal = ref('')
const fatalTitle = ref('')
const credErr = ref<{ code: string; msg: string; fp?: string } | null>(null)
const busy = ref(false)
const sess = ref<SessionInfo | null>(null)
const target = ref('')
const termState = ref<TermState>('connecting')
const deskState = ref<DeskState>('connecting')
const endedMsg = ref('')
const others = ref<LiveSession[]>([])

// 布局：左右 / 上下分栏、文件面板开合（本地记忆）
const LS_LAYOUT = 'rl-remote-layout'
const store = {
  get(k: string) { try { return localStorage.getItem(k) } catch { return null } },
  set(k: string, v: string) { try { localStorage.setItem(k, v) } catch { /* 忽略 */ } },
}
const saved = (() => { try { return JSON.parse(store.get(LS_LAYOUT) || '{}') } catch { return {} } })()
const layout = ref<'horizontal' | 'vertical'>(saved.layout === 'vertical' ? 'vertical' : 'horizontal')
const filesOpen = ref(saved.files !== false)
const splitSize = ref<number>(typeof saved.size === 'number' ? saved.size : 0.62)
watch([layout, filesOpen, splitSize], () => store.set(LS_LAYOUT, JSON.stringify({ layout: layout.value, files: filesOpen.value, size: splitSize.value })))

const isSSH = computed(() => !!info.value?.terminal)
const isDesktop = computed(() => !!info.value?.desktop)
const recordTip = computed(() => (isDesktop.value ? '管理员已开启会话录像，远程桌面画面会被记录用于安全审计（不含键盘输入）' : '管理员已开启会话录像，终端输出会被记录用于安全审计'))
const title = computed(() => info.value?.note || info.value?.name || '远程访问')
const autoMode = computed(() => !!info.value && !info.value.need_username && !info.value.need_password)
const perms = computed(() => sess.value?.permissions || info.value?.permissions || { upload: false, download: false, delete: false, clipboard: false, max_file_size: 0 })
const userAtHost = computed(() => {
  const u = sess.value?.target_user || info.value?.fixed_username || ''
  return target.value ? (u ? `${u}@${target.value}` : target.value) : u
})

// 会话恢复（刷新页面后重新附着），按应用记在 sessionStorage
const ssKey = () => `rl-remote-sid:${info.value?.name}`
function remember(s: SessionInfo | null) {
  try {
    if (s) sessionStorage.setItem(ssKey(), JSON.stringify({ sid: s.sid, target_user: s.target_user }))
    else sessionStorage.removeItem(ssKey())
  } catch { /* 忽略 */ }
}

const status = computed<{ tone: 'success' | 'warning' | 'error' | 'info' | 'muted'; text: string; pulse?: boolean }>(() => {
  switch (phase.value) {
    case 'loading': case 'connecting': return { tone: 'info', text: '连接中', pulse: true }
    case 'cred': return { tone: 'muted', text: '未连接' }
    case 'fatal': return { tone: 'error', text: '不可用' }
    case 'ended': return { tone: 'muted', text: '已结束' }
  }
  if (isDesktop.value) {
    switch (deskState.value) {
      case 'connected': return { tone: 'success', text: '已连接' }
      case 'reconnecting': return { tone: 'warning', text: '重连中', pulse: true }
      case 'disconnected': return { tone: 'error', text: '已断开' }
      case 'ended': return { tone: 'muted', text: '已结束' }
    }
    return { tone: 'info', text: '连接中', pulse: true }
  }
  if (!isSSH.value) return { tone: 'success', text: '已连接' }
  switch (termState.value) {
    case 'connected': return { tone: 'success', text: '已连接' }
    case 'reconnecting': return { tone: 'warning', text: '重连中', pulse: true }
    case 'disconnected': return { tone: 'error', text: '已断开' }
    case 'exited': return { tone: 'muted', text: 'Shell 已退出' }
    case 'ended': return { tone: 'muted', text: '已结束' }
  }
  return { tone: 'info', text: '连接中', pulse: true }
})

function toPortalLogin() {
  // 未登录：回到当前域名的门户登录页，登录后回到应用入口（子域名根 / 路径模式的 /web/<协议>/<应用>/），会再次跳转到本页
  window.location.href = '/ui/#/portal?redirect=' + encodeURIComponent(remoteEntry())
}

async function loadOthers() {
  try { others.value = (await remoteApi.sessions()) || [] } catch { others.value = [] }
}

async function fillTarget(sid: string) {
  try {
    const list = (await remoteApi.sessions()) || []
    others.value = list
    target.value = list.find(s => s.sid === sid)?.target || ''
  } catch { /* 忽略 */ }
}

// 附着已有会话（刷新恢复或从列表接入）
async function attach(sid: string, targetUser = '') {
  const i = info.value!
  phase.value = 'connecting'
  try {
    let ticket = ''
    let wsPath = i.ws_path || ''
    if (wsPath || i.terminal || i.desktop) {
      const t = await remoteApi.ticket(sid)
      ticket = t.ticket
      wsPath = t.ws_path || wsPath
    }
    sess.value = {
      sid, protocol: i.protocol, target_user: targetUser || i.fixed_username, host_key_fp: '', idle_timeout: i.idle_timeout,
      record: i.record, permissions: i.permissions, ticket, ticket_expires: 0, ws_path: wsPath,
    }
    remember(sess.value)
    termState.value = 'connecting'
    deskState.value = 'connecting'
    phase.value = 'connected'
    fillTarget(sid)
    return true
  } catch (e) {
    remember(null)
    if (e instanceof RemoteError && e.status === 401) { toPortalLogin(); return false }
    credErr.value = { code: e instanceof RemoteError ? e.code : '', msg: friendlyError(e) }
    phase.value = 'cred'
    loadOthers()
    return false
  }
}

async function connect(c: CredInput) {
  busy.value = true
  credErr.value = null
  if (autoMode.value) phase.value = 'connecting'
  try {
    const s = await remoteApi.createSession({ ...c, app: info.value!.name })
    sess.value = s
    remember(s)
    termState.value = 'connecting'
    deskState.value = 'connecting'
    phase.value = 'connected'
    fillTarget(s.sid)
  } catch (e) {
    if (e instanceof RemoteError && e.status === 401 && e.code === 'unauthorized') { toPortalLogin(); return }
    credErr.value = {
      code: e instanceof RemoteError ? e.code : '',
      msg: friendlyError(e),
      fp: e instanceof RemoteError && e.code === 'host_key_mismatch' ? e.data?.fingerprint : undefined,
    }
    phase.value = 'cred'
    loadOthers()
  } finally {
    busy.value = false
  }
}

async function init() {
  phase.value = 'loading'
  try {
    info.value = await remoteApi.appInfo()
  } catch (e) {
    if (e instanceof RemoteError && e.status === 401) return toPortalLogin()
    fatalTitle.value = ''
    fatal.value = friendlyError(e)
    phase.value = 'fatal'
    return
  }
  document.title = `${title.value} · 远程访问`
  // 远程桌面依赖 guacd：未部署时直接说明，避免让用户白填凭据
  if (info.value.desktop && info.value.guacd_available === false) {
    fatalTitle.value = '远程桌面服务未部署'
    fatal.value = '管理员尚未部署远程桌面代理服务（guacd），暂时无法通过浏览器访问 RDP / VNC 桌面，请联系管理员。'
    phase.value = 'fatal'
    return
  }
  // 刷新恢复：本标签页记住的会话仍在线则直接附着
  let prev: { sid: string; target_user: string } | null = null
  try { prev = JSON.parse(sessionStorage.getItem(ssKey()) || 'null') } catch { /* 忽略 */ }
  if (prev?.sid) {
    await loadOthers()
    if (others.value.some(s => s.sid === prev!.sid) && (await attach(prev.sid, prev.target_user))) return
    remember(null)
  }
  if (autoMode.value) return connect({})
  phase.value = 'cred'
  loadOthers()
}

async function disconnect() {
  if (!sess.value) return
  if (!(await confirm('确认断开并结束本次会话？', '断开连接', false))) return
  const sid = sess.value.sid
  try { await remoteApi.closeSession(sid) } catch { /* 已结束 */ }
  endSession('你已断开连接')
}

function endSession(msg: string) {
  remember(null)
  endedMsg.value = msg
  phase.value = 'ended'
}

// 远程桌面认证失败：结束该会话（认证材料已失效），回到凭据步骤
async function onDeskAuthFailed(msg: string) {
  const sid = sess.value?.sid
  remember(null)
  sess.value = null
  if (sid) { try { await remoteApi.closeSession(sid) } catch { /* 已结束 */ } }
  credErr.value = { code: 'auth_failed', msg }
  phase.value = 'cred'
  loadOthers()
}

function onTermEnded(_reason: string, msg: string) {
  // 终端收到会话结束：保留终端画面几秒钟内的上下文，直接切到结束页
  endSession(msg)
}

async function endOther(s: LiveSession) {
  try { await remoteApi.closeSession(s.sid) } catch { /* 忽略 */ }
  loadOthers()
}

function reconnect() {
  sess.value = null
  target.value = ''
  credErr.value = null
  if (autoMode.value) connect({})
  else { phase.value = 'cred'; loadOthers() }
}

// SFTP 会话：定期探测会话是否仍在（被踢 / 空闲回收）
let aliveTimer = 0
watch(phase, p => {
  clearInterval(aliveTimer)
  if (p !== 'connected' || isSSH.value || isDesktop.value) return
  aliveTimer = window.setInterval(async () => {
    if (!sess.value) return
    try {
      const list = await remoteApi.sessions()
      if (!(list || []).some(s => s.sid === sess.value!.sid)) endSession('会话已结束（可能被管理员断开或空闲超时）')
    } catch (e) {
      if (e instanceof RemoteError && (e.status === 401 || e.status === 403)) endSession(friendlyError(e))
    }
  }, 30000)
})

onMounted(init)
onBeforeUnmount(() => clearInterval(aliveTimer))
</script>

<template>
  <div class="rr">
    <!-- 顶栏 -->
    <header class="rr__top">
      <div class="flex min-w-0 items-center gap-10px">
        <span class="rr__icon"><DesktopIcon v-if="isDesktop" /><Console v-else-if="isSSH || info?.protocol === 'ssh'" /><FolderOpen v-else /></span>
        <div class="min-w-0">
          <div class="flex items-center gap-8px">
            <span class="truncate text-md font-semibold">{{ title }}</span>
            <span v-if="info" class="ax-badge h-18px! px-6px! text-secondary">{{ info.protocol.toUpperCase() }}</span>
          </div>
          <div v-if="userAtHost" class="rl-mono truncate text-sm! text-secondary">{{ userAtHost }}</div>
        </div>
      </div>
      <StatusDot class="ml-16px flex-shrink-0" :tone="status.tone" :pulse="status.pulse">{{ status.text }}</StatusDot>
      <NTooltip v-if="info?.record">
        <template #trigger>
          <span class="ax-badge ml-8px flex-shrink-0 gap-4px text-error"><Record class="rr__rec" />本会话将被录像</span>
        </template>
        {{ recordTip }}
      </NTooltip>
      <span v-if="phase === 'connected' && !perms.clipboard && isSSH" class="ax-badge ml-8px hidden flex-shrink-0 text-secondary md:inline-flex">剪贴板已禁用</span>

      <div class="flex-1" />

      <template v-if="phase === 'connected' && isSSH">
        <NTooltip>
          <template #trigger>
            <button class="rl-icon-btn" :class="filesOpen && 'rr__on'" @click="filesOpen = !filesOpen"><PanelFiles /></button>
          </template>
          {{ filesOpen ? '收起' : '展开' }}文件面板
        </NTooltip>
        <NTooltip>
          <template #trigger>
            <button class="rl-icon-btn" :disabled="!filesOpen" @click="layout = layout === 'horizontal' ? 'vertical' : 'horizontal'">
              <SplitV v-if="layout === 'horizontal'" /><SplitH v-else />
            </button>
          </template>
          切换为{{ layout === 'horizontal' ? '上下' : '左右' }}分栏
        </NTooltip>
      </template>
      <button class="rl-icon-btn" title="切换主题" @click="theme.toggle()"><Sun v-if="theme.isDark" /><Moon v-else /></button>
      <NButton v-if="phase === 'connected' && !isDesktop" class="ml-6px" size="small" secondary @click="disconnect">
        <template #icon><Plug /></template>断开
      </NButton>
    </header>

    <!-- 主体 -->
    <main class="rr__main">
      <div v-if="phase === 'loading' || phase === 'connecting'" class="rr__center">
        <NSpin size="medium" />
        <div class="mt-12px text-md text-secondary">{{ phase === 'loading' ? '正在加载…' : '正在连接目标主机…' }}</div>
      </div>

      <div v-else-if="phase === 'fatal'" class="rr__center">
        <ShieldError class="text-4xl text-error" />
        <div class="mt-8px text-lg font-semibold">{{ fatalTitle || '无法访问该应用' }}</div>
        <div class="mt-4px max-w-420px text-center text-md text-secondary">{{ fatal }}</div>
        <NButton class="mt-16px" @click="init">重试</NButton>
      </div>

      <div v-else-if="phase === 'cred' && info" class="rr__center rr__center--top">
        <CredForm v-if="!autoMode" :info="info" :busy="busy" :error="credErr" @submit="connect" />
        <div v-else class="w-full max-w-380px">
          <h1 class="m-0 text-4xl font-semibold leading-32px">连接失败</h1>
          <div class="mb-20px mt-6px text-md text-secondary">{{ credErr?.msg || '无法连接到目标主机' }}</div>
          <div v-if="credErr?.fp" class="mb-16px text-sm text-secondary">
            目标主机当前指纹：<span class="rl-mono break-all text-sm! text-primary">{{ credErr.fp }}</span>
          </div>
          <NButton type="primary" block :loading="busy" @click="connect({})">重试</NButton>
        </div>

        <!-- 本应用上的其它在线会话 -->
        <div v-if="others.length" class="mt-32px w-full max-w-380px">
          <div class="mb-8px text-sm text-secondary">你在此应用上有 {{ others.length }} 个在线会话（上限 {{ info.max_sessions }}）</div>
          <div class="rr__others">
            <div v-for="s in others" :key="s.sid" class="rr__other">
              <div class="min-w-0 flex-1">
                <div class="rl-mono truncate text-sm!">{{ s.target }}</div>
                <div class="truncate text-sm text-tertiary">{{ formatDate(s.started_at, 'MM-DD HH:mm') }} 起 · {{ s.client_ip }}<template v-if="s.terminals"> · {{ isDesktop ? '桌面' : '终端' }}已附着</template></div>
              </div>
              <NButton size="tiny" secondary @click="attach(s.sid)">接入</NButton>
              <NButton size="tiny" quaternary type="error" @click="endOther(s)">结束</NButton>
            </div>
          </div>
        </div>
      </div>

      <div v-else-if="phase === 'ended'" class="rr__center">
        <Plug class="text-4xl text-tertiary" />
        <div class="mt-8px text-lg font-semibold">会话已结束</div>
        <div class="mt-4px max-w-420px text-center text-md text-secondary">{{ endedMsg }}</div>
        <NButton class="mt-16px" type="primary" @click="reconnect">重新连接</NButton>
      </div>

      <template v-else-if="phase === 'connected' && sess">
        <DesktopPane
          v-if="isDesktop"
          :sid="sess.sid" :ticket="sess.ticket" :ws-path="sess.ws_path || info?.ws_path" :clipboard="perms.clipboard"
          :cred-prompt="info?.cred_mode === 'prompt'"
          @state="deskState = $event" @ended="onTermEnded" @auth-failed="onDeskAuthFailed" @disconnect="disconnect"
        />
        <NSplit
          v-else-if="isSSH"
          :direction="layout" :size="filesOpen ? splitSize : 1" :min="0.25" :max="filesOpen ? 0.85 : 1"
          :disabled="!filesOpen" :resize-trigger-size="filesOpen ? 4 : 0" class="h-full"
          pane2-class="rr__pane2" @update:size="(v: number | string) => { if (filesOpen) splitSize = Number(v) }"
        >
          <template #1>
            <TerminalPane
              :sid="sess.sid" :ticket="sess.ticket" :clipboard="perms.clipboard" :dark="theme.isDark"
              @state="termState = $event" @ended="onTermEnded"
            />
          </template>
          <template #2>
            <FileBrowser v-show="filesOpen" :sid="sess.sid" :perms="perms" :compact="layout === 'horizontal'" />
          </template>
          <template #resize-trigger>
            <div class="rr__trigger" :class="`rr__trigger--${layout}`" />
          </template>
        </NSplit>
        <FileBrowser v-else :sid="sess.sid" :perms="perms" />
      </template>
    </main>
  </div>
</template>

<style scoped>
.rr { display: flex; flex-direction: column; height: 100vh; height: 100dvh; background: var(--color-body); color: var(--color-text-primary); }
.rr__top {
  display: flex; align-items: center; gap: 4px; flex-shrink: 0; height: 52px; padding: 0 12px 0 16px;
  border-bottom: 1px solid var(--color-border-default); background: var(--color-primary); min-width: 0;
}
.rr__icon { display: flex; align-items: center; justify-content: center; width: 32px; height: 32px; flex-shrink: 0; border-radius: 8px; background: var(--color-fill); font-size: 18px; color: var(--color-text-secondary); }
.rr__on { color: var(--color-text-primary); background: var(--color-action-hover); }
.rr__rec { animation: rr-blink 1.6s ease-in-out infinite; }
@keyframes rr-blink { 50% { opacity: .35; } }
.rr__main { position: relative; flex: 1; min-height: 0; }
.rr__center { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; padding: 24px 16px; overflow-y: auto; }
.rr__center--top { justify-content: flex-start; padding-top: max(48px, 12vh); }
.rr__others { border: 1px solid var(--color-border-default); border-radius: 10px; }
.rr__other { display: flex; align-items: center; gap: 8px; padding: 8px 10px 8px 12px; }
.rr__other + .rr__other { border-top: 1px solid var(--color-border-default); }
.rr__trigger { background: var(--color-border-default); transition: background .15s; }
.rr__trigger:hover { background: var(--color-text-info); }
.rr__trigger--horizontal { width: 100%; height: 100%; }
.rr__trigger--vertical { width: 100%; height: 100%; }
.rl-icon-btn:disabled { opacity: .4; cursor: not-allowed; }
:deep(.rr__pane2) { min-width: 0; min-height: 0; overflow: hidden; }
</style>
