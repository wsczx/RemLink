import dayjs from 'dayjs'

export const formatDate = (v?: string | number | Date | null, fmt = 'YYYY-MM-DD HH:mm:ss') =>
  v ? dayjs(v).format(fmt) : ''

export function formatBytes(n: number) {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1)
  return `${(n / 1024 ** i).toFixed(i ? 2 : 0)} ${u[i]}`
}

export function formatCount(n: number) {
  if (n >= 1e6) return `${(n / 1e6).toFixed(2)}M`
  if (n >= 1e3) return `${(n / 1e3).toFixed(2)}K`
  return String(n)
}
