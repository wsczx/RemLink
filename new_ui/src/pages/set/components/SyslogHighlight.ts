// 日志行语法高亮：先转义 HTML，再按规则包裹 <span class="slg-*">；配色在 SyslogView.vue 里用 token 定义

const escapeHtml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')

const reEscape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

// 顺序即优先级
const PATTERNS: [string, string][] = [
  ['statusError', '((?:DTLS.*?handshake error|DTLS.*?握手失败|\\bhandshake error\\b|握手失败|握手错误|MasterSecret is nil|tun Read err.*?file already closed))'],
  ['statusWarn', '((?:read hdata:.*?|read tun:.*?|i/o timeout|context deadline exceeded))'],
  ['statusSuccess', '((?:WebAuth认证已完成|用户通过证书认证|证书自动认证|\\blogin\\b|\\bAcquireIp\\b|\\bconnect\\b))'],
  ['group', '((?:组|group)\\s*[:=]?\\s*[\\w.-]+)'],
  ['identity', '((?:username|user_name|nickname|realname|name|user|用户|姓名|login)\\s*(?:[:=]\\s*)?[\\w.-]+(?:\\([\\w.-]+\\))?|\\b[\\w.-]+\\([\\w.-]+\\))'],
  ['url', '(https?:\\/\\/[\\w\\-]+(?:\\.[\\w\\-]+)+(?:[\\w\\-\\.,@?^=%&amp;:/~\\+#]*[\\w\\-@?^=%&amp;/~\\+#])?)'],
  ['address', '((?:(?:\\b(?:\\d{1,3}\\.){3}\\d{1,3}(?::\\d{1,5})?\\b)|(?:\\b[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){2,7}\\b))(?:\\/\\d{1,3})?)'],
  ['domain', '(\\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\\.)+(?:[a-z]{2,63}|xn--[a-z0-9-]+)\\b)'],
  ['network', '((?:NAT6?|IPv6|DTLS|TUN|TAP|macvtap|nftables|WebVPN)\\b)'],
  ['keyword', '((?:\\b(?:Error|Exception|Failed|Denied|Timeout|Panic|Fatal)\\b|失败|错误|异常|拒绝|超时|中断|不可用|中止|致命|\\b(?:Warning|Warn)\\b|警告|注意))'],
]
const ERROR_WORDS = new Set(['error', 'exception', 'failed', 'denied', 'timeout', 'panic', 'fatal', '失败', '错误', '异常', '拒绝', '超时', '中断', '不可用', '中止', '致命'])
const CLS: Record<string, string> = {
  statusError: 'slg-err', statusWarn: 'slg-warn', statusSuccess: 'slg-ok', group: 'slg-group', identity: 'slg-id',
  address: 'slg-addr', domain: 'slg-domain', network: 'slg-net',
}

// 规则固定、关键字变化不频繁：按关键字缓存正则
const cache = new Map<string, { re: RegExp; types: string[] }>()
function compile(keyword: string) {
  let c = cache.get(keyword)
  if (c) return c
  const parts: [string, string][] = keyword ? [['search', `(${reEscape(escapeHtml(keyword))})`], ...PATTERNS] : PATTERNS
  c = { re: new RegExp(parts.map(p => p[1]).join('|'), 'gi'), types: parts.map(p => p[0]) }
  if (cache.size > 20) cache.clear()
  cache.set(keyword, c)
  return c
}

export function highlightLog(msg: string, keyword = '') {
  const { re, types } = compile(keyword)
  re.lastIndex = 0
  return escapeHtml(msg || '').replace(re, (m, ...groups) => {
    const type = types[groups.findIndex(g => g !== undefined)]
    if (type === 'search') return `<mark class="slg-mark">${m}</mark>`
    if (type === 'url') return `<a href="${m}" target="_blank" rel="noopener noreferrer" class="slg-url">${m}</a>`
    if (type === 'keyword') return `<span class="${ERROR_WORDS.has(m.toLowerCase()) ? 'slg-err' : 'slg-warn'}">${m}</span>`
    return CLS[type] ? `<span class="${CLS[type]}">${m}</span>` : m
  })
}
