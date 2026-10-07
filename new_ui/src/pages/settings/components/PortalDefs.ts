// 门户首页设置的类型与静态定义
export interface QuickLink { label: string; url: string; icon: string }
export interface GuideGroup { name: string; steps: string[] }

// 默认 5 平台指引（与门户未配置时的回退内容一致）；steps 为 HTML 片段，支持 {{server_addr}} 占位
export const defaultGuide = (): GuideGroup[] => [
  {
    name: 'Windows',
    steps: [
      '下载并安装 Cisco AnyConnect 客户端',
      '打开客户端，在地址栏输入：<code class="addr-code">{{server_addr}}</code>',
      '点击"连接"，输入用户名和密码即可',
    ],
  },
  {
    name: 'macOS',
    steps: [
      '从 App Store 下载 Cisco AnyConnect',
      '打开后输入服务器地址：<code class="addr-code">{{server_addr}}</code>',
      '输入用户名和密码，点击 Connect',
    ],
  },
  {
    name: 'Linux',
    steps: [
      '安装 OpenConnect：<code class="addr-code">apt install openconnect</code> 或 <code class="addr-code">yum install openconnect</code>',
      '命令行连接：<code class="addr-code">sudo openconnect {{server_addr}}</code>',
      '按提示输入用户名和密码',
    ],
  },
  {
    name: 'Android',
    steps: [
      '从 Google Play 或应用商店下载 Cisco AnyConnect',
      '添加新连接，输入服务器地址：<code class="addr-code">{{server_addr}}</code>',
      '输入用户名和密码连接',
    ],
  },
  {
    name: 'iOS',
    steps: [
      '从 App Store 下载 Cisco AnyConnect',
      '打开后点击"添加 VPN 连接"，输入服务器：<code class="addr-code">{{server_addr}}</code>',
      '保存后点击连接，输入用户名和密码',
    ],
  },
]

export const defaultCards = (): Record<string, boolean> => ({
  devices: true, groups: true, personal_policy: true, password: true, otp: true, certs: true, webvpn: true,
})

export const CARD_OPTIONS = [
  { key: 'devices', label: '在线设备', desc: '当前已连接的终端' },
  { key: 'webvpn', label: 'WebVPN 应用', desc: '可直接访问的内网应用' },
  { key: 'groups', label: '分组权限', desc: '所属用户组及可访问资源' },
  { key: 'personal_policy', label: '个人策略', desc: '针对个人下发的路由与 ACL' },
  { key: 'password', label: '修改密码', desc: '自助修改登录密码' },
  { key: 'otp', label: '二次验证', desc: '绑定 / 查看 OTP 动态口令' },
  { key: 'certs', label: '我的证书', desc: '下载个人客户端证书' },
]

export const LEVELS = [
  { value: 'info', label: '通知' },
  { value: 'success', label: '成功' },
  { value: 'warning', label: '警告' },
  { value: 'error', label: '紧急' },
] as const

// 主题色候选（用户可选的数据值，不用于本控制台样式）
export const THEME_SWATCHES = ['#18181b', '#2f7cff', '#0ea5e9', '#14b8a6', '#16a34a', '#f59e0b', '#ef4444', '#8b5cf6']

export const isHexColor = (c?: string) => !!c && /^#?[0-9a-f]{6}$/i.test(c)
export const normHex = (c: string) => (c.startsWith('#') ? c : '#' + c)

// 快捷链接图标类型：图片地址 / emoji 或文字 / 默认图标（空或旧版 el-icon 类名）
export function linkIconKind(icon?: string): 'image' | 'emoji' | 'default' {
  if (!icon) return 'default'
  if (/^(https?:\/\/|\/\/|\.\.?\/|data:image\/)/i.test(icon)) return 'image'
  if (/^el-icon/.test(icon)) return 'default'
  return 'emoji'
}
