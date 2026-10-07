// 短信页静态定义：服务商与各自必填项
export interface SmsForm {
  provider: string
  ali_access_key_id: string
  ali_access_key_secret: string
  ali_sign_name: string
  ali_template_code: string
  tencent_secret_id: string
  tencent_secret_key: string
  tencent_app_id: string
  tencent_sign_name: string
  tencent_template_id: string
  tencent_region: string
}

export const SMS_PROVIDERS = [
  { value: 'aliyun', label: '阿里云', desc: '阿里云短信服务（Dysms），使用 AccessKey 接入' },
  { value: 'tencent', label: '腾讯云', desc: '腾讯云短信 SMS，使用 SecretId / SecretKey 接入' },
  { value: '', label: '关闭', desc: '停用短信，依赖短信的验证方式将不可用' },
]

const REQUIRED: Record<string, [keyof SmsForm, string][]> = {
  aliyun: [['ali_access_key_id', 'AccessKey ID'], ['ali_access_key_secret', 'AccessKey Secret'], ['ali_sign_name', '短信签名'], ['ali_template_code', '模板 CODE']],
  tencent: [
    ['tencent_secret_id', 'SecretId'], ['tencent_secret_key', 'SecretKey'], ['tencent_region', '地域'],
    ['tencent_app_id', 'SDKAppID'], ['tencent_sign_name', '短信签名'], ['tencent_template_id', '模板 ID'],
  ],
}

// 返回指定服务商缺失的必填项名称
export function smsMissing(v: SmsForm, provider: string) {
  return (REQUIRED[provider] || []).filter(([k]) => !v[k]).map(([, l]) => l)
}
