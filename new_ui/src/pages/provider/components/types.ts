// 认证源类型元数据与各类型配置默认值（字段与后端 provider config 一一对应）
import type { Component } from 'vue'
import Server from '~icons/fluent/server-20-regular'
import ShieldLock from '~icons/fluent/shield-lock-20-regular'
import Building from '~icons/fluent/building-20-regular'
import ChatMultiple from '~icons/fluent/chat-multiple-20-regular'
import PeopleTeam from '~icons/fluent/people-team-20-regular'

export type ProviderType = 'ldap' | 'radius' | 'wxwork' | 'feishu' | 'dingtalk'

export interface ProviderMeta {
  value: ProviderType
  label: string
  full: string
  desc: string
  /** 适用场景（面向管理员任务的说明） */
  scene: string
  /** directory：账号密码类；im：办公平台扫码类 */
  kind: 'directory' | 'im'
  icon: Component
  caps: string[]
}

export const PROVIDER_TYPES: ProviderMeta[] = [
  {
    value: 'ldap', label: 'LDAP', full: 'LDAP / Active Directory', kind: 'directory', icon: Server,
    desc: '对接 OpenLDAP、Windows AD 等目录服务，用目录账号密码登录。',
    scene: '公司已有 AD 域或 LDAP 目录，希望员工沿用域账号登录 VPN',
    caps: ['账号密码', 'OTP', '用户同步'],
  },
  {
    value: 'radius', label: 'RADIUS', full: 'RADIUS', kind: 'directory', icon: ShieldLock,
    desc: '把登录请求转发给 RADIUS 服务器校验。',
    scene: '已有 NPS、FreeRADIUS 或令牌认证（AAA）体系',
    caps: ['账号密码'],
  },
  {
    value: 'wxwork', label: '企微', full: '企业微信', kind: 'im', icon: Building,
    desc: '企业微信扫码登录，可按部门限制并同步通讯录。',
    scene: '员工日常使用企业微信办公',
    caps: ['扫码登录', '部门限制', '用户同步'],
  },
  {
    value: 'feishu', label: '飞书', full: '飞书', kind: 'im', icon: ChatMultiple,
    desc: '飞书扫码登录，可按部门限制并同步通讯录。',
    scene: '员工日常使用飞书办公，或需要 Web 认证扫码',
    caps: ['扫码登录', '部门限制', '用户同步'],
  },
  {
    value: 'dingtalk', label: '钉钉', full: '钉钉', kind: 'im', icon: PeopleTeam,
    desc: '钉钉扫码登录，可按部门限制并同步通讯录。',
    scene: '员工日常使用钉钉办公',
    caps: ['扫码登录', '部门限制', '用户同步'],
  },
]

export const typeMeta = (t: string) => PROVIDER_TYPES.find(p => p.value === t)

export const CONFIG_DEFAULTS: Record<ProviderType, Record<string, any>> = {
  ldap: {
    addr: '', tls: false, tls_verify: false, ldaps: false, bind_name: '', bind_pwd: '', base_dn: '',
    object_class: 'person', search_attr: 'sAMAccountName',
    member_of: '', sync_user_status: false, enable_otp: false, sync_users: false,
  },
  radius: { addr: '', secret: '', nasip: '' },
  wxwork: { corp_id: '', agent_id: '', secret: '', use_default_browser: false, allowed_departments: '', blocked_userids: '', sync_users: false, verify_file_name: '', verify_file_content: '' },
  feishu: { app_id: '', app_secret: '', use_default_browser: false, allowed_departments: '', blocked_userids: '', sync_users: false },
  dingtalk: { client_id: '', client_secret: '', use_default_browser: false, allowed_departments: '', blocked_userids: '', sync_users: false },
}

export const defaultConfig = (t: string) => ({ ...(CONFIG_DEFAULTS[t as ProviderType] || {}) })

// 支持「测试登录」的类型（账号密码类）
export const isTestable = (t: string) => t === 'ldap' || t === 'radius'
// 支持「同步用户」的类型
export const isSyncable = (t: string) => t === 'ldap' || t === 'wxwork' || t === 'feishu' || t === 'dingtalk'
export const isOAuth = (t: string) => t === 'wxwork' || t === 'feishu' || t === 'dingtalk'

export interface ProviderRow {
  id: number
  name: string
  type: string
  status: number
  config?: any
  created_at?: string
  updated_at?: string
}

// 后端 config 可能是对象或 JSON 字符串
export function parseCfg(c: any): Record<string, any> {
  try { return typeof c === 'string' ? JSON.parse(c) : (c || {}) } catch { return {} }
}

// 连接摘要（列表 / 概览用）
export function connSummary(type: string, c: Record<string, any>) {
  switch (type) {
    case 'ldap': return c.addr ? `${c.ldaps ? 'ldaps' : 'ldap'}://${c.addr}${c.tls ? ' · StartTLS' : ''}` : ''
    case 'radius': return c.addr || ''
    case 'wxwork': return c.corp_id ? `企业 ID ${c.corp_id}` : ''
    case 'feishu': return c.app_id ? `App ID ${c.app_id}` : ''
    case 'dingtalk': return c.client_id ? `AppKey ${c.client_id}` : ''
  }
  return ''
}

export interface CallbackItem { label: string; url: string; warn?: boolean; note?: string }

// 第三方开放平台需要填写的回调地址；base 优先用后端下发的 callbackBase（VPN 服务端口）
export function callbacksOf(type: string, base: string): CallbackItem[] {
  const b = base || window.location.origin
  switch (type) {
    case 'wxwork': return [{ label: '企业微信后台「企业微信登录 - 可信域名/回调」', url: `${b}/WXAuth/callback` }]
    case 'feishu': return [
      { label: '飞书开放平台「安全设置 - 重定向 URL」', url: `${b}/FeishuAuth/callback` },
      {
        label: '使用 Web 认证（浏览器弹窗扫码）时，在「重定向 URL」中补加', url: `${b}/web-auth/sso-callback`, warn: true,
        note: '飞书按完整路径精确匹配回调，门户/原生与 Web 认证路径不同，两条都需配置。',
      },
    ]
    case 'dingtalk': return [{ label: '钉钉开放平台「登录与分享 - 回调地址」', url: `${b}/DingtalkAuth/callback` }]
  }
  return []
}

// 用户组引用：从 /api/v1/group/list 的 auth_profile.step[].provider 统计
export interface GroupRef { id: number; name: string; status: number }

export function groupRefs(groups: any[]): Record<string, GroupRef[]> {
  const m: Record<string, GroupRef[]> = {}
  for (const g of groups) {
    let prof = g.auth_profile
    if (typeof prof === 'string') { try { prof = JSON.parse(prof) } catch { prof = null } }
    const names = new Set<string>((prof?.step || []).map((s: any) => s.provider).filter(Boolean))
    names.forEach(n => { (m[n] ||= []).push({ id: g.id, name: g.name, status: g.status }) })
  }
  return m
}
