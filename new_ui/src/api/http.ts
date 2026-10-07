import axios, { type AxiosRequestConfig } from 'axios'
import { useAuthStore } from '@/stores/auth'
import router from '@/router'

// 后端统一响应：{ code: 0, msg, data }
export interface ApiResp<T = any> { code: number; msg: string; data: T }

export const http = axios.create()

// 裸对象自动 JSON；URLSearchParams / FormData / 字符串原样发送
http.defaults.transformRequest = [
  (data, headers) => {
    if (data && typeof data === 'object' && !(data instanceof FormData) && !(data instanceof URLSearchParams)) {
      headers['Content-Type'] = 'application/json'
      return JSON.stringify(data)
    }
    return data
  },
]

http.interceptors.response.use(undefined, err => {
  if (err.response?.status === 401) {
    const auth = useAuthStore()
    const path = router.currentRoute.value.path
    const url = String(err.config?.url || '')
    // 门户页面（/portal 及其子路由）或门户接口（/api/v1/portal/*）的 401 属于门户会话，不跳后台登录
    const isPortal = path === '/portal' || path.startsWith('/portal/') || url.startsWith('/api/v1/portal/')
    // 登录态校验与登录/登出接口的 401 由调用方（路由守卫、登录页）处理；已在公开页时也无需再跳登录
    const isAuthApi = /\/api\/v1\/base\/(auth_check|login|logout)/.test(url)
    const onPublic = !!router.currentRoute.value.meta.public
    // 门户/登出中/刚重新登录（迟到的旧请求 401）不跳转
    if (!isPortal && !isAuthApi && !onPublic && !auth.loggingOut && !(auth.checked && Date.now() - auth.lastLoginTime < 5000)) {
      auth.loggingOut = true
      auth.logout().finally(() => router.push({ path: '/login', query: { redirect: path } }))
    }
  }
  return Promise.reject(err)
})

export class ApiError extends Error {
  constructor(msg: string, public code: number) { super(msg) }
}

// 请求并解包 data；code !== 0 抛 ApiError
export async function request<T = any>(config: AxiosRequestConfig): Promise<T> {
  const resp = await http.request<ApiResp<T>>(config)
  const body = resp.data
  if (body && typeof body === 'object' && 'code' in body) {
    if (body.code !== 0) throw new ApiError(body.msg || '请求失败', body.code)
    return body.data
  }
  return body as unknown as T
}

export const get = <T = any>(url: string, params?: any, config?: AxiosRequestConfig) =>
  request<T>({ ...config, url, method: 'GET', params })

export const post = <T = any>(url: string, data?: any, config?: AxiosRequestConfig) =>
  request<T>({ ...config, url, method: 'POST', data })

// 表单编码提交（后端部分接口按 x-www-form-urlencoded 解析）
export const postForm = <T = any>(url: string, data: Record<string, any>) =>
  post<T>(url, new URLSearchParams(Object.entries(data).map(([k, v]) => [k, v == null ? '' : String(v)])))

// 统一错误文本
export function errMsg(e: unknown, fallback = '请求出错') {
  if (e instanceof ApiError) return e.message
  const r = (e as any)?.response
  if (r?.data?.msg) return r.data.msg
  // 只有 axios 请求失败才算网络错误，前端异常按原文提示便于排查
  if (!r) return axios.isAxiosError(e) ? '网络错误，请检查连接' : (e as Error)?.message || fallback
  return fallback
}
