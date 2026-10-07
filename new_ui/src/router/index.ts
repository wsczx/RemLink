import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ui } from '@/utils/ui'

const adminChildren: RouteRecordRaw[] = [
  { path: 'home', component: () => import('@/pages/Home.vue'), meta: { title: '仪表盘' } },
  { path: 'user/online', component: () => import('@/pages/user/Online.vue'), meta: { title: '在线会话' } },

  // 用户与身份
  { path: 'user/list', component: () => import('@/pages/user/List.vue'), meta: { title: '用户' } },
  { path: 'group/list', component: () => import('@/pages/group/List.vue'), meta: { title: '用户组' } },
  { path: 'provider/list', component: () => import('@/pages/provider/List.vue'), meta: { title: '认证源' } },
  { path: 'provider/:id', component: () => import('@/pages/provider/Detail.vue'), meta: { title: '认证源' } },
  { path: 'user/lockmanager', component: () => import('@/pages/user/LockManager.vue'), meta: { title: '锁定管理' } },

  // 访问控制
  { path: 'policy/list', component: () => import('@/pages/policy/List.vue'), meta: { title: '策略' } },
  { path: 'user/ip_map', component: () => import('@/pages/user/IpMap.vue'), meta: { title: 'IP 映射' } },
  { path: 'webvpn', component: () => import('@/pages/webvpn/AppList.vue'), meta: { title: 'WebVPN 应用' } },

  // 证书
  { path: 'cert/server', component: () => import('@/pages/cert/Server.vue'), meta: { title: '服务器证书' } },
  { path: 'cert/acme', component: () => import('@/pages/cert/Acme.vue'), meta: { title: '自动签发' } },
  { path: 'cert/client', component: () => import('@/pages/cert/Client.vue'), meta: { title: '客户端证书' } },

  // 审计日志
  { path: 'audit/flow', component: () => import('@/pages/log/AccessFlow.vue'), meta: { title: '接入链路' } },
  { path: 'audit/activity', component: () => import('@/pages/audit/Activity.vue'), meta: { title: '用户行为' } },
  { path: 'audit/access', component: () => import('@/pages/audit/Access.vue'), meta: { title: '访问日志' } },
  { path: 'audit/webvpn', component: () => import('@/pages/webvpn/Visits.vue'), meta: { title: 'WebVPN 访问' } },
  { path: 'audit/admin', component: () => import('@/pages/audit/AdminOps.vue'), meta: { title: '管理操作' } },
  { path: 'audit/syslog', component: () => import('@/pages/set/Syslog.vue'), meta: { title: '系统日志' } },

  // 系统设置
  { path: 'settings/system', component: () => import('@/pages/settings/System.vue'), meta: { title: '系统信息' } },
  { path: 'settings/service', component: () => import('@/pages/settings/Service.vue'), meta: { title: '服务配置' } },
  { path: 'settings/profile', component: () => import('@/pages/settings/Profile.vue'), meta: { title: '客户端 Profile' } },
  { path: 'settings/backup', component: () => import('@/pages/settings/Backup.vue'), meta: { title: '数据备份' } },
  { path: 'settings/account', component: () => import('@/pages/settings/Account.vue'), meta: { title: '管理员账号' } },
  { path: 'settings/encryption', component: () => import('@/pages/settings/Encryption.vue'), meta: { title: '数据加密' } },
  { path: 'settings/geoip', component: () => import('@/pages/settings/Geoip.vue'), meta: { title: 'IP 地理库' } },

  // 通知与品牌
  { path: 'settings/mail', component: () => import('@/pages/settings/Mail.vue'), meta: { title: '邮件' } },
  { path: 'settings/sms', component: () => import('@/pages/settings/Sms.vue'), meta: { title: '短信' } },
  { path: 'settings/brand', component: () => import('@/pages/settings/Brand.vue'), meta: { title: '品牌' } },
  { path: 'settings/portal', component: () => import('@/pages/settings/PortalHome.vue'), meta: { title: '用户门户' } },
  { path: 'settings/templates', component: () => import('@/pages/settings/Templates.vue'), meta: { title: '访问地址与模板' } },
  { path: 'settings/retention', component: () => import('@/pages/settings/Retention.vue'), meta: { title: '日志保留' } },

  // 旧路径兼容
  { path: 'set/system', redirect: '/admin/settings/system' },
  { path: 'set/soft', redirect: '/admin/settings/service' },
  { path: 'set/security', redirect: '/admin/settings/account' },
  { path: 'set/cert', redirect: '/admin/cert/server' },
  { path: 'set/other', redirect: '/admin/settings/mail' },
  { path: 'set/syslog', redirect: '/admin/audit/syslog' },
  { path: 'set/audit', redirect: to => ({ path: to.query.tabname === 'admin_op_log' ? '/admin/audit/admin' : to.query.tabname === 'access_audit' ? '/admin/audit/access' : '/admin/audit/activity' }) },
  { path: 'log/access', redirect: '/admin/audit/flow' },
  { path: 'webvpn/audit', redirect: '/admin/audit/webvpn' },
  { path: 'webvpn/remote', redirect: '/admin/audit/webvpn' },
]

// 用户门户：独立外壳（侧边栏），未登录时外壳内显示登录
const portalChildren: RouteRecordRaw[] = [
  { path: '', component: () => import('@/pages/portal/views/Overview.vue'), meta: { public: true, title: '概览' } },
  { path: 'devices', component: () => import('@/pages/portal/views/Devices.vue'), meta: { public: true, title: '我的设备' } },
  { path: 'clients', component: () => import('@/pages/portal/views/Clients.vue'), meta: { public: true, title: '客户端下载' } },
  { path: 'apps', component: () => import('@/pages/portal/views/Apps.vue'), meta: { public: true, title: '应用' } },
  { path: 'access', component: () => import('@/pages/portal/views/Access.vue'), meta: { public: true, title: '访问权限' } },
  { path: 'security', component: () => import('@/pages/portal/views/Security.vue'), meta: { public: true, title: '账号安全' } },
  { path: 'certs', component: () => import('@/pages/portal/views/Certs.vue'), meta: { public: true, title: '我的证书' } },
]

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/admin/home' },
  { path: '/login', component: () => import('@/pages/Login.vue'), meta: { public: true } },
  { path: '/portal', component: () => import('@/layout/PortalLayout.vue'), meta: { public: true, title: '用户门户' }, children: portalChildren },
  { path: '/web-auth', component: () => import('@/pages/WebAuth.vue'), meta: { public: true, title: 'Web 认证' } },
  // WebVPN 非 HTTP 应用（SSH/SFTP）远程访问页：运行在应用子域，独立外壳
  { path: '/webvpn/remote', component: () => import('@/pages/webvpn-remote/Remote.vue'), meta: { public: true, title: '远程访问' } },
  { path: '/web-auth/continue', component: () => import('@/pages/WebAuth.vue'), meta: { public: true, title: 'Web 认证' } },
  {
    path: '/admin',
    component: () => import('@/layout/AppLayout.vue'),
    redirect: '/admin/home',
    children: adminChildren,
  },
  { path: '/:pathMatch(.*)*', component: () => import('@/pages/NotFound.vue'), meta: { public: true } },
]

const router = createRouter({ history: createWebHashHistory(), routes })

router.beforeEach(async to => {
  ui.loadingBar?.start()
  if (to.meta.public) return true
  const auth = useAuthStore()
  // 未校验或超出时间窗口时重验 JWT；只有「明确未授权」才跳登录，网络错误放行
  if (auth.shouldRecheck()) {
    const ok = await auth.check()
    if (!ok && auth.expired) return { path: '/login', query: { redirect: to.fullPath } }
  }
  return true
})

router.afterEach(() => ui.loadingBar?.finish())
router.onError(() => ui.loadingBar?.error())

export default router
