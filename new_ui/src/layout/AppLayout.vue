<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NDropdown, NIcon } from 'naive-ui'
import { useLocalStorage } from '@vueuse/core'
import Moon from '~icons/fluent/weather-moon-20-regular'
import Sun from '~icons/fluent/weather-sunny-20-regular'
import ShieldCheckmark from '~icons/fluent/shield-checkmark-20-regular'
import SignOut from '~icons/fluent/sign-out-20-regular'
import PanelLeft from '~icons/fluent/panel-left-20-regular'
import ChevronUpDown from '~icons/fluent/chevron-up-down-20-regular'
import { adminNav, findNav, type NavLeaf } from './nav'
import SideNav from '@/components/SideNav.vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { http } from '@/api/http'
import { applyBrandToDocument, normalizeImageSrc, type Brand } from '@/utils/brand'
import { ui } from '@/utils/ui'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const collapsed = useLocalStorage('rl-sider-collapsed', false)
const crumb = computed(() => findNav(adminNav, route.path))
const brand = ref<Partial<Brand>>({})
const brandTitle = computed(() => brand.value.title || 'RemLink')
// 只显示管理员自定义的 logo，默认不放图标
const logo = computed(() => normalizeImageSrc(brand.value.logo))
const initial = computed(() => (auth.user || 'A').slice(0, 1).toUpperCase())

function loadBrand() {
  http.get('/api/v1/portal/login-config').then(r => {
    if (r.data?.data) {
      brand.value = r.data.data
      applyBrandToDocument(brand.value)
    }
  }).catch(() => {})
}
onMounted(() => {
  loadBrand()
  // 后台保存品牌配置后广播该事件，即时刷新 logo/标题
  window.addEventListener('remlink:brand-updated', loadBrand)
})
onBeforeUnmount(() => window.removeEventListener('remlink:brand-updated', loadBrand))

// 诊断工具未启用时后端返回 403，先探测再打开，避免新窗口里看到错误页
async function openExternal(item: NavLeaf) {
  try {
    const r = await http.get(item.external!, { validateStatus: () => true })
    if (r.status === 200) window.open(item.external, '_blank')
    else ui.message.error('诊断工具未启用，请在服务端配置中开启 pprof 后重启服务')
  } catch {
    ui.message.error('无法访问诊断工具，请确认服务状态')
  }
}

const renderIcon = (icon: any) => () => h(NIcon, null, { default: () => h(icon) })
const userMenu = computed(() => [
  { key: 'security', label: '账号与安全', icon: renderIcon(ShieldCheckmark) },
  { key: 'd1', type: 'divider' },
  { key: 'logout', label: '退出登录', icon: renderIcon(SignOut) },
])

async function onUserMenu(key: string) {
  if (key === 'security') router.push('/admin/set/security')
  else if (key === 'logout') {
    await auth.logout()
    router.push('/login')
  }
}
</script>

<template>
  <div class="flex min-h-screen bg-body">
    <!-- 单栏侧边导航：分组 + 可折叠 -->
    <aside
      class="rl-sider sticky top-0 z-100 flex h-screen flex-shrink-0 flex-col bg-fill transition-[width] duration-200"
      :class="collapsed ? 'w-64px' : 'w-240px'"
    >
      <div class="flex h-56px flex-shrink-0 items-center gap-sm px-16px" :class="collapsed && 'justify-center px-0!'">
        <RouterLink to="/admin/home" class="flex min-w-0 items-center gap-sm">
          <img v-if="logo" :src="logo" class="block h-24px w-24px flex-shrink-0 object-contain" alt="">
          <span v-if="!collapsed" class="truncate text-lg font-bold tracking-tight">{{ brandTitle }}</span>
          <span v-else-if="!logo" class="text-lg font-bold">{{ brandTitle.slice(0, 1) }}</span>
        </RouterLink>
      </div>

      <div class="ax-scrollbar-none flex-1 overflow-y-auto px-10px pb-sm pt-4px">
        <SideNav :nav="adminNav" :collapsed="collapsed" @external="openExternal" />
      </div>

      <!-- 底部：节点状态 + 管理员 -->
      <div class="flex-shrink-0 p-10px b-t-1px b-t-solid b-[var(--color-border-default)]">
        <NDropdown trigger="click" placement="top-start" :options="userMenu" @select="onUserMenu">
          <div class="flex cursor-pointer items-center gap-sm rounded-8px p-6px hover:bg-action-hover" :class="collapsed && 'justify-center'">
            <div class="relative flex h-32px w-32px flex-shrink-0 items-center justify-center rounded-full bg-action text-md font-bold" style="color: var(--color-body)">
              {{ initial }}
              <span class="absolute bottom-0 right-0 h-8px w-8px rounded-full ring-2 ring-[var(--color-fill)]" style="background: var(--color-text-success)" />
            </div>
            <template v-if="!collapsed">
              <div class="min-w-0 flex-1 leading-tight">
                <div class="truncate text-md font-semibold">{{ auth.user || '-' }}</div>
                <div class="text-sm text-secondary">管理员 · 服务运行中</div>
              </div>
              <NIcon :size="16" class="text-tertiary"><ChevronUpDown /></NIcon>
            </template>
          </div>
        </NDropdown>
      </div>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col">
      <!-- 顶栏：折叠 / 面包屑 / 主题 -->
      <header class="sticky top-0 z-90 flex h-56px flex-shrink-0 items-center gap-sm bg-body px-lg b-b-1px b-b-solid b-[var(--color-border-default)]">
        <button class="rl-icon-btn" title="折叠侧栏" @click="collapsed = !collapsed"><PanelLeft /></button>
        <div class="mx-xs h-16px w-1px bg-border-default" />
        <div class="flex min-w-0 items-center gap-xs text-md">
          <template v-if="crumb?.leaf">
            <span class="text-tertiary">{{ crumb.node.label }}</span>
            <span class="text-quaternary">/</span>
            <span class="truncate text-primary">{{ crumb.leaf.label }}</span>
          </template>
          <span v-else-if="crumb" class="text-primary">{{ crumb.node.label }}</span>
          <span v-else class="text-primary">{{ route.meta.title }}</span>
        </div>
        <div class="flex-1" />
        <button class="rl-icon-btn" :title="theme.isDark ? '切换为亮色' : '切换为暗色'" @click="theme.toggle()">
          <Sun v-if="theme.isDark" /><Moon v-else />
        </button>
      </header>

      <main class="min-w-0 flex-1 px-lg pb-lg">
        <div class="mx-auto max-w-1600px">
          <RouterView />
        </div>
      </main>
    </div>
  </div>
</template>

<style>
.rl-sider { border-right: 1px solid var(--color-border-default); }
</style>
