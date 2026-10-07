// 用户模块页面共用的小工具
import { formatDate } from '@/utils/format'

// 统一用户名展示：用户名(昵称)，昵称为空时只显示用户名
export function userLabel(username?: string, nickname?: string) {
  if (!username) return ''
  return nickname ? `${username}(${nickname})` : username
}

// 时间格式化；Go 零值时间（0001-01-01）视为空
export function fmtTime(v?: string | null) {
  if (!v || v.startsWith('0001-')) return '—'
  return formatDate(v)
}

// 认证类型
export const USER_TYPES: { label: string; value: string }[] = [
  { label: '本地', value: 'local' },
  { label: 'LDAP', value: 'ldap' },
  { label: 'RADIUS', value: 'radius' },
  { label: '企微', value: 'wxwork' },
  { label: '飞书', value: 'feishu' },
  { label: '钉钉', value: 'dingtalk' },
]
export const typeLabel = (t: string) => USER_TYPES.find(x => x.value === t)?.label || t

// 外部身份源用户（密码、姓名、状态等由身份源管理）
export const isExternalType = (t: string) => ['ldap', 'wxwork', 'feishu', 'dingtalk'].includes(t)

// 字节数：0 显示为 —
export function fmtTraffic(n?: number) {
  if (!n || n <= 0) return '—'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1)
  return `${i ? (n / 1024 ** i).toFixed(2) : n} ${u[i]}`
}
