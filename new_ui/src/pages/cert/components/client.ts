// 客户端证书相关的共享类型与工具
import { get } from '@/api/http'
import { ui } from '@/utils/ui'

export interface CertUser { username: string; nickname?: string; groups?: string[]; Groups?: string[] }

export interface ClientCert {
  username: string
  groupname: string
  device_binding_enabled: boolean
  device_id?: string[] | null
  max_devices: number
  created_at: string
  not_after: string
  status: number // 0 启用 / 1 禁用 / 2 过期
  is_csr_based?: boolean
}

// 各状态证书数量
export interface CertStats { all: number; ok: number; disabled: number; expired: number }

export const userLabel = (username: string, nickname?: string) => (nickname ? `${username}(${nickname})` : username)

// 提示：所选组未配置 TLS 证书认证
export async function checkCertAuth(group: string, hint: string) {
  if (!group) return
  try {
    const d = await get('/api/v1/group/cert_auth_check', { groupname: group })
    if (d && !d.has_cert_auth) ui.message.warning(`当前组「${group}」尚未配置证书认证，${hint}`)
  } catch { /* 忽略 */ }
}

// 下载 blob；后端出错时以 JSON 返回 {code,msg}
export async function saveBlob(data: Blob, contentType: string | undefined, filename: string) {
  if (contentType?.includes('application/json')) {
    let msg = '下载失败'
    try { msg = JSON.parse(await data.text()).msg || msg } catch { /* 忽略 */ }
    throw new Error(msg)
  }
  const url = URL.createObjectURL(new Blob([data], { type: contentType }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
