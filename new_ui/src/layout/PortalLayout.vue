<script setup lang="ts">
// 用户门户外壳：已登录为「侧边栏 + 顶栏 + 内容」的自助中心；未登录时整屏居中显示登录
import { computed, h, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NConfigProvider, NDropdown, NIcon, NSpin, type DropdownOption, type GlobalThemeOverrides } from 'naive-ui'
import { useLocalStorage, useMediaQuery } from '@vueuse/core'
import { redirectToVpnOriginIfAdmin } from '@/utils/portalOrigin'
import Moon from '~icons/fluent/weather-moon-20-regular'
import Sun from '~icons/fluent/weather-sunny-20-regular'
import PanelLeft from '~icons/fluent/panel-left-20-regular'
import Navigation from '~icons/fluent/navigation-20-regular'
import ChevronUpDown from '~icons/fluent/chevron-up-down-20-regular'
import SignOut from '~icons/fluent/sign-out-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import Home from '~icons/fluent/home-20-regular'
import Laptop from '~icons/fluent/laptop-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import AppsIcon from '~icons/fluent/apps-20-regular'
import ShieldKeyhole from '~icons/fluent/shield-keyhole-20-regular'
import ShieldCheckmark from '~icons/fluent/shield-checkmark-20-regular'
import Certificate from '~icons/fluent/certificate-20-regular'
import SideNav from '@/components/SideNav.vue'
import type { NavNode } from './nav'
import { useThemeStore } from '@/stores/theme'
import { normalizeImageSrc } from '@/utils/brand'
import PortalAuth from '@/pages/portal/PortalAuth.vue'
import { usePortal } from '@/pages/portal/store'

const route = useRoute()
const router = useRouter()
const theme = useThemeStore()
const p = usePortal()
const { checking, loggedIn, brand, user, dashboard, webvpnMode, preset, pageVisible } = p

const brandTitle = computed(() => brand.value.title || 'RemLink')
// 只显示管理员自定义的 logo，默认不放图标
const logo = computed(() => normalizeImageSrc(brand.value.logo))
const initial = computed(() => (user.value.username || 'U').slice(0, 1).toUpperCase())

// ---------- 导航 ----------
// 概览用 /portal/（带尾斜杠），避免其前缀匹配把所有子页都算成激活
const allNav: (NavNode & { page?: string })[] = [
  { key: 'overview', label: '概览', icon: Home, to: '/portal/' },
  { key: 'devices', label: '我的设备', icon: Laptop, to: '/portal/devices', page: 'devices' },
  { key: 'clients', label: '客户端下载', icon: ArrowDownload, to: '/portal/clients', page: 'clients' },
  { key: 'apps', label: '应用', icon: AppsIcon, to: '/portal/apps', page: 'apps' },
  { key: 'access', label: '访问权限', icon: ShieldKeyhole, to: '/portal/access', page: 'access' },
  { key: 'security', label: '账号安全', icon: ShieldCheckmark, to: '/portal/security', page: 'security' },
  { key: 'certs', label: '我的证书', icon: Certificate, to: '/portal/certs', page: 'certs' },
]
const nav = computed<NavNode[]>(() => allNav.filter(n => !n.page || pageVisible.value[n.page]))

// 统一为 /portal/，让侧栏高亮生效（保留 query，供 redirect / reset_token 等参数使用）
watch(() => route.path, path => {
  if (path === '/portal') router.replace({ path: '/portal/', query: route.query, hash: route.hash })
}, { immediate: true })

// 被后台隐藏的页面：直接访问时回到概览
watch([() => route.path, pageVisible, loggedIn, checking], () => {
  if (!loggedIn.value || checking.value) return
  const hit = allNav.find(n => n.page && route.path === n.to)
  if (hit?.page && !pageVisible.value[hit.page]) router.replace('/portal/')
})

// ---------- 侧栏折叠 / 移动端抽屉 ----------
const collapsed = useLocalStorage('rl-portal-sider-collapsed', false)
const isMobile = useMediaQuery('(max-width: 768px)')
const mobileOpen = ref(false)
const miniNav = computed(() => collapsed.value && !isMobile.value)
watch(() => route.fullPath, () => { mobileOpen.value = false })
function toggleSider() {
  if (isMobile.value) mobileOpen.value = !mobileOpen.value
  else collapsed.value = !collapsed.value
}

// ---------- 用户菜单 ----------
const menuIcon = (c: Component) => () => h(NIcon, null, { default: () => h(c) })
const userMenu = computed<DropdownOption[]>(() => [
  { key: 'who', type: 'render', render: () => h('div', { class: 'px-12px py-8px' }, [
    h('div', { class: 'text-md font-semibold' }, p.displayName.value),
    h('div', { class: 'text-sm text-secondary' }, p.typeLabel.value),
  ]) },
  { type: 'divider', key: 'd0' },
  ...(pageVisible.value.security ? [{ label: '修改密码', key: 'password', icon: menuIcon(LockClosed) }] : []),
  { label: '退出登录', key: 'logout', icon: menuIcon(SignOut) },
])
function onUserMenu(key: string) {
  if (key === 'password') router.push('/portal/security')
  else if (key === 'logout') p.logout()
}

// 后台配置的门户主题色：局部覆盖主色
const themeOverrides = computed<GlobalThemeOverrides | undefined>(() => {
  const c = dashboard.value.theme_color
  if (!loggedIn.value || !c || !/^#?[0-9a-f]{6}$/i.test(c)) return undefined
  const color = c.startsWith('#') ? c : '#' + c
  return { common: { primaryColor: color, primaryColorHover: color, primaryColorPressed: color, primaryColorSuppl: color } }
})

const onBrandUpdated = () => p.loadBrand()
onMounted(async () => {
  // 在管理端口打开时跳到 VPN 服务端口（门户接口只在那里）
  if (await redirectToVpnOriginIfAdmin()) return
  p.init()
  // 后台保存品牌配置后立即刷新
  window.addEventListener('remlink:brand-updated', onBrandUpdated)
})
onBeforeUnmount(() => {
  window.removeEventListener('remlink:brand-updated', onBrandUpdated)
  p.dispose()
})
</script>

<template>
  <NConfigProvider :theme-overrides="themeOverrides" abstract>
    <!-- 校验登录态 -->
    <div v-if="checking" class="flex min-h-screen items-center justify-center bg-body">
      <NSpin />
    </div>

    <!-- 未登录：居中登录，无卡片外框 -->
    <div v-else-if="!loggedIn" class="flex min-h-screen flex-col bg-body text-primary">
      <div class="flex h-64px items-center justify-between px-24px">
        <div class="flex min-w-0 items-center gap-8px">
          <img v-if="logo" :src="logo" class="h-24px w-24px object-contain" alt="">
          <span class="truncate text-lg font-bold tracking-tight">{{ brandTitle }}</span>
        </div>
        <button class="rl-icon-btn" :title="theme.isDark ? '切换为亮色' : '切换为暗色'" @click="theme.toggle()">
          <Sun v-if="theme.isDark" /><Moon v-else />
        </button>
      </div>
      <div class="flex flex-1 items-center justify-center px-16px pb-64px pt-16px">
        <div class="w-full max-w-360px">
          <PortalAuth :brand="brand" :webvpn-mode="webvpnMode" :preset="preset" @pass="p.onPass" @sms-pass="p.onSmsPass" />
        </div>
      </div>
      <div v-if="brand.footer" class="portal-html px-16px pb-24px text-center text-sm text-tertiary" v-html="brand.footer" />
      <div v-else class="pb-24px text-center text-sm text-tertiary">© {{ brandTitle }}</div>
    </div>

    <!-- 已登录：自助中心 -->
    <div v-else class="flex min-h-screen bg-body text-primary">
      <div v-if="isMobile && mobileOpen" class="fixed inset-0 z-190 bg-black/30" @click="mobileOpen = false" />
      <aside
        class="portal-sider top-0 z-200 flex h-screen flex-shrink-0 flex-col bg-fill transition-[width,transform] duration-200"
        :class="[
          isMobile ? 'fixed left-0 w-240px' : 'sticky',
          isMobile && !mobileOpen && '-translate-x-full',
          !isMobile && (collapsed ? 'w-64px' : 'w-240px'),
        ]"
      >
        <div class="flex h-56px flex-shrink-0 items-center gap-sm px-16px" :class="miniNav && 'justify-center px-0!'">
          <RouterLink to="/portal/" class="flex min-w-0 items-center gap-sm">
            <img v-if="logo" :src="logo" class="block h-24px w-24px flex-shrink-0 object-contain" alt="">
            <span v-if="!miniNav" class="truncate text-lg font-bold tracking-tight">{{ brandTitle }}</span>
            <span v-else-if="!logo" class="text-lg font-bold">{{ brandTitle.slice(0, 1) }}</span>
          </RouterLink>
        </div>

        <div class="ax-scrollbar-none flex-1 overflow-y-auto px-10px pb-sm pt-4px">
          <SideNav :nav="nav" :collapsed="miniNav" />
        </div>

        <div class="flex-shrink-0 p-10px b-t-1px b-t-solid b-[var(--color-border-default)]">
          <NDropdown trigger="click" placement="top-start" :options="userMenu" @select="onUserMenu">
            <div class="flex cursor-pointer items-center gap-sm rounded-8px p-6px hover:bg-action-hover" :class="miniNav && 'justify-center'">
              <div class="relative flex h-32px w-32px flex-shrink-0 items-center justify-center rounded-full bg-action text-md font-bold" style="color: var(--color-body)">
                {{ initial }}
                <span
                  class="absolute bottom-0 right-0 h-8px w-8px rounded-full ring-2 ring-[var(--color-fill)]"
                  :style="{ background: p.devices.value.length ? 'var(--color-text-success)' : 'var(--color-text-quaternary)' }"
                />
              </div>
              <template v-if="!miniNav">
                <div class="min-w-0 flex-1 leading-tight">
                  <div class="truncate text-md font-semibold">{{ user.username || '-' }}</div>
                  <div class="truncate text-sm text-secondary">{{ p.devices.value.length ? `${p.devices.value.length} 台设备在线` : p.typeLabel.value }}</div>
                </div>
                <NIcon :size="16" class="text-tertiary"><ChevronUpDown /></NIcon>
              </template>
            </div>
          </NDropdown>
        </div>
      </aside>

      <div class="flex min-w-0 flex-1 flex-col">
        <header class="sticky top-0 z-90 flex h-56px flex-shrink-0 items-center gap-sm bg-body px-16px md:px-lg b-b-1px b-b-solid b-[var(--color-border-default)]">
          <button class="rl-icon-btn" :title="isMobile ? '菜单' : '折叠侧栏'" @click="toggleSider">
            <Navigation v-if="isMobile" /><PanelLeft v-else />
          </button>
          <div class="mx-xs h-16px w-1px bg-border-default" />
          <div class="flex min-w-0 items-center gap-xs text-md">
            <RouterLink to="/portal/" class="text-tertiary hover:text-primary">用户中心</RouterLink>
            <template v-if="route.path !== '/portal/'">
              <span class="text-quaternary">/</span>
              <span class="truncate text-primary">{{ route.meta.title }}</span>
            </template>
          </div>
          <div class="flex-1" />
          <button class="rl-icon-btn" :title="theme.isDark ? '切换为亮色' : '切换为暗色'" @click="theme.toggle()">
            <Sun v-if="theme.isDark" /><Moon v-else />
          </button>
        </header>

        <main class="min-w-0 flex-1 px-16px pb-lg md:px-lg">
          <div class="mx-auto max-w-1200px">
            <RouterView />
          </div>
        </main>

        <footer class="portal-html flex flex-wrap items-center justify-center gap-8px px-16px pb-24px text-sm text-tertiary">
          <div v-if="brand.footer" v-html="brand.footer" />
          <template v-else>
            <span>{{ user.issuer || '企业级安全远程接入网关' }}</span>
            <span>·</span>
            <span>Powered by <a href="https://github.com/wsczx/RemLink" target="_blank" rel="noopener" class="text-secondary hover:text-primary">RemLink</a></span>
          </template>
        </footer>
      </div>
    </div>
  </NConfigProvider>
</template>

<style>
.portal-sider { border-right: 1px solid var(--color-border-default); }
.portal-html a { color: var(--color-text-info); }
.portal-html a:hover { text-decoration: underline; }
.portal-html p:first-child { margin-top: 0; }
.portal-html p:last-child { margin-bottom: 0; }
</style>
