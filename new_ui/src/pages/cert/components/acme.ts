// Let's Encrypt（ACME DNS-01）配置：与后端 dbdata.SettingLetsEncrypt 字段一致，certType 仅随签发请求提交
export type Provider = 'aliyun' | 'txcloud' | 'cfcloud'

export interface LetsForm {
  domain: string
  legomail: string
  name: Provider | ''
  renew: boolean
  dnsResolver: string
  certType: 'main' | 'wild'
  aliyun: { apiKey: string; secretKey: string }
  txcloud: { secretId: string; secretKey: string }
  cfcloud: { authToken: string }
}

export const blankLets = (): LetsForm => ({
  domain: '', legomail: '', name: '', renew: false, dnsResolver: '', certType: 'main',
  aliyun: { apiKey: '', secretKey: '' },
  txcloud: { secretId: '', secretKey: '' },
  cfcloud: { authToken: '' },
})

export const fromServer = (d: any): LetsForm => {
  const b = blankLets()
  return {
    ...b, ...d,
    certType: 'main',
    renew: !!d?.renew,
    aliyun: { ...b.aliyun, ...d?.aliyun },
    txcloud: { ...b.txcloud, ...d?.txcloud },
    cfcloud: { ...b.cfcloud, ...d?.cfcloud },
  }
}

export interface ProviderDef {
  value: Provider
  label: string
  mark: string
  desc: string
  fields: { label: string; key: string; secret?: boolean; hint: string }[]
}

export const PROVIDERS: ProviderDef[] = [
  {
    value: 'aliyun', label: '阿里云 DNS', mark: 'Ali', desc: '域名解析托管在阿里云（万网）',
    fields: [
      { label: 'AccessKey ID', key: 'apiKey', hint: 'RAM 用户的 AccessKey ID，需有 AliyunDNSFullAccess 权限' },
      { label: 'AccessKey Secret', key: 'secretKey', secret: true, hint: '与 AccessKey ID 配对的密钥' },
    ],
  },
  {
    value: 'txcloud', label: '腾讯云 DNSPod', mark: 'TC', desc: '域名解析托管在腾讯云 / DNSPod',
    fields: [
      { label: 'SecretId', key: 'secretId', hint: 'API 密钥的 SecretId，需有 DNSPod 读写权限' },
      { label: 'SecretKey', key: 'secretKey', secret: true, hint: '与 SecretId 配对的密钥' },
    ],
  },
  {
    value: 'cfcloud', label: 'Cloudflare', mark: 'CF', desc: '域名 NS 指向 Cloudflare',
    fields: [{ label: 'API Token', key: 'authToken', secret: true, hint: '需包含 Zone.DNS 编辑权限的 API Token' }],
  },
]

export const providerOf = (v: string) => PROVIDERS.find(p => p.value === v)
