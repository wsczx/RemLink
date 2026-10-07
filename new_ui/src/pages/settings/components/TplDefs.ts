// 模板页静态定义：邮件模板变量与示例数据、首页状态码模式
export interface TplVar { key: string; label: string; sample: string }

// 账号开通邮件（后端 userAccountMailData）
export const ACCOUNT_VARS: TplVar[] = [
  { key: 'Username', label: '用户名', sample: 'zhangsan' },
  { key: 'Nickname', label: '姓名', sample: '张三' },
  { key: 'PinCode', label: '初始密码', sample: 'Rl#8x2Kq' },
  { key: 'Group', label: '用户组', sample: 'dev,ops' },
  { key: 'LinkAddr', label: '访问地址', sample: 'vpn.example.com' },
  { key: 'Issuer', label: '签发方', sample: 'RemLink' },
  { key: 'LimitTime', label: '账号有效期', sample: '2027-12-31' },
  { key: 'OtpImg', label: 'OTP 二维码地址', sample: '' },
  { key: 'OtpImgBase64', label: 'OTP 二维码（内嵌）', sample: '' },
  { key: 'DisableOtp', label: '是否禁用 OTP', sample: 'false' },
]

// 证书下发邮件
export const CERT_VARS: TplVar[] = [
  { key: 'Username', label: '用户名', sample: 'zhangsan' },
  { key: 'Groupname', label: '用户组', sample: 'dev' },
  { key: 'SerialNumber', label: '证书序列号', sample: '1A2B3C4D' },
  { key: 'NotAfter', label: '到期时间', sample: '2027-10-05' },
  { key: 'Password', label: 'P12 密码', sample: 'p12-Pass' },
  { key: 'LinkAddr', label: '访问地址', sample: 'vpn.example.com' },
]

// 用示例数据渲染 Go 模板：替换已知变量，去掉 if / else / end 等控制标签
export function renderSample(html: string, vars: TplVar[]) {
  if (!html) return ''
  let out = html
  for (const v of vars) out = out.replace(new RegExp(`\{\{-?\s*\.${v.key}\s*-?\}\}`, 'g'), v.sample)
  return out.replace(/\{\{[^}]*\}\}/g, '')
}

export type HomeMode = 'portal' | 'custom' | 'status'
export const homeModeOf = (code?: number | null): HomeMode => (code === 0 ? 'custom' : code === 200 ? 'portal' : 'status')
