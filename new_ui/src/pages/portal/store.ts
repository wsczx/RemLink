import { computed, reactive, ref } from 'vue'
import router from '@/router'
import { applyBrandToDocument } from '@/utils/brand'
import { ui } from '@/utils/ui'
import { defaultClientGuide, type GuideItem } from './clientGuide'
import { defaultBrand, isWebPath, parseJson, portalApi, safeRedirect, userLabel, type PortalBrand } from './shared'

// 用户门户共享状态（模块级单例）：登录态、用户信息、品牌、门户配置、设备 / 证书 / 应用数据
export interface AuthPreset {
  mode: '' | 'challenge' | 'reset'
  challengeType?: string
  session?: string
  resetToken?: string
  resetUsername?: string
}

const checking = ref(true)
const loggedIn = ref(false)
const brand = ref<PortalBrand>(defaultBrand())
const user = ref<any>({})
const dashboard = ref<any>({})
// WebVPN 子域名验证场景：登录只用于校验访问权限，通过后直接回跳内网应用，不进入门户
const webvpnMode = ref(false)
const webvpnGrant = ref('')
// 认证区初始状态（SSO 回调挑战 / 重置链接）
const preset = reactive<AuthPreset>({ mode: '' })

const devices = ref<any[]>([])
const devicesLoading = ref(false)
const devicesLoaded = ref(false)
const certs = ref<any[]>([])
const certsLoading = ref(false)
const webvpnApps = ref<any[]>([])
const webvpnLoading = ref(false)
const webvpnLoaded = ref(false)
const webvpnDomain = ref('')

const query = () => router.currentRoute.value.query

// ---------- 品牌 ----------
async function loadBrand() {
  try {
    const r = await portalApi('get', '/api/v1/portal/login-config')
    if (r.code !== 0 || !r.data) return
    const d = r.data
    brand.value = Object.assign(defaultBrand(), d)
    applyBrandToDocument(brand.value)
    // 复核：当前 host 是 WebVPN 主域的子域（init 已按 redirect 参数判过一次，不反向覆盖）
    if (!webvpnMode.value && d.domain) {
      const host = window.location.hostname
      const main = String(d.domain).replace(/^\./, '').split(':')[0]
      if (main && host !== main && host.endsWith('.' + main)) webvpnMode.value = true
    }
  } catch { /* 忽略 */ }
}

// 后台配置的自定义 CSS：注入 head，退出或离开门户时移除
let customCssEl: HTMLStyleElement | null = null
function applyCustomCss() {
  customCssEl?.remove()
  customCssEl = null
  const css = loggedIn.value ? dashboard.value.custom_css : ''
  if (css) {
    customCssEl = document.createElement('style')
    customCssEl.id = 'portal-custom-css'
    customCssEl.textContent = css
    document.head.appendChild(customCssEl)
  }
}

// ---------- 登录态 ----------
// WebVPN 子域名场景的统一回跳：有效 redirect 回跳该路径，否则回子域名根，由网关兑换会话后反代内网应用
function doWebVpnRedirect() {
  const path = safeRedirect(query().redirect) || '/'
  // 路径模式（/web/…）在主域名上直接使用门户会话，不附带一次性授权
  const q = webvpnGrant.value && !isWebPath(path) ? (path.includes('?') ? '&' : '?') + 'webvpn_grant=' + encodeURIComponent(webvpnGrant.value) : ''
  window.location.href = window.location.origin + path + q
}

async function loadMe() {
  try {
    const r = await portalApi('get', '/api/v1/portal/me')
    if (r.code !== 0) return
    user.value = r.data || {}
    if (webvpnMode.value) {
      doWebVpnRedirect()
      return
    }
    dashboard.value = r.data?.dashboard || {}
    loggedIn.value = true
    applyCustomCss()
    // 已登录且带 redirect（从 WebVPN 子站点跳来）：自动回跳
    const redirect = safeRedirect(query().redirect)
    if (redirect) {
      window.location.href = window.location.origin + redirect
      return
    }
    loadCerts()
    loadDevices()
    startDevicesPoll()
    loadWebVpnApps()
  } catch {
    loggedIn.value = false
  } finally {
    checking.value = false
  }
}

function onPass(d: any) {
  webvpnGrant.value = d?.webvpn_grant || ''
  if (webvpnMode.value) {
    doWebVpnRedirect()
    return
  }
  loggedIn.value = true
  loadMe()
  const redirect = safeRedirect(query().redirect)
  if (redirect) {
    window.location.href = window.location.origin + redirect
    return
  }
  ui.message.success('登录成功')
}

function onSmsPass() {
  loggedIn.value = true
  loadMe()
  ui.message.success('登录成功')
}

// 重置密码链接 ?reset_token=
async function checkResetToken() {
  const token = query().reset_token
  if (typeof token !== 'string' || !token || loggedIn.value) return
  try {
    const r = await portalApi('get', '/api/v1/portal/reset_password/verify?token=' + encodeURIComponent(token))
    const d = r.data || {}
    if (d.valid) Object.assign(preset, { mode: 'reset', resetToken: token, resetUsername: d.username || '未知用户' })
    else ui.message.error('重置链接无效或已过期')
  } catch {
    ui.message.error('验证重置链接失败')
  }
}

// SSO 回调后需二次验证：URL 带 session_id / challenge
function resumeCallbackChallenge() {
  const sid = query().session_id
  if (typeof sid !== 'string' || !sid) return
  const c = query().challenge
  Object.assign(preset, { mode: 'challenge', challengeType: typeof c === 'string' && c ? c : 'otp', session: sid })
  router.replace('/portal/')
}

async function logout() {
  stopDevicesPoll()
  // WebVPN 单点登出：清除独立 webvpn_session，不影响门户登录态
  portalApi('post', '/webvpn/logout').catch(() => {})
  try { await portalApi('post', '/api/v1/portal/logout') } catch { /* 忽略 */ }
  loggedIn.value = false
  user.value = {}
  dashboard.value = {}
  applyCustomCss()
  devices.value = []
  devicesLoaded.value = false
  certs.value = []
  webvpnApps.value = []
  webvpnLoaded.value = false
  Object.assign(preset, { mode: '', challengeType: '', session: '', resetToken: '', resetUsername: '' })
  router.replace('/portal/')
}

// ---------- 数据 ----------
let devicesTimer: ReturnType<typeof setInterval> | null = null

async function loadDevices() {
  devicesLoading.value = true
  try {
    const r = await portalApi('get', '/api/v1/portal/devices')
    if (r.code === 0) devices.value = (r.data || []).map((d: any) => ({ ...d, _kicking: false }))
  } catch { /* 忽略 */ } finally {
    devicesLoading.value = false
    devicesLoaded.value = true
  }
}
function startDevicesPoll() {
  stopDevicesPoll()
  devicesTimer = setInterval(loadDevices, 30000)
}
function stopDevicesPoll() {
  if (devicesTimer) clearInterval(devicesTimer)
  devicesTimer = null
}

async function kickDevice(d: any) {
  d._kicking = true
  try {
    const r = await portalApi('post', '/api/v1/portal/devices/offline', { token: d.token })
    if (r.code !== 0) {
      ui.message.error(r.msg)
      d._kicking = false
      return
    }
    ui.message.success('已断开该设备连接')
    devices.value = devices.value.filter(x => x.token !== d.token)
  } catch {
    ui.message.error('操作失败')
    d._kicking = false
  }
}

async function loadCerts() {
  certsLoading.value = true
  try {
    const r = await portalApi('get', '/api/v1/portal/certs')
    if (r.code === 0) certs.value = r.data || []
  } catch { /* 忽略 */ } finally {
    certsLoading.value = false
  }
}

async function loadWebVpnApps() {
  webvpnLoading.value = true
  portalApi('get', '/webvpn/login-config').then(r => {
    if (r.code === 0) webvpnDomain.value = r.data?.domain || ''
  }).catch(() => {})
  try {
    const r = await portalApi('get', '/webvpn/my-apps')
    if (r.code === 0) webvpnApps.value = r.data || []
  } catch { /* 忽略 */ } finally {
    webvpnLoading.value = false
    webvpnLoaded.value = true
  }
}

// ---------- 派生展示 ----------
const displayName = computed(() => userLabel(user.value.username, user.value.name) || '用户')
const typeLabel = computed(() => {
  const m: Record<string, string> = { local: '本地用户', ldap: 'LDAP 用户', radius: 'RADIUS 用户', wxwork: '企微用户', feishu: '飞书用户', dingtalk: '钉钉用户', external: '外部用户' }
  return m[user.value.type] || user.value.type || '本地用户'
})

// 到期：limittime 可能是秒级时间戳或日期字符串
const limitTs = computed(() => {
  const t = user.value.limittime
  if (!t) return 0
  return typeof t === 'number' ? t : Date.parse(t) / 1000
})
const remainDays = computed(() => (limitTs.value ? Math.ceil((limitTs.value - Date.now() / 1000) / 86400) : null))
const expire = computed(() => {
  const d = remainDays.value
  let value = '永久有效'
  if (limitTs.value) {
    const dt = new Date(limitTs.value * 1000)
    const p = (n: number) => String(n).padStart(2, '0')
    value = `${dt.getFullYear()}-${p(dt.getMonth() + 1)}-${p(dt.getDate())} ${p(dt.getHours())}:${p(dt.getMinutes())}`
  }
  return {
    value,
    label: d === null ? '永久有效' : d <= 0 ? '已过期' : `还有 ${d} 天到期`,
    tone: (d === null ? 'success' : d <= 0 ? 'error' : d <= 30 ? 'warning' : 'success') as 'success' | 'warning' | 'error',
  }
})

const cardsVisible = computed(() => parseJson<Record<string, boolean>>(dashboard.value.cards_visible, {}))
const cardVisible = (name: string) => cardsVisible.value[name] !== false

const clientGuide = computed<GuideItem[]>(() => {
  const l = parseJson<any[]>(dashboard.value.client_guide, [])
  return Array.isArray(l) && l.length ? l : defaultClientGuide
})
const showClientGuide = computed(() => dashboard.value.client_guide_enabled === 1 && clientGuide.value.length > 0)

// 各页面是否对用户开放（按后台「门户卡片可见性」）
const pageVisible = computed<Record<string, boolean>>(() => ({
  devices: cardVisible('devices'),
  clients: showClientGuide.value || !!dashboard.value.client_download_html,
  // 应用：后台未隐藏，且（尚未加载完或确有应用）
  apps: cardVisible('webvpn') && (!webvpnLoaded.value || webvpnApps.value.length > 0),
  access: cardVisible('groups') || (cardVisible('personal_policy') && !!user.value.user_policy),
  security: cardVisible('password') || cardVisible('otp'),
  certs: cardVisible('certs'),
}))

let inited = false
function init() {
  if (inited) return
  inited = true
  checkResetToken()
  // 初判 WebVPN 子域名场景：网关 302 到登录页总带 redirect；同步设置避免与 loadMe 竞态
  if (safeRedirect(query().redirect)) webvpnMode.value = true
  loadBrand()
  loadMe()
  resumeCallbackChallenge()
}

// 离开门户（外壳卸载）时释放定时器与注入样式；再次进入重新初始化
function dispose() {
  stopDevicesPoll()
  customCssEl?.remove()
  customCssEl = null
  inited = false
}

export function usePortal() {
  return {
    checking, loggedIn, brand, user, dashboard, webvpnMode, preset,
    devices, devicesLoading, devicesLoaded, certs, certsLoading, webvpnApps, webvpnLoading, webvpnDomain,
    displayName, typeLabel, remainDays, expire, cardVisible, clientGuide, showClientGuide, pageVisible,
    init, dispose, loadBrand, loadMe, onPass, onSmsPass, logout,
    loadDevices, kickDevice, loadCerts, loadWebVpnApps,
  }
}
