// 策略相关的类型、默认值与单位换算 / CIDR 校验（策略页与用户组快速建策略共用）

export interface ValData { val: string; note: string }
export interface AclRule { action: 'allow' | 'deny' | string; val: string; protocol: string; port: string; note: string }

export interface PolicyModel {
  id: number
  name: string
  note: string
  status: number
  bandwidth: number
  bandwidth_format: string
  bandwidth_up: number
  bandwidth_up_format: string
  traffic_quota: number
  traffic_quota_format: string
  traffic_reset: string
  allow_lan: boolean
  client_dns: ValData[]
  route_include: ValData[]
  route_exclude: ValData[]
  link_acl: AclRule[]
  ds_include_domains: string
  ds_exclude_domains: string
  enable_fakedns: boolean
  fake_dns_upstream: string
  fake_dns_include: string
  fake_dns_exclude: string
  prefer_ipv6: boolean
  [k: string]: any
}

export const defaultPolicy = (): PolicyModel => ({
  id: 0,
  name: '',
  note: '',
  status: 1,
  bandwidth: 0,
  bandwidth_format: '0',
  bandwidth_up: 0,
  bandwidth_up_format: '0',
  traffic_quota: 0,
  traffic_quota_format: '0',
  traffic_reset: '',
  allow_lan: true,
  client_dns: [{ val: '114.114.114.114', note: '默认dns' }],
  route_include: [{ val: 'all', note: '默认全局代理' }],
  route_exclude: [],
  link_acl: [],
  ds_include_domains: '',
  ds_exclude_domains: '',
  enable_fakedns: false,
  fake_dns_upstream: '',
  fake_dns_include: '',
  fake_dns_exclude: '',
  prefer_ipv6: false,
})

export const newAcl = (): AclRule => ({ action: 'allow', val: '', protocol: 'all', port: '0', note: '' })

// 带宽换算：后端单位 Byte/s；Byte/s × 8 ÷ 10^6 = Mbps
const BW_UNITS: Record<string, number> = { bps: 1, Kbps: 1e3, Mbps: 1e6, Gbps: 1e9, BYTE: 8 }
export function convertBandwidth(v: number | string, from: string, to: string) {
  const n = Number(v) || 0
  return parseFloat(((n * BW_UNITS[from]) / BW_UNITS[to]).toFixed(2))
}

// 流量换算：基于 1024
const TR_UNITS: Record<string, number> = { BYTE: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4 }
export function convertTraffic(v: number | string, from: string, to: string) {
  const n = Number(v) || 0
  return parseFloat(((n * TR_UNITS[from]) / TR_UNITS[to]).toFixed(2))
}

export const RESET_OPTIONS = [
  { label: '不重置', value: '' },
  { label: '按日', value: 'daily' },
  { label: '按周', value: 'weekly' },
  { label: '按月', value: 'monthly' },
]
export const resetLabel = (p?: string) => ({ daily: '每日', weekly: '每周', monthly: '每月' } as Record<string, string>)[p || ''] || ''

// 数字输入过滤：最多两位小数
export const numFilter = (s: string) => s.match(/\d+(\.\d{0,2})?/)?.[0] ?? ''

export interface CidrCheck { valid: boolean; suggestion?: string | null }

// IPv6 CIDR：支持 :: 缩写与末段内嵌 IPv4；maxPrefix 默认 128
export function isValidCIDR6(input: string, maxPrefix = 128): CidrCheck {
  const parts = (input || '').split('/')
  if (parts.length !== 2 || !/^\d+$/.test(parts[1])) return { valid: false }
  const prefix = parseInt(parts[1], 10)
  if (prefix < 0 || prefix > maxPrefix) return { valid: false }
  const dbl = parts[0].split('::')
  if (dbl.length > 2) return { valid: false }
  const isHextet = (h: string) => /^[0-9a-fA-F]{1,4}$/.test(h)
  const isV4 = (s: string) => /^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)$/.test(s)
  const count = (seg: string) => {
    if (seg === '') return 0
    const gs = seg.split(':')
    let n = 0
    for (let i = 0; i < gs.length; i++) {
      const g = gs[i]
      if (g === '') return -1
      if (i === gs.length - 1 && isV4(g)) { n += 2; continue }
      if (!isHextet(g)) return -1
      n += 1
    }
    return n
  }
  if (dbl.length === 2) {
    const l = count(dbl[0]), r = count(dbl[1])
    return { valid: !(l < 0 || r < 0 || l + r > 7) }
  }
  return { valid: count(dbl[0]) === 8 }
}

// IPv4/IPv6 CIDR 校验；IPv4 主机位非零时给出网络地址建议
export function isValidCIDR(input: string): CidrCheck {
  if (input.includes(':')) return isValidCIDR6(input)
  const re = /^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)\/([12]?\d|3[0-2])$/
  if (!re.test(input)) return { valid: false, suggestion: null }
  const [ip, mask] = input.split('/')
  const m = parseInt(mask)
  const bin = ip.split('.').map(p => parseInt(p).toString(2).padStart(8, '0')).join('')
  if (bin.slice(m).includes('1')) {
    const net = bin.slice(0, m).padEnd(32, '0')
    const oct = [0, 1, 2, 3].map(j => parseInt(net.slice(j * 8, j * 8 + 8), 2))
    return { valid: false, suggestion: `${oct.join('.')}/${mask}` }
  }
  return { valid: true, suggestion: null }
}
