// WebVPN 应用类型与提交载荷工具
import type { Component } from 'vue'
import Globe from '~icons/fluent/globe-20-regular'
import Console from '~icons/fluent/window-console-20-regular'
import FolderOpen from '~icons/fluent/folder-open-20-regular'
import Server from '~icons/fluent/server-20-regular'
import Desktop from '~icons/fluent/desktop-20-regular'
import ArrowSwap from '~icons/fluent/arrow-swap-20-regular'

export type Protocol = 'http' | 'ssh' | 'sftp' | 'smb' | 'ftp' | 'rdp' | 'vnc'
export type CredMode = 'prompt' | 'stored' | 'passthrough'
// 访问方式：子域名（前置）/ 路径（后置）/ 两者
export type AccessMode = 'subdomain' | 'path' | 'both'

export interface WebVpnApp {
  id: number
  name: string
  note: string
  backend: string
  host_rewrite?: string
  skip_verify?: boolean
  users?: string[]
  groups?: string[]
  allow_path?: string[]
  ip_allow_list?: string[]
  allow_cross_site?: boolean
  cors_allowed_origins?: string[]
  status: number
  access_mode?: AccessMode | ''
  // 非 HTTP 协议
  protocol?: Protocol | ''
  target_host?: string
  target_port?: number
  cred_mode?: CredMode | ''
  cred_user?: string
  cred_secret?: string // 仅提交：明文，留空 = 不修改
  cred_key?: string
  clear_secret?: boolean
  clear_key?: boolean
  has_secret?: boolean
  has_key?: boolean
  host_key_fp?: string
  options?: string // JSON，如 {"root":"/data"}
  allow_upload?: boolean
  allow_download?: boolean
  allow_delete?: boolean
  allow_clipboard?: boolean
  max_file_size?: number
  record?: boolean
  idle_timeout?: number
  max_sessions_per_user?: number
  created_at?: string
  updated_at?: string
}

export interface ProtocolDef {
  value: Protocol
  label: string
  desc: string
  icon: Component
  port: number
  ready: boolean // 后端已支持
}

export const PROTOCOLS: ProtocolDef[] = [
  { value: 'http', label: 'HTTP / HTTPS', desc: '内网 Web 应用，通过子域名或路径反向代理', icon: Globe, port: 0, ready: true },
  { value: 'ssh', label: 'SSH', desc: '浏览器终端 + 文件管理（SFTP）', icon: Console, port: 22, ready: true },
  { value: 'sftp', label: 'SFTP', desc: '仅文件浏览、上传与下载', icon: FolderOpen, port: 22, ready: true },
  { value: 'smb', label: 'SMB', desc: 'Windows 共享文件夹，浏览、上传与下载', icon: Server, port: 445, ready: true },
  { value: 'ftp', label: 'FTP / FTPS', desc: 'FTP 文件服务器，支持显式 / 隐式 TLS', icon: ArrowSwap, port: 21, ready: true },
  { value: 'rdp', label: 'RDP', desc: 'Windows 远程桌面（需部署 guacd）', icon: Desktop, port: 3389, ready: true },
  { value: 'vnc', label: 'VNC', desc: 'VNC 远程桌面（需部署 guacd）', icon: Desktop, port: 5900, ready: true },
]

// 协议能力
export const isDesktopProto = (p: string) => p === 'rdp' || p === 'vnc'
export const isFileProto = (p: string) => p === 'ssh' || p === 'sftp' || p === 'smb' || p === 'ftp'
export const hasKeyAuth = (p: string) => p === 'ssh' || p === 'sftp'

// 默认端口（FTP 隐式 TLS 为 990）
export function defaultPort(p: string, ftpTLS?: string) {
  if (p === 'ftp' && ftpTLS === 'implicit') return 990
  return PROTOCOLS.find(d => d.value === p)?.port || 0
}

export const protocolOf = (a: Pick<WebVpnApp, 'protocol'>): Protocol => (a.protocol || 'http') as Protocol
export const protocolDef = (p: string) => PROTOCOLS.find(d => d.value === (p || 'http')) || PROTOCOLS[0]

export const accessModeOf = (a: Pick<WebVpnApp, 'access_mode'>): AccessMode => (a.access_mode || 'subdomain') as AccessMode

// 实际可用的访问方式（与后端 AccessOf 一致）：未配置域名时子域名不可用，远程协议自动回落到路径
export function accessOf(a: Pick<WebVpnApp, 'access_mode' | 'protocol'>, domain: string) {
  const m = accessModeOf(a)
  const sub = !!domain && (m === 'subdomain' || m === 'both')
  let path = m === 'path' || m === 'both'
  if (!domain && protocolOf(a) !== 'http') path = true
  return { sub, path }
}

// 子域名地址（不含协议）与路径地址（相对 VPN 主域名）
export const subdomainAddr = (name: string, domain: string) => (domain ? `${name}.${domain}` : '')
export const pathAddr = (name: string, protocol: string) => `/web/${protocol || 'http'}/${name}/`

export const CRED_MODE_LABEL: Record<CredMode, string> = {
  prompt: '每次输入',
  stored: '托管凭据',
  passthrough: '门户账号透传',
}

export function parseOptions(s?: string): Record<string, any> {
  if (!s) return {}
  try {
    const o = JSON.parse(s)
    return o && typeof o === 'object' && !Array.isArray(o) ? o : {}
  } catch {
    return {}
  }
}

// 跨域来源：按换行 / 逗号拆分并去空
export function parseCorsOrigins(value?: string | string[] | null) {
  const text = Array.isArray(value) ? value.join('\n') : (value || '')
  return text.split(/[\n,]+/).map(s => s.trim()).filter(Boolean)
}

// /api/v1/webvpn/app/set 的完整载荷（后端整条覆盖，字段必须齐全；凭据明文留空 = 不修改）
export function appPayload(r: WebVpnApp) {
  return {
    id: r.id,
    name: r.name,
    note: r.note,
    backend: r.backend || '',
    host_rewrite: r.host_rewrite || '',
    skip_verify: !!r.skip_verify,
    users: r.users || [],
    groups: r.groups || [],
    allow_path: r.allow_path || [],
    ip_allow_list: r.ip_allow_list || [],
    allow_cross_site: !!r.allow_cross_site,
    cors_allowed_origins: parseCorsOrigins(r.cors_allowed_origins),
    status: r.status,
    access_mode: accessModeOf(r),
    protocol: protocolOf(r),
    target_host: r.target_host || '',
    target_port: r.target_port || 0,
    cred_mode: r.cred_mode || 'prompt',
    cred_user: r.cred_user || '',
    cred_secret: r.cred_secret || '',
    cred_key: r.cred_key || '',
    clear_secret: !!r.clear_secret,
    clear_key: !!r.clear_key,
    host_key_fp: r.host_key_fp || '',
    options: r.options || '',
    allow_upload: !!r.allow_upload,
    allow_download: !!r.allow_download,
    allow_delete: !!r.allow_delete,
    allow_clipboard: !!r.allow_clipboard,
    max_file_size: r.max_file_size || 0,
    record: !!r.record,
    idle_timeout: r.idle_timeout || 0,
    max_sessions_per_user: r.max_sessions_per_user || 0,
  }
}
