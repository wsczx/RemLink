// 邮件页静态定义：加密方式、常用服务商预设、就绪检查
export interface SmtpForm {
  host: string
  port: number | null
  username: string
  password: string
  from: string
  encryption: string
}

export const MAIL_ENCRYPTIONS = [
  { value: 'SSLTLS', label: 'SSL / TLS', desc: '连接即加密，推荐', port: 465 },
  { value: 'STARTTLS', label: 'STARTTLS', desc: '先明文握手再升级加密', port: 587 },
  { value: 'None', label: '不加密', desc: '仅限内网中继使用', port: 25 },
]

export const MAIL_PRESETS = [
  { name: '腾讯企业邮', host: 'smtp.exmail.qq.com', port: 465, encryption: 'SSLTLS' },
  { name: '阿里企业邮', host: 'smtp.qiye.aliyun.com', port: 465, encryption: 'SSLTLS' },
  { name: '网易企业邮', host: 'smtp.qiye.163.com', port: 465, encryption: 'SSLTLS' },
  { name: 'Microsoft 365', host: 'smtp.office365.com', port: 587, encryption: 'STARTTLS' },
  { name: 'Gmail', host: 'smtp.gmail.com', port: 587, encryption: 'STARTTLS' },
]

export function mailChecks(s: SmtpForm) {
  const plain = !s.encryption || s.encryption === 'None'
  return [
    { label: '服务器地址', ok: !!s.host, required: true, detail: s.host || '未填写' },
    { label: '端口', ok: s.port != null && s.port > 0, required: true, detail: s.port ? String(s.port) : '未填写' },
    { label: '登录账号', ok: !!s.username, required: true, detail: s.username ? '已设置' : '未填写' },
    { label: '密码 / 授权码', ok: !!s.password, required: false, detail: s.password ? '已保存' : '未设置' },
    { label: '发件人地址', ok: !!s.from, required: true, detail: s.from ? '已设置' : '未填写' },
    { label: '传输加密', ok: !plain, required: false, detail: plain ? '明文' : s.encryption },
  ]
}
