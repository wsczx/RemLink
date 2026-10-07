// WebVPN 远程访问（SSH/SFTP/SMB/FTP/RDP/VNC）用户侧接口，两种访问方式：
//   - 子域名模式：在应用子域同源调用 /webvpn/*，依赖 webvpn_session cookie
//   - 路径模式：在 VPN 主域名同源调用 /web/<协议>/<应用>/webvpn/*，依赖门户会话 cookie
// 注意：不走 @/api/http（其 401 会跳后台登录），这里用独立 fetch 封装

// 接口基础路径：子域名模式为空，路径模式为 /web/<协议>/<应用>
let apiBase = ''
const PROTOS = ['ssh', 'sftp', 'smb', 'ftp', 'rdp', 'vnc']

// 按页面参数（?app=&protocol=&mode=path）设置访问方式；参数不合法时退回子域名模式
export function configureRemote(q: { app?: unknown; protocol?: unknown; mode?: unknown }) {
  const app = typeof q.app === 'string' ? q.app : ''
  const proto = typeof q.protocol === 'string' ? q.protocol : ''
  apiBase = q.mode === 'path' && /^[a-z0-9-]{1,60}$/.test(app) && PROTOS.includes(proto) ? `/web/${proto}/${app}` : ''
  return apiBase
}

// 是否路径模式
export const isPathMode = () => apiBase !== ''

// 当前访问方式下的入口地址（未登录时登录完成后回到这里）
export const remoteEntry = () => (apiBase ? apiBase + '/' : '/')

export interface Permissions {
  upload: boolean
  download: boolean
  delete: boolean
  clipboard: boolean
  max_file_size: number
}

export interface AppInfo {
  name: string
  note: string
  protocol: string
  cred_mode: 'prompt' | 'stored' | 'passthrough'
  fixed_username: string
  need_username: boolean
  need_password: boolean
  username_optional?: boolean // VNC：账号可留空
  password_optional?: boolean // VNC：密码可留空
  private_key?: boolean // 仅 ssh/sftp 支持私钥
  terminal: boolean
  file: boolean
  desktop?: boolean // rdp/vnc：远程桌面（经 guacd）
  ws_path?: string // [/web/<协议>/<应用>]/webvpn/ws/term | …/ws/desktop | 空（纯文件协议）
  guacd_available?: boolean // 仅桌面协议返回
  record: boolean
  record_format?: 'asciicast' | 'guac' | ''
  idle_timeout: number
  permissions: Permissions
  max_sessions: number
  username: string
  ticket_ttl_seconds: number
}

export interface SessionInfo {
  sid: string
  protocol: string
  target_user: string
  host_key_fp: string
  idle_timeout: number
  record: boolean
  permissions: Permissions
  ticket: string
  ticket_expires: number
  ws_path?: string
}

export interface LiveSession {
  sid: string
  username: string
  app_name: string
  protocol: string
  target: string
  client_ip: string
  started_at: string
  last_active: string
  bytes_in: number
  bytes_out: number
  terminals: number
}

export interface FileEntry {
  name: string
  path: string
  size: number
  mode: string
  mod_time: number
  is_dir: boolean
  is_link: boolean
}

export interface CredInput {
  app?: string
  username?: string
  password?: string
  private_key?: string
  passphrase?: string
}

// 网关错误：status=HTTP 状态码，code=机器可读错误码（auth_failed 等），data 为附加数据
export class RemoteError extends Error {
  constructor(msg: string, public status: number, public code: string, public data?: any) { super(msg) }
}

async function call<T = any>(method: string, url: string, body?: any): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin', headers: {} }
  if (body instanceof FormData) init.body = body
  else if (body !== undefined) {
    init.body = JSON.stringify(body)
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
  }
  let resp: Response
  try {
    resp = await fetch(url, init)
  } catch {
    throw new RemoteError('网络错误，请检查连接', 0, 'network')
  }
  let j: any = null
  try { j = await resp.json() } catch { /* 非 JSON */ }
  if (!resp.ok || !j || j.code !== 0) {
    throw new RemoteError(j?.msg || `请求失败（${resp.status}）`, resp.status, j?.error || 'http_' + resp.status, j?.data)
  }
  return j.data as T
}

const q = (o: Record<string, string>) => new URLSearchParams(o).toString()

// 接口路径（自动带上当前访问方式的基础路径）
const u = (p: string) => apiBase + '/webvpn' + p
const sidSeg = (sid: string) => encodeURIComponent(sid)

export const remoteApi = {
  appInfo: () => call<AppInfo>('GET', u('/app-info')),
  sessions: () => call<LiveSession[]>('GET', u('/sessions')),
  createSession: (c: CredInput) => call<SessionInfo>('POST', u('/sessions'), c),
  closeSession: (sid: string) => call('DELETE', u(`/sessions/${sidSeg(sid)}`)),
  ticket: (sid: string) => call<{ sid: string; ticket: string; ticket_expires: number; ws_path?: string }>('POST', u(`/sessions/${sidSeg(sid)}/ticket`)),
  list: (sid: string, path: string) => call<{ path: string; entries: FileEntry[] | null }>('GET', u(`/fs/${sidSeg(sid)}/list?${q({ path })}`)),
  stat: (sid: string, path: string) => call<{ exists: boolean; entry?: FileEntry }>('GET', u(`/fs/${sidSeg(sid)}/stat?${q({ path })}`)),
  mkdir: (sid: string, path: string) => call('POST', u(`/fs/${sidSeg(sid)}/mkdir`), { path }),
  rename: (sid: string, from: string, to: string) => call('POST', u(`/fs/${sidSeg(sid)}/rename`), { from, to }),
  remove: (sid: string, paths: string[]) => call('POST', u(`/fs/${sidSeg(sid)}/delete`), { paths }),
  downloadUrl: (sid: string, path: string) => u(`/fs/${sidSeg(sid)}/download?${q({ path })}`),
  uploadUrl: (sid: string, dir: string, overwrite: boolean) => u(`/fs/${sidSeg(sid)}/upload?${q({ dir, overwrite: overwrite ? '1' : '0' })}`),
  termUrl: (ticket: string, cols: number, rows: number) => {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${location.host}${u('/ws/term')}?${q({ ticket, cols: String(cols), rows: String(rows) })}`
  },
  // 远程桌面 WebSocket 路径（后端 ws_path 缺省时的回退）
  desktopPath: () => u('/ws/desktop'),
}

// 错误码 → 友好提示
export function friendlyError(e: unknown): string {
  if (!(e instanceof RemoteError)) return (e as Error)?.message || '操作失败'
  switch (e.code) {
    case 'auth_failed': return '用户名或密码错误，目标主机拒绝了登录'
    case 'too_many_failures': return '认证失败次数过多，已暂时锁定，请 5 分钟后再试'
    case 'host_key_mismatch': return '目标主机指纹与管理员固定的值不一致，已拒绝连接（可能存在中间人攻击）。请联系管理员核实。'
    case 'target_unreachable': return '无法连接到目标主机，请稍后再试或联系管理员'
    case 'too_many_sessions': return '已达到该应用的同时在线会话上限，请先关闭其它会话'
    case 'need_username': return '请输入目标主机账号'
    case 'need_credential': return '请输入密码或私钥'
    case 'need_password': return '请输入密码'
    case 'bad_private_key': return '私钥格式错误或口令不正确'
    case 'bad_username': return '账号格式不合法'
    case 'stored_credential_error': return '托管凭据不可用，请联系管理员重新录入'
    case 'sftp_failed': case 'fs_failed': return '文件服务初始化失败，请联系管理员检查根目录配置'
    case 'share_not_found': return '共享不存在或当前账号无权访问该共享，请联系管理员核实共享名'
    case 'guacd_unavailable': return '远程桌面服务（guacd）未部署或不可达，请联系管理员'
    case 'session_not_found': return '会话不存在或已结束'
    case 'unsupported_protocol': return '该应用协议暂不支持网页访问'
    case 'forbidden': return '你无权访问该应用'
    case 'app_not_found': return '应用不存在'
    case 'path_denied': return '路径不允许访问'
    case 'permission_denied': return '目标主机拒绝访问（权限不足）'
    case 'upload_denied': case 'download_denied': case 'delete_denied': return e.message
    case 'exists': return '目标已存在'
    case 'too_large': return '超出管理员设置的文件大小上限'
  }
  return e.message
}

// 路径工具（相对根，"/" 开头）
export function joinPath(dir: string, name: string) {
  return (dir.endsWith('/') ? dir : dir + '/') + name
}
export function parentPath(p: string) {
  const i = p.replace(/\/+$/, '').lastIndexOf('/')
  return i <= 0 ? '/' : p.slice(0, i)
}
