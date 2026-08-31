import Vue from 'vue'
import axios from 'axios'

// 使用 Vue 响应式对象管理登录态（仅存内存，不存 localStorage）
// token 通过 HttpOnly Cookie 由浏览器自动管理，JS 不可访问
const state = Vue.observable({
    user: null,
    checked: false,
    showFakeDNS: false,
})

// 上次成功校验登录态的时间戳；用于路由守卫的「时间窗口」主动重验，
// 避免长期空闲后 JWT 过期、内存态却仍认为已登录，导致点菜单进页面才被 401 打断。
let lastCheckTime = 0
// 路由守卫重新校验的时间窗口：距上次成功校验超过该时长才重新请求后端，
// 避免每次菜单切换都打 /base/auth_check。
const CHECK_INTERVAL = 5 * 60 * 1000

// 登录成功（含重新登录）时间戳；用于屏蔽「重新登录前」残留在途请求延迟返回的 401，
// 这类迟到的 401 不应再把已登录的用户踢回登录页。
let lastLoginTime = 0

// 获取最近一次登录成功的时间戳（供请求拦截器判断迟到 401）。
export function getLastLoginTime() {
    return lastLoginTime
}

// 401 防抖：避免并发 401 反复 doLogout + push('/login') 造成抖动/竞态。
// 首次 401 进入后置 true，直到本次登录成功才复位。
let loggingOut = false

// 最近一次 checkAuth 是否为明确未授权（401/cookie 失效）。
let lastAuthExpired = false

export function getUser() {
    return state.user
}

export function setUser(username) {
    state.user = username
    state.checked = true
    lastCheckTime = Date.now()
    lastLoginTime = Date.now()
    loggingOut = false
}

export function isChecked() {
    return state.checked
}

export function getShowFakeDNS() {
    return state.showFakeDNS
}

// 路由守卫判断是否需要重新校验登录态：未校验过，或距上次成功校验已超过窗口。
export function shouldRecheck() {
    return !state.checked || (Date.now() - lastCheckTime > CHECK_INTERVAL)
}

// 最近一次 checkAuth 是否为「明确的未授权（401/cookie 失效）」。
// 仅此情况路由守卫才跳登录；网络错误不算未授权，避免网络抖动误踢。
export function isAuthExpired() {
    return lastAuthExpired
}

// 401 防抖查询：是否正在登出流程中（用于拦截并发 401）。
export function isLoggingOut() {
    return loggingOut
}

// 标记进入登出流程，抑制后续并发 401 的重复跳转。
export function beginLogout() {
    loggingOut = true
}

// 调用后端验证 Cookie 中的 JWT 是否有效，并获取当前用户名
// 同时获取 showFakeDNS 配置，确保页面渲染前就绪。
// 网络错误（err.response 不存在）不视为未授权，保留现有登录态、放行，
export async function checkAuth() {
    try {
        const resp = await axios.get('/base/auth_check')
        if (resp.data && resp.data.code === 0 && resp.data.data) {
            state.user = resp.data.data.admin_user
            state.checked = true
            lastCheckTime = Date.now()
            lastAuthExpired = false
            // 认证通过后获取 showFakeDNS
            try {
                const homeResp = await axios.get('/set/home')
                if (homeResp.data && homeResp.data.data) {
                    state.showFakeDNS = !!homeResp.data.data.show_fakedns
                }
            } catch {
                // /set/home 失败不影响登录
            }
            return true
        }
        // 后端返回非 0：明确未授权
        state.user = null
        state.checked = false
        lastAuthExpired = true
        return false
    } catch (err) {
        // 明确 401：JWT 过期或 cookie 失效
        if (err.response && err.response.status === 401) {
            state.user = null
            state.checked = false
            lastAuthExpired = true
            return false
        }
        // 网络错误/超时：保留登录态并放行，避免误踢
        lastAuthExpired = false
        return false
    }
}

// logout 调用后端清除 HttpOnly Cookie，并清空内存中的用户状态
export async function logout() {
    try {
        await axios.post('/base/logout')
    } catch {
        // 忽略错误，本地状态仍然清除
    }
    state.user = null
    state.checked = false
    lastCheckTime = 0
    lastAuthExpired = false
    // 注意：不复位 loggingOut，避免登出过程中并发 401 再触发跳转
}
