// 审计事件流的公共工具：时间范围预设、相对时间、筛选字段定义、CSV/文本下载
import dayjs from 'dayjs'

export type RangeMode = 'date' | 'datetime'
// 时间范围：预设 key 或自定义区间（custom 为格式化后的字符串）
export interface TimeSel { preset: string; custom: [string, string] | null }

interface Preset { key: string; label: string; short: string; amount?: number; unit?: 'hour' | 'day'; modes: RangeMode[] }
export const PRESETS: Preset[] = [
  { key: 'all', label: '全部时间', short: '全部', modes: ['date', 'datetime'] },
  { key: '1h', label: '近 1 小时', short: '1h', amount: 1, unit: 'hour', modes: ['datetime'] },
  { key: '24h', label: '近 24 小时', short: '24h', amount: 24, unit: 'hour', modes: ['datetime'] },
  { key: 'today', label: '今天', short: '今天', amount: 0, unit: 'day', modes: ['date', 'datetime'] },
  { key: '7d', label: '近 7 天', short: '7d', amount: 6, unit: 'day', modes: ['date', 'datetime'] },
  { key: '30d', label: '近 30 天', short: '30d', amount: 29, unit: 'day', modes: ['date', 'datetime'] },
  { key: '90d', label: '近 90 天', short: '90d', amount: 89, unit: 'day', modes: ['date', 'datetime'] },
]

const FMT: Record<RangeMode, string> = { date: 'YYYY-MM-DD', datetime: 'YYYY-MM-DD HH:mm:ss' }

// 解析为后端需要的 [开始, 结束]；全部时间返回 null。相对预设在每次请求时重新计算（自动刷新时窗口随之滑动）
export function resolveRange(sel: TimeSel, mode: RangeMode): [string, string] | null {
  if (sel.preset === 'custom') return sel.custom
  const p = PRESETS.find(x => x.key === sel.preset)
  if (!p || !p.unit) return null
  const now = dayjs()
  const start = p.unit === 'hour' ? now.subtract(p.amount!, 'hour') : now.subtract(p.amount!, 'day').startOf('day')
  const end = mode === 'date' ? now : now.endOf('day')
  return [start.format(FMT[mode]), (p.unit === 'hour' ? now : end).format(FMT[mode])]
}

export function rangeLabel(sel: TimeSel, mode: RangeMode) {
  if (sel.preset === 'custom' && sel.custom) {
    const f = mode === 'date' ? 'YYYY-MM-DD' : 'MM-DD HH:mm'
    const [a, b] = sel.custom.map(v => dayjs(v).format(f))
    return a === b ? a : `${a} → ${b}`
  }
  return PRESETS.find(x => x.key === sel.preset)?.label || '全部时间'
}

// 某一天对应的区间（柱状图点选）
export const dayRange = (day: string, mode: RangeMode): [string, string] =>
  mode === 'date' ? [day, day] : [`${day} 00:00:00`, `${day} 23:59:59`]

// 相对时间：刚刚 / N 分钟前 / N 小时前 / N 天前
export function relTime(v?: string) {
  if (!v) return ''
  const s = dayjs().diff(dayjs(v), 'second')
  if (s < 0) return dayjs(v).format('MM-DD HH:mm')
  if (s < 60) return '刚刚'
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  if (s < 86400 * 30) return `${Math.floor(s / 86400)} 天前`
  return dayjs(v).format('YYYY-MM-DD')
}

// 筛选条件定义（FilterChips 使用）
export interface FilterOption { label: string; value: string | number }
export interface FilterField {
  key: string
  label: string
  type: 'text' | 'select'
  options?: FilterOption[]
  placeholder?: string
  mono?: boolean
  maxlength?: number
  // 返回错误文案则不允许应用
  validate?: (v: string) => string
}
export type FilterValues = Record<string, string | number | null>

export const isEmpty = (v: unknown) => v === null || v === undefined || v === ''

// 触发浏览器下载
export function downloadText(text: string, name: string, type = 'text/plain;charset=utf-8') {
  const url = URL.createObjectURL(new Blob([text], { type }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

// 展开行中的字段
export interface EventField {
  label: string
  value?: string | number | null
  mono?: boolean
  // 显示国旗（CountryFlag code）
  flag?: string
  // 占满整行（长文本）
  wide?: boolean
  onFilter?: () => void
}
