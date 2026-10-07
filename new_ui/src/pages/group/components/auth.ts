// 用户组认证流程：步骤类型、默认配置与工具函数

export interface AuthStep { _key: string; type: string; provider: string; config: Record<string, any> }

export const AUTH_LABELS: Record<string, string> = {
  local: '本地密码', cert: 'TLS证书', ldap: 'LDAP', radius: 'RADIUS', otp: 'OTP验证',
  wxwork: '企微', feishu: '飞书', dingtalk: '钉钉', sms: '短信验证',
}
export const AUTH_OPTIONS = Object.entries(AUTH_LABELS).map(([value, label]) => ({ label, value }))

// 需要关联「认证源」的步骤类型
export const PROVIDER_TYPES = ['ldap', 'radius', 'wxwork', 'feishu', 'dingtalk']

const STEP_DEFAULTS: Record<string, Record<string, any>> = {
  local: {}, otp: {}, cert: {}, sms: {},
  radius: { addr: '', secret: '', nasip: '' },
  ldap: {
    addr: '', tls: false, tls_verify: false, bind_name: '', bind_pwd: '', base_dn: '',
    object_class: 'person', search_attr: 'sAMAccountName', member_of: '', sync_user_status: false, enable_otp: false,
  },
  wxwork: { corp_id: '', agent_id: '', secret: '', use_default_browser: false, allowed_departments: '' },
  feishu: { app_id: '', app_secret: '', use_default_browser: false, allowed_departments: '' },
}
export const stepDefaults = (type: string) => ({ ...(STEP_DEFAULTS[type] || {}) })

let seq = 1
export const newStep = (type = 'local'): AuthStep => ({ _key: 's' + seq++, type, provider: '', config: stepDefaults(type) })
export const normalizeStep = (s: any): AuthStep => ({
  _key: 's' + seq++,
  type: s.type || 'local',
  provider: s.provider || '',
  config: { ...stepDefaults(s.type), ...(s.config || {}) },
})

// SSO（企微 / 飞书）与凭据步骤（本地密码 / LDAP / RADIUS）互斥
export function authConflict(steps: AuthStep[]) {
  const t = steps.map(s => s.type)
  const sso = t.some(x => x === 'wxwork' || x === 'feishu')
  const cred = t.some(x => x === 'local' || x === 'ldap' || x === 'radius')
  return sso && cred
    ? 'SSO 认证（企业微信/飞书）不能与本地密码、LDAP、RADIUS 等凭据认证同时使用：SSO 由第三方身份提供，不产生登录密码，无法与需要密码的步骤组合。如需二次验证请改用 OTP。'
    : ''
}
