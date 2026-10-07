import axios, { type Method } from 'axios'
import { ui } from '@/utils/ui'

// 门户接口统一走裸 axios：不经过后台 http 实例的拦截器（401 不会跳后台登录页）
export interface PortalResp<T = any> { code: number; msg: string; data: T }

// 门户写接口的 CSRF 防护头（后端 portalCSRFMiddleware 校验；跨站页面无法在不触发预检的情况下携带）
export const PORTAL_HEADERS = { 'Content-Type': 'application/json', 'X-RemLink-Portal': '1' }

export function portalApi<T = any>(method: Method, url: string, data?: any) {
  return axios
    .request<PortalResp<T>>({ method, url, data, headers: PORTAL_HEADERS })
    .then(r => r.data)
}

// 品牌配置（/api/v1/portal/login-config）
export interface PortalBrand {
  title: string
  logo: string
  favicon: string
  desc?: string
  footer: string
  sso_types: string[]
  sms_enabled: boolean
  features_enabled: number
  features: string
  domain: string
  webvpn_sso_domain: string
  [k: string]: any
}

export const defaultBrand = (): PortalBrand => ({
  title: '', logo: '', favicon: '', footer: '', sso_types: [], sms_enabled: false,
  features_enabled: 0, features: '', domain: '', webvpn_sso_domain: '',
})

// redirect 仅允许本站相对路径（单 "/" 开头；排除 "//"、"/\" 等会被浏览器视为协议相对地址的写法与控制字符）
export function safeRedirect(v: unknown): string {
  if (typeof v !== 'string' || !v.startsWith('/') || v[1] === '/' || v[1] === '\\') return ''
  for (let i = 0; i < v.length; i++) if (v.charCodeAt(i) < 0x20) return ''
  return v
}

// 是否为 WebVPN 路径模式入口（/web/<协议>/<应用>/…）：主域名直接复用门户会话，无需 webvpn_grant
export const isWebPath = (p: string) => /^\/web\/[a-z]+\/[a-z0-9-]+(\/|$)/.test(p)

// 用户名(昵称)
export function userLabel(username?: string, nickname?: string) {
  if (!username) return ''
  return nickname ? `${username}(${nickname})` : username
}

// 新密码规则：至少 8 位，含字母和数字
export function validatePassword(pwd: string) {
  if (!pwd || pwd.length < 8) return '密码长度至少 8 位'
  if (!/[a-zA-Z]/.test(pwd)) return '密码必须包含至少一个字母'
  if (!/[0-9]/.test(pwd)) return '密码必须包含至少一个数字'
  return ''
}

// 密码强度 0-4
export function pwdStrength(pwd: string) {
  if (!pwd) return 0
  let s = 0
  if (pwd.length >= 8) s++
  if (pwd.length >= 12) s++
  if (/[a-z]/.test(pwd) && /[A-Z]/.test(pwd)) s++
  if (/\d/.test(pwd)) s++
  if (/[^a-zA-Z0-9]/.test(pwd)) s++
  return Math.min(4, s)
}

// 带宽：后端单位 Byte/s，显示为 bps 系列
export function formatBandwidth(bytePerSec: number) {
  if (!bytePerSec) return '-'
  const mbps = (bytePerSec * 8) / 1e6
  if (mbps >= 1000) return (mbps / 1000).toFixed(1) + ' Gbps'
  if (mbps >= 1) return mbps.toFixed(1) + ' Mbps'
  return ((bytePerSec * 8) / 1000).toFixed(0) + ' Kbps'
}

// 流量：IEC 二进制单位
export function formatTraffic(bytes: number) {
  if (!bytes) return '-'
  const KB = 1024, MB = KB * 1024, GB = MB * 1024
  if (bytes >= GB) return (bytes / GB).toFixed(2) + ' GB'
  if (bytes >= MB) return (bytes / MB).toFixed(2) + ' MB'
  if (bytes >= KB) return (bytes / KB).toFixed(2) + ' KB'
  return bytes + ' B'
}

export function resetLabel(period?: string) {
  return ({ daily: '/日', weekly: '/周', monthly: '/月' } as Record<string, string>)[period || ''] || ''
}

export function formatCertTime(t: any) {
  if (!t) return '-'
  if (typeof t === 'number') return new Date(t * 1000).toLocaleDateString('zh-CN')
  const d = new Date(t)
  if (!isNaN(d.getTime())) return d.toLocaleDateString('zh-CN')
  return String(t).slice(0, 10)
}

export function copyText(text?: string) {
  if (!text) return
  const done = () => ui.message.success('已复制到剪贴板')
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).then(done)
    return
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.left = '-9999px'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  done()
}

// 安全解析 JSON 字符串，失败返回兜底值
export function parseJson<T>(s: unknown, fallback: T): T {
  if (!s || typeof s !== 'string') return fallback
  try { return JSON.parse(s) ?? fallback } catch { return fallback }
}
