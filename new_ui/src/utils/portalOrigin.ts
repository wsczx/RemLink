// 门户 / Web 认证页只能在 VPN 服务端口上工作（接口在用户服务上）。
// 若在管理端口（如 :8800）打开，管理服务的 login-config 会返回 admin_origin + vpn_port，
// 此时跳转到 VPN 服务端口上的同一页面。返回 true 表示已发起跳转
import axios from 'axios'

export async function redirectToVpnOriginIfAdmin(): Promise<boolean> {
  try {
    const r = await axios.get('/api/v1/portal/login-config', { withCredentials: false })
    const d = r.data?.data
    if (!d?.admin_origin) return false
    const port = String(d.vpn_port || '443')
    const host = location.hostname.includes(':') ? `[${location.hostname}]` : location.hostname
    const target = new URL(location.href)
    target.protocol = 'https:'
    target.host = port === '443' ? host : `${host}:${port}`
    if (target.href === location.href) return false
    location.replace(target.href)
    return true
  } catch {
    return false
  }
}
