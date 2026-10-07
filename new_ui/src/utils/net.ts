// 网络地址相关的小工具

// 去掉端口：1.2.3.4:5678 / [::1]:443 → 1.2.3.4 / ::1
export function hostOf(addr?: string) {
  if (!addr) return ''
  const m = addr.match(/^\[([^\]]+)\](?::\d+)?$/)
  if (m) return m[1]
  const parts = addr.split(':')
  return parts.length === 2 ? parts[0] : addr
}

// 私有/保留地址（RFC1918、CGNAT、回环、链路本地、ULA）
export function isPrivateIp(ip: string) {
  if (!ip) return false
  if (ip.includes(':')) return /^(::1$|fc|fd|fe80)/i.test(ip)
  const [a, b] = ip.split('.').map(Number)
  return a === 10 || a === 127 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) ||
    (a === 100 && b >= 64 && b <= 127) || (a === 169 && b === 254)
}

// IPv4 取 /24 网段，用于聚合分配地址
export function subnet24(ip?: string) {
  if (!ip || ip.includes(':')) return ip || ''
  const p = ip.split('.')
  return p.length === 4 ? `${p[0]}.${p[1]}.${p[2]}.0/24` : ip
}

const regionNames = (() => {
  try { return new Intl.DisplayNames(['zh-CN'], { type: 'region' }) } catch { return null }
})()

// ISO 3166-1 alpha-2 → 中文国家/地区名
export function countryName(code?: string) {
  if (!code) return ''
  try { return regionNames?.of(code.toUpperCase()) || code } catch { return code }
}
