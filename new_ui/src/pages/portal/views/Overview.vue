<script setup lang="ts">
// 概览：问候 + 连接状态 + 账号摘要 + 公告 + 快捷入口
import { computed, type Component } from 'vue'
import { NAlert, NIcon } from 'naive-ui'
import Copy from '~icons/fluent/copy-20-regular'
import LinkIcon from '~icons/fluent/link-20-regular'
import Open from '~icons/fluent/open-16-regular'
import ChevronRight from '~icons/fluent/chevron-right-16-regular'
import Laptop from '~icons/fluent/laptop-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import AppsIcon from '~icons/fluent/apps-20-regular'
import ShieldKeyhole from '~icons/fluent/shield-keyhole-20-regular'
import ShieldCheckmark from '~icons/fluent/shield-checkmark-20-regular'
import Certificate from '~icons/fluent/certificate-20-regular'
import PlugConnected from '~icons/fluent/plug-connected-20-regular'
import PlugDisconnected from '~icons/fluent/plug-disconnected-20-regular'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import { usePortal } from '../store'
import { copyText, parseJson, userLabel } from '../shared'

const p = usePortal()
const { user, dashboard, devices, devicesLoaded, certs, webvpnApps, pageVisible, expire } = p

const hour = new Date().getHours()
const greeting = hour < 6 ? '夜深了' : hour < 12 ? '早上好' : hour < 14 ? '中午好' : hour < 18 ? '下午好' : '晚上好'

const showAnnouncement = computed(() => dashboard.value.announcement_enabled !== 2 && !!dashboard.value.announcement)
const announcementType = computed(() => {
  const t = dashboard.value.announcement_level
  return t === 'success' || t === 'warning' || t === 'error' ? t : 'info'
})

const quickLinks = computed(() => {
  const l = parseJson<any[]>(dashboard.value.quick_links, [])
  return Array.isArray(l) ? l : []
})
const showQuickLinks = computed(() => dashboard.value.quick_links_enabled !== 2 && quickLinks.value.length > 0)
const isIconImage = (icon?: string) => !!icon && /^(https?:\/\/|\/\/|\.\.?\/|data:image\/)/i.test(icon)
// 旧版图标类名（el-icon 等）在新界面无法渲染，统一回退为链接图标
const isIconClass = (icon?: string) => !!icon && (/\s/.test(icon) || /^(el-icon|icon|fa|fas|far|fab|ivu-icon|anticon)/i.test(icon))

const connected = computed(() => devices.value.length > 0)
const validCerts = computed(() => certs.value.filter(c => c.status === 0).length)

// 快捷入口：只列出管理员开放的页面
interface Shortcut { key: string; to: string; icon: Component; title: string; desc: string }
const shortcuts = computed<Shortcut[]>(() => {
  const list: Shortcut[] = [
    { key: 'devices', to: '/portal/devices', icon: Laptop, title: '我的设备', desc: connected.value ? `${devices.value.length} 台设备在线` : '查看与断开在线设备' },
    { key: 'clients', to: '/portal/clients', icon: ArrowDownload, title: '客户端下载', desc: '下载客户端并按步骤连接' },
    { key: 'apps', to: '/portal/apps', icon: AppsIcon, title: '应用', desc: webvpnApps.value.length ? `${webvpnApps.value.length} 个内网应用` : '通过浏览器访问内网应用' },
    { key: 'access', to: '/portal/access', icon: ShieldKeyhole, title: '访问权限', desc: `${(user.value.groups || []).length} 个用户组` },
    { key: 'security', to: '/portal/security', icon: ShieldCheckmark, title: '账号安全', desc: user.value.otp_enabled ? '已启用两步验证' : '修改密码、两步验证' },
    { key: 'certs', to: '/portal/certs', icon: Certificate, title: '我的证书', desc: certs.value.length ? `${validCerts.value} 个可用证书` : '下载客户端证书' },
  ]
  return list.filter(s => pageVisible.value[s.key])
})
</script>

<template>
  <div class="grid gap-16px pb-md pt-28px">
    <!-- 问候 -->
    <section>
      <h1 class="m-0 text-4xl font-semibold leading-32px">{{ greeting }}，{{ user.name || user.username }}</h1>
      <div class="mt-6px flex flex-wrap items-center gap-x-16px gap-y-6px text-md text-secondary">
        <StatusDot :tone="user.status === 1 ? 'success' : 'error'">{{ user.status === 1 ? '账号正常' : '账号异常' }}</StatusDot>
        <span :class="expire.tone !== 'success' && `text-${expire.tone}`">{{ expire.label }}</span>
        <span>{{ p.typeLabel.value }}</span>
      </div>
    </section>

    <!-- 公告 -->
    <NAlert v-if="showAnnouncement" :type="announcementType" :show-icon="true">
      <div class="portal-html" v-html="dashboard.announcement" />
    </NAlert>

    <!-- 连接状态 -->
    <RlPanel :padded="false">
      <div class="flex flex-wrap items-center gap-16px px-20px py-18px">
        <div
          class="flex h-44px w-44px flex-shrink-0 items-center justify-center rounded-10px text-[22px]"
          :class="connected ? 'text-success' : 'text-tertiary'"
          style="background: color-mix(in srgb, currentColor 12%, transparent)"
        >
          <PlugConnected v-if="connected" /><PlugDisconnected v-else />
        </div>
        <div class="min-w-0 flex-1">
          <div class="text-lg font-semibold">
            <template v-if="!devicesLoaded">正在获取连接状态…</template>
            <template v-else-if="connected">已连接 · {{ devices.length }} 台设备在线</template>
            <template v-else>当前未连接</template>
          </div>
          <div class="mt-2px text-md text-secondary">
            <template v-if="connected">
              {{ devices.map(d => d.ip).filter(Boolean).slice(0, 3).join('、') }}{{ devices.length > 3 ? ' 等' : '' }}
            </template>
            <template v-else>使用 VPN 客户端连接下方服务器地址后，即可访问内网资源</template>
          </div>
        </div>
        <RouterLink v-if="connected && pageVisible.devices" to="/portal/devices" class="text-md text-secondary hover:text-primary">管理设备</RouterLink>
        <RouterLink v-else-if="!connected && pageVisible.clients" to="/portal/clients" class="text-md text-secondary hover:text-primary">如何连接</RouterLink>
      </div>
      <div class="flex flex-wrap items-center gap-x-12px gap-y-4px b-t-1px b-t-solid b-[var(--color-border-default)] bg-fill px-20px py-10px">
        <span class="text-sm text-secondary">服务器地址</span>
        <span class="rl-mono min-w-0 truncate text-md font-semibold">{{ user.server_addr || '-' }}</span>
        <button v-if="user.server_addr" class="rl-icon-btn h-24px! w-24px! text-md!" title="复制" @click="copyText(user.server_addr)"><Copy /></button>
      </div>
    </RlPanel>

    <!-- 账号摘要 -->
    <RlPanel title="账号" :padded="false">
      <dl class="summary m-0 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4">
        <div class="summary-cell">
          <dt class="text-sm text-secondary">用户名</dt>
          <dd class="m-0 mt-2px truncate text-md font-semibold">{{ userLabel(user.username, user.name) || '-' }}</dd>
        </div>
        <div class="summary-cell">
          <dt class="text-sm text-secondary">账号类型</dt>
          <dd class="m-0 mt-2px truncate text-md font-semibold">{{ p.typeLabel.value }}</dd>
        </div>
        <div class="summary-cell">
          <dt class="text-sm text-secondary">用户组</dt>
          <dd class="m-0 mt-2px flex flex-wrap gap-4px">
            <span v-for="g in user.groups || []" :key="g" class="rl-kbd">{{ g }}</span>
            <span v-if="!(user.groups || []).length" class="text-md text-tertiary">无</span>
          </dd>
        </div>
        <div class="summary-cell">
          <dt class="text-sm text-secondary">有效期至</dt>
          <dd class="m-0 mt-2px truncate text-md font-semibold" :class="`text-${expire.tone}`">{{ expire.value }}</dd>
        </div>
      </dl>
    </RlPanel>

    <!-- 快捷入口 -->
    <section v-if="shortcuts.length">
      <div class="mb-10px text-lg font-semibold">快捷入口</div>
      <div class="grid grid-cols-1 gap-12px sm:grid-cols-2 lg:grid-cols-3">
        <RouterLink v-for="s in shortcuts" :key="s.key" :to="s.to" class="shortcut group">
          <span class="flex h-36px w-36px flex-shrink-0 items-center justify-center rounded-8px bg-fill text-lg text-secondary group-hover:text-primary">
            <component :is="s.icon" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="block truncate text-md font-semibold">{{ s.title }}</span>
            <span class="block truncate text-sm text-secondary">{{ s.desc }}</span>
          </span>
          <NIcon class="text-tertiary group-hover:text-primary"><ChevronRight /></NIcon>
        </RouterLink>
      </div>
    </section>

    <!-- 管理员配置的快捷链接 -->
    <RlPanel v-if="showQuickLinks" title="常用链接" subtitle="管理员为你准备的常用站点">
      <div class="flex flex-wrap gap-8px">
        <a
          v-for="(l, i) in quickLinks" :key="i" :href="l.url" target="_blank" rel="noopener"
          class="inline-flex h-32px items-center gap-6px rounded-8px border border-solid border-[var(--color-border-default)] px-12px text-md transition-colors hover:border-[var(--color-border-strong)]"
        >
          <img v-if="isIconImage(l.icon)" :src="l.icon" class="h-16px w-16px object-contain" alt="">
          <LinkIcon v-else-if="!l.icon || isIconClass(l.icon)" class="text-secondary" />
          <span v-else>{{ l.icon }}</span>
          <span>{{ l.label }}</span>
          <Open class="text-tertiary" />
        </a>
      </div>
    </RlPanel>
  </div>
</template>

<style scoped>
.summary-cell { min-width: 0; padding: 14px 20px; }
.summary-cell + .summary-cell { border-top: 1px solid var(--color-border-default); }
@media (min-width: 640px) {
  .summary-cell + .summary-cell { border-top: 0; }
  .summary-cell:nth-child(n + 3) { border-top: 1px solid var(--color-border-default); }
  .summary-cell:nth-child(2n) { border-left: 1px solid var(--color-border-default); }
}
@media (min-width: 1024px) {
  .summary-cell:nth-child(n + 3) { border-top: 0; }
  .summary-cell + .summary-cell { border-left: 1px solid var(--color-border-default); }
}
.shortcut {
  display: flex; align-items: center; gap: 12px; padding: 14px 16px;
  border: 1px solid var(--color-border-default); border-radius: 12px; background: var(--color-primary);
  transition: border-color .15s;
}
.shortcut:hover { border-color: var(--color-border-strong); }
</style>
