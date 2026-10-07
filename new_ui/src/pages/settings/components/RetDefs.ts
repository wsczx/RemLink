// 日志保留页静态定义与计算
export const RET_PRESETS = [
  { days: 7, big: '7', unit: '天' },
  { days: 30, big: '30', unit: '天' },
  { days: 90, big: '90', unit: '天' },
  { days: 180, big: '180', unit: '天' },
  { days: 365, big: '1', unit: '年' },
  { days: 0, big: '∞', unit: '永久' },
]

// "05:00" / "5:00" → 5
export function hourOf(t?: string) {
  const h = parseInt(String(t || '').split(':')[0], 10)
  return Number.isNaN(h) ? -1 : h
}

// 下一次清理的时间点（本地时间）
export function nextCleanup(t?: string) {
  const h = hourOf(t)
  if (h < 0) return null
  const d = new Date()
  d.setHours(h, 0, 0, 0)
  if (d.getTime() <= Date.now()) d.setDate(d.getDate() + 1)
  return d
}

export function intervalText(v?: number) {
  if (v == null) return '未知'
  if (v < 0) return '已关闭异步审计'
  if (v === 0) return '不去重，记录每一次访问'
  if (v % 60 === 0) return `${v / 60} 分钟内重复访问只记一次`
  return `${v} 秒内重复访问只记一次`
}
