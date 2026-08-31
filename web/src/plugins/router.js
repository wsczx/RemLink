import Vue from "vue";
import VueRouter from "vue-router";
import { shouldRecheck, isAuthExpired, checkAuth } from "./auth";

Vue.use(VueRouter)


const routes = [
    { path: '/', redirect: '/admin/home' },
    { path: '/login', component: () => import('@/pages/Login') },
    { path: '/portal', component: () => import('@/pages/Portal') },
    { path: '/web-auth', component: () => import('@/pages/WebAuth') },
    { path: '/web-auth/continue', component: () => import('@/pages/WebAuth') },
    {
        path: '/admin',
        component: () => import('@/layout/Layout'),
        redirect: '/admin/home',
        children: [
            { path: 'home', component: () => import('@/pages/Home') },

            { path: 'set/system', component: () => import('@/pages/set/System') },
            { path: 'set/soft', component: () => import('@/pages/set/Soft') },
            { path: 'set/security', component: () => import('@/pages/set/Security') },
            { path: 'set/cert', component: () => import('@/pages/set/Cert') },
            { path: 'set/other', component: () => import('@/pages/set/Other') },
            { path: 'set/audit', component: () => import('@/pages/set/Audit') },
            { path: 'set/syslog', component: () => import('@/pages/set/Syslog') },

            { path: 'user/list', component: () => import('@/pages/user/List') },
            { path: 'user/online', component: () => import('@/pages/user/Online') },
            { path: 'user/ip_map', component: () => import('@/pages/user/IpMap') },
            { path: 'user/lockmanager', component: () => import('@/pages/user/LockManager') },

            { path: 'group/list', component: () => import('@/pages/group/List') },
            { path: 'policy/list', component: () => import('@/pages/policy/List') },
            { path: 'provider/list', component: () => import('@/pages/provider/List') },

            { path: 'webvpn', component: () => import('@/pages/webvpn/AppList') },
            { path: 'webvpn/audit', component: () => import('@/pages/webvpn/AuditList') },

        ],
    },

    { path: '*', component: () => import('@/pages/NotFound') },
]

// 3. 创建 router 实例，然后传 `routes` 配置
// 你还可以传别的配置参数, 不过先这么简单着吧。
const router = new VueRouter({
    routes
})

// 路由守卫
router.beforeEach(async (to, from, next) => {
    // 未知 pathname 展示 404 页面（门户 /portal 走 routes 内已注册路由，正常放行）
    const pn = window.location.pathname
    const validPn = pn === '/' || pn === '/ui' || pn === '/ui/' || pn === '/login' || pn.startsWith('/web-auth')
    if (!validPn && to.path !== '/404') {
        next('/404')
        return
    }

    // 公开页面直接放行
    if (to.path === "/login" || to.path === "/portal" || to.path.startsWith("/portal/") || to.path.startsWith("/web-auth")) {
        next();
        return;
    }

    // 404 页面直接放行展示
    if (to.matched.length && to.matched[0].path === '*') {
        next();
        return;
    }

    // 受保护页面：未校验过，或距上次成功校验超过时间窗口时，主动重新校验 JWT 是否有效。
    // 仅「明确未授权（401/cookie 过期）」才跳登录；网络错误放行，避免网络抖动误踢用户。
    if (shouldRecheck()) {
        const ok = await checkAuth();
        if (!ok && isAuthExpired()) {
            next({
                path: '/login',
                query: { redirect: to.path }
            });
            return;
        }
    }

    next();
});

export default router;
