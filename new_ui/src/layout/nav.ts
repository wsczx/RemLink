import type { Component } from 'vue'
import Board from '~icons/fluent/board-20-regular'
import PersonAvailable from '~icons/fluent/person-available-20-regular'
import People from '~icons/fluent/people-20-regular'
import ShieldKeyhole from '~icons/fluent/shield-keyhole-20-regular'
import Certificate from '~icons/fluent/certificate-20-regular'
import History from '~icons/fluent/history-20-regular'
import Settings from '~icons/fluent/settings-20-regular'
import Megaphone from '~icons/fluent/megaphone-20-regular'
import Stethoscope from '~icons/fluent/stethoscope-20-regular'

export interface NavLeaf {
  label: string
  to?: string
  // 外部诊断工具：先探测再新窗口打开
  external?: string
  // 额外归属本项的路由前缀
  match?: string[]
}

export interface NavNode {
  key: string
  label: string
  icon: Component
  to?: string
  children?: NavLeaf[]
}

// 两级侧边栏：一级为模块（可展开），二级为具体页面
export const adminNav: NavNode[] = [
  { key: 'home', label: '仪表盘', icon: Board, to: '/admin/home' },
  { key: 'online', label: '在线会话', icon: PersonAvailable, to: '/admin/user/online' },
  { key: 'identity', label: '用户与身份', icon: People, children: [
    { label: '用户', to: '/admin/user/list' },
    { label: '用户组', to: '/admin/group/list' },
    { label: '认证源', to: '/admin/provider/list', match: ['/admin/provider/'] },
    { label: '锁定管理', to: '/admin/user/lockmanager' },
  ] },
  { key: 'access', label: '访问控制', icon: ShieldKeyhole, children: [
    { label: '策略', to: '/admin/policy/list' },
    { label: 'IP 映射', to: '/admin/user/ip_map' },
    { label: 'WebVPN 应用', to: '/admin/webvpn' },
  ] },
  { key: 'cert', label: '证书', icon: Certificate, children: [
    { label: '服务器证书', to: '/admin/cert/server' },
    { label: '自动签发', to: '/admin/cert/acme' },
    { label: '客户端证书', to: '/admin/cert/client' },
  ] },
  { key: 'audit', label: '审计日志', icon: History, children: [
    { label: '接入链路', to: '/admin/audit/flow' },
    { label: '用户行为', to: '/admin/audit/activity' },
    { label: '访问日志', to: '/admin/audit/access' },
    { label: 'WebVPN 访问', to: '/admin/audit/webvpn' },
    { label: '管理操作', to: '/admin/audit/admin' },
    { label: '系统日志', to: '/admin/audit/syslog' },
  ] },
  { key: 'system', label: '系统设置', icon: Settings, children: [
    { label: '系统信息', to: '/admin/settings/system' },
    { label: '服务配置', to: '/admin/settings/service' },
    { label: '客户端 Profile', to: '/admin/settings/profile' },
    { label: '数据备份', to: '/admin/settings/backup' },
    { label: '管理员账号', to: '/admin/settings/account' },
    { label: '数据加密', to: '/admin/settings/encryption' },
    { label: 'IP 地理库', to: '/admin/settings/geoip' },
  ] },
  { key: 'notify', label: '通知与品牌', icon: Megaphone, children: [
    { label: '邮件', to: '/admin/settings/mail' },
    { label: '短信', to: '/admin/settings/sms' },
    { label: '品牌', to: '/admin/settings/brand' },
    { label: '用户门户', to: '/admin/settings/portal' },
    { label: '访问地址与模板', to: '/admin/settings/templates' },
    { label: '日志保留', to: '/admin/settings/retention' },
  ] },
  { key: 'debug', label: '诊断', icon: Stethoscope, children: [
    { label: 'pprof', external: '/debug/pprof/' },
    { label: 'statsviz', external: '/debug/statsviz/' },
  ] },
]

// /admin/webvpn 只精确匹配，避免把 /admin/webvpn/audit 也算进去
export function isLeafActive(path: string, item: { to?: string; match?: string[] }) {
  if (!item.to) return false
  if (path === item.to) return true
  if (item.match?.some(p => path.startsWith(p))) return true
  return item.to !== '/admin/webvpn' && path.startsWith(item.to + '/')
}

// 当前路由对应的一级 / 二级（用于面包屑与自动展开）
export function findNav(nav: NavNode[], path: string) {
  for (const n of nav) {
    if (n.to && isLeafActive(path, n)) return { node: n, leaf: null as NavLeaf | null }
    const leaf = n.children?.find(c => isLeafActive(path, c))
    if (leaf) return { node: n, leaf }
  }
  return null
}
