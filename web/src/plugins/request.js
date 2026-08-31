// 全局的 axios 默认值
import axios from "axios";
import { logout as doLogout, isLoggingOut, beginLogout, isChecked, getLastLoginTime } from "./auth";

axios.defaults.headers.post['Content-Type'] = 'application/x-www-form-urlencoded';
// 兼容FormData和裸对象自动 JSON 序列化
axios.defaults.transformRequest = [
    function (data, headers) {
        if (data && typeof data === 'object' && !(data instanceof FormData) && typeof data !== 'string') {
            headers['Content-Type'] = 'application/json';
            return JSON.stringify(data);
        }
        return data;
    }
];

if (process.env.NODE_ENV !== 'production') {
    axios.defaults.baseURL = process.env.VUE_APP_API_BASE || '';
}

function request(vm) {
    // HTTP 请求拦截器
    axios.interceptors.request.use(config => {
        return config;
    });

    // HTTP 响应拦截器
    // 统一处理 401 状态，token 过期的处理，清除token跳转login
    axios.interceptors.response.use(null, err => {
        // 网络错误/超时/CORS：err.response 可能为 undefined
        if (!err.response) {
            return Promise.reject(err);
        }
        // 没有登录或令牌过期
        if (err.response.status === 401) {
            if (vm.$router.currentRoute.path === '/portal') {
                return Promise.reject(err);
            }
            // 防抖：已在登出流程中（并发 401）则忽略，避免反复 doLogout + push('/login')
            if (isLoggingOut()) {
                return Promise.reject(err);
            }
            // 重新登录后短时间内到达的 401，视为「重新登录前」残留在途请求延迟返回的旧响应，
            // 不应再把已登录的用户踢回登录页
            if (isChecked() && Date.now() - getLastLoginTime() < 5000) {
                return Promise.reject(err);
            }
            beginLogout();
            doLogout();
            vm.$router.push('/login');
        }
        return Promise.reject(err);
    });
}

export default request;
