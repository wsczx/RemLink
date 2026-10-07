// 浏览器端解析 PEM 证书（仅取展示所需字段：主题 CN / SAN / 颁发者 / 有效期），不做任何校验
// 只用于上传前预览，最终以后端 tls.X509KeyPair 校验结果为准

// 服务器证书槽位：主证书 / WebVPN 泛域名证书
export type CertSlot = 'main' | 'wild'

export interface CertInfo {
  cn: string
  org: string
  sans: string[]
  issuerCn: string
  issuerOrg: string
  notBefore: Date | null
  notAfter: Date | null
  selfSigned: boolean
  chainLen: number
}

interface Node { tag: number; start: number; len: number; end: number }

function readNode(b: Uint8Array, pos: number): Node {
  const tag = b[pos]
  let len = b[pos + 1]
  let start = pos + 2
  if (len & 0x80) {
    const n = len & 0x7f
    len = 0
    for (let i = 0; i < n; i++) len = len * 256 + b[pos + 2 + i]
    start += n
  }
  if (start + len > b.length) throw new Error('DER 长度越界')
  return { tag, start, len, end: start + len }
}

function children(b: Uint8Array, n: Node) {
  const out: Node[] = []
  let p = n.start
  while (p < n.end) {
    const c = readNode(b, p)
    out.push(c)
    p = c.end
  }
  return out
}

const OID = { cn: '550403', org: '55040a', san: '551d11' }
const hex = (b: Uint8Array, n: Node) => Array.from(b.subarray(n.start, n.end), x => x.toString(16).padStart(2, '0')).join('')
const text = (b: Uint8Array, n: Node) => new TextDecoder().decode(b.subarray(n.start, n.end))

// Name ::= SEQUENCE OF SET OF SEQUENCE { OID, value }
function parseName(b: Uint8Array, n: Node) {
  const r: Record<string, string> = {}
  for (const set of children(b, n)) {
    for (const atv of children(b, set)) {
      const [oid, val] = children(b, atv)
      if (oid && val) r[hex(b, oid)] = text(b, val)
    }
  }
  return { cn: r[OID.cn] || '', org: r[OID.org] || '', raw: hex(b, n) }
}

function parseTime(b: Uint8Array, n: Node): Date | null {
  const s = text(b, n)
  const m = n.tag === 0x17
    ? s.match(/^(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})?Z$/)
    : s.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})?(?:\.\d+)?Z$/)
  if (!m) return null
  let y = Number(m[1])
  if (n.tag === 0x17) y += y < 50 ? 2000 : 1900
  return new Date(Date.UTC(y, Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]), Number(m[6] || 0)))
}

function parseIp(b: Uint8Array, n: Node) {
  const a = Array.from(b.subarray(n.start, n.end))
  if (a.length === 4) return a.join('.')
  if (a.length === 16) {
    const g: string[] = []
    for (let i = 0; i < 16; i += 2) g.push(((a[i] << 8) | a[i + 1]).toString(16))
    return g.join(':')
  }
  return ''
}

function parseDer(b: Uint8Array): Omit<CertInfo, 'chainLen'> {
  const cert = readNode(b, 0)
  const tbs = children(b, cert)[0]
  const f = children(b, tbs)
  let i = f[0].tag === 0xa0 ? 1 : 0 // 可选 version
  i += 2 // serial、signature
  const issuer = parseName(b, f[i++])
  const [nb, na] = children(b, f[i++])
  const subject = parseName(b, f[i++])
  i++ // subjectPublicKeyInfo
  const sans: string[] = []
  const ext = f.slice(i).find(x => x.tag === 0xa3)
  if (ext) {
    for (const e of children(b, children(b, ext)[0])) {
      const parts = children(b, e)
      if (hex(b, parts[0]) !== OID.san) continue
      const oct = parts[parts.length - 1]
      for (const gn of children(b, readNode(b, oct.start))) {
        if (gn.tag === 0x82) sans.push(text(b, gn))
        else if (gn.tag === 0x87) { const ip = parseIp(b, gn); if (ip) sans.push(ip) }
      }
    }
  }
  return {
    cn: subject.cn,
    org: subject.org,
    sans,
    issuerCn: issuer.cn,
    issuerOrg: issuer.org,
    notBefore: nb ? parseTime(b, nb) : null,
    notAfter: na ? parseTime(b, na) : null,
    selfSigned: issuer.raw === subject.raw,
  }
}

// 解析 PEM 文本中的第一张证书（叶子证书），并统计链长度
export function parsePemCert(pem: string): CertInfo {
  const blocks = pem.match(/-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----/g)
  if (!blocks?.length) throw new Error('未找到 PEM 证书块（-----BEGIN CERTIFICATE-----）')
  const b64 = blocks[0].replace(/-----(BEGIN|END) CERTIFICATE-----|\s/g, '')
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let k = 0; k < bin.length; k++) bytes[k] = bin.charCodeAt(k)
  return { ...parseDer(bytes), chainLen: blocks.length }
}

// 证书 SAN（无 SAN 时用 CN）中是否包含某名称，按字面比较（如 *.example.com）
export function certCovers(info: CertInfo, host: string) {
  const names = info.sans.length ? info.sans : [info.cn]
  return names.some(n => n.toLowerCase() === host.toLowerCase())
}

// 剩余天数 → 语义色
export function daysLeft(d: Date | string | null | undefined) {
  if (!d) return null
  const t = typeof d === 'string' ? new Date(d).getTime() : d.getTime()
  if (Number.isNaN(t)) return null
  return Math.floor((t - Date.now()) / 86400000)
}

export function expiryTone(days: number | null): 'success' | 'warning' | 'error' | 'muted' {
  if (days == null) return 'muted'
  if (days < 0) return 'error'
  if (days <= 7) return 'error'
  if (days <= 30) return 'warning'
  return 'success'
}

export function expiryText(days: number | null) {
  if (days == null) return '未知'
  if (days < 0) return `已过期 ${-days} 天`
  if (days === 0) return '今天到期'
  return `剩余 ${days} 天`
}

// 证书卡展示记录（来自服务端 /set/cert/info）
export interface CertRecord { info: CertInfo; fileName: string; at: number }
