// WebVPN 访问（远程会话 + HTTP 访问聚合）的类型、文案与图标
import type { Component } from 'vue'
import Globe from '~icons/fluent/globe-20-regular'
import Console from '~icons/fluent/window-console-20-regular'
import Folder from '~icons/fluent/folder-20-regular'
import FolderLink from '~icons/fluent/folder-link-20-regular'
import Desktop from '~icons/fluent/desktop-20-regular'
import { http } from '@/api/http'

export type RecReason =
  | 'recorded' | 'disabled' | 'file_protocol' | 'no_terminal' | 'file_missing' | 'http' | 'recording_failed' | 'unknown'

export interface Recording {
  available: boolean
  reason: RecReason
  format?: 'asciicast' | 'guac'
  parts: number
  error?: string
}

export interface Visit {
  id: string
  kind: 'session' | 'http'
  sid?: string
  username: string
  group: string
  app_name: string
  protocol: string
  target: string
  client_ip: string
  country_code: string
  started_at: string
  ended_at: string | null
  last_active?: string
  online: boolean
  duration: number
  bytes_in: number
  bytes_out: number
  file_ops: number
  requests: number
  errors: number
  risk: number
  terminals: number
  recording: Recording
  close_reason: string
  close_reason_text: string
}

export interface FileLog {
  id: number
  op: string
  path: string
  size: number
  result: string
  created_at: string
}

export interface AuditReq {
  id: number
  method: string
  path: string
  status_code: number
  duration_ms: number
  bytes_sent: number
  risk_level: number
  created_at: string
}

export const PROTO_LABEL: Record<string, string> = {
  http: 'HTTP', ssh: 'SSH', sftp: 'SFTP', smb: 'SMB', ftp: 'FTP', rdp: 'RDP', vnc: 'VNC',
}

const PROTO_ICON: Record<string, Component> = {
  http: Globe, ssh: Console, sftp: Folder, ftp: Folder, smb: FolderLink, rdp: Desktop, vnc: Desktop,
}
export const protoIcon = (p: string) => PROTO_ICON[p] || Globe

export const protocolOptions = [
  { label: '全部协议', value: '' },
  { label: 'HTTP 应用', value: 'http' },
  { label: '全部远程协议', value: 'remote' },
  ...['ssh', 'sftp', 'smb', 'ftp', 'rdp', 'vnc'].map(v => ({ label: PROTO_LABEL[v], value: v })),
]

export const isDesktop = (p: string) => p === 'rdp' || p === 'vnc'
export const isFileProto = (p: string) => p === 'sftp' || p === 'smb' || p === 'ftp'
// 有屏幕可录像的协议
export const hasScreen = (p: string) => p === 'ssh' || isDesktop(p)

// 录像不可用的原因（面向管理员的完整说明）
export function recReasonText(v: Visit): string {
  const r = v.recording
  const desk = isDesktop(v.protocol)
  switch (r.reason) {
    case 'recorded':
      return v.online ? '会话进行中，录像持续写入，回放内容截至加载时' : ''
    case 'disabled':
      return '该应用在会话开始时未开启录像，可在「WebVPN 应用」设置中开启'
    case 'file_protocol':
      return 'SFTP、SMB、FTP 等文件类协议只记录文件操作，没有屏幕录像'
    case 'no_terminal':
      if (v.online) return desk ? '尚未连接远程桌面，连接后开始录像' : '尚未打开终端，打开后开始录像'
      return desk ? '会话期间未成功连接远程桌面，没有可录制的画面' : '会话期间未打开终端（只使用了文件管理），没有可录制的画面'
    case 'file_missing':
      return r.format === 'guac'
        ? '录像文件不存在：可能已被清理，或 guacd 录像目录未与本服务共享'
        : '录像文件不存在，可能已被清理'
    case 'recording_failed':
      return `录像创建失败${r.error ? `：${r.error}` : ''}`
    case 'unknown':
      return '未开启录像或会话期间未打开终端（早期版本的记录无法区分）'
    default:
      return 'HTTP 应用不录制屏幕，可查看请求明细'
  }
}

// 列表徽标上的短原因
export const REC_SHORT: Record<RecReason, string> = {
  recorded: '录像',
  disabled: '未开启录像',
  file_protocol: '文件协议',
  no_terminal: '未打开终端',
  file_missing: '录像已清理',
  http: '',
  recording_failed: '录像失败',
  unknown: '无录像',
}

export const FILE_OPS: Record<string, string> = {
  list: '浏览', download: '下载', download_dir: '下载目录', upload: '上传', mkdir: '新建目录', rename: '重命名', delete: '删除',
}

export function fmtDuration(sec: number) {
  const s = Math.max(0, Math.floor(sec))
  if (s < 60) return `${s} 秒`
  if (s < 3600) return `${Math.floor(s / 60)} 分`
  if (s < 86400) return `${Math.floor(s / 3600)} 时 ${Math.floor((s % 3600) / 60)} 分`
  return `${Math.floor(s / 86400)} 天 ${Math.floor((s % 86400) / 3600)} 时`
}

export const recUrl = (sid: string, part = 1) =>
  `/api/v1/webvpn/remote/recording?sid=${encodeURIComponent(sid)}${part > 1 ? `&part=${part}` : ''}`
export const recName = (sid: string, fmt?: string, part = 1) =>
  fmt === 'guac' ? `${sid}${part > 1 ? `.${part}` : ''}.guac` : `${sid}.cast`

// 下载文件流（后端出错时返回 JSON {code,msg}）
export async function downloadBlob(url: string, params: Record<string, any>, fallback: string) {
  let resp
  try {
    resp = await http.get(url, { params, responseType: 'blob' })
  } catch (e: any) {
    const data = e?.response?.data
    if (data instanceof Blob) {
      let msg = ''
      try { msg = JSON.parse(await data.text())?.msg || '' } catch { /* 忽略 */ }
      if (msg) throw new Error(msg)
    }
    throw e
  }
  const blob = resp.data as Blob
  if (blob.type.includes('application/json')) {
    const body = JSON.parse(await blob.text())
    if (body?.code !== 0) throw new Error(body?.msg || '导出失败')
  }
  const cd = String(resp.headers['content-disposition'] || '')
  const m = cd.match(/filename\*?=(?:UTF-8'')?"?([^";]+)"?/i)
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = m ? decodeURIComponent(m[1]) : fallback
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(a.href)
}
