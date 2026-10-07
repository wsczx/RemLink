<script setup lang="ts">
// 应用：WebVPN 单点登录后，在浏览器中一键访问内网 Web 应用、SSH 终端与文件服务
import { computed, ref } from 'vue'
import { NInput, NSpin } from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import Open from '~icons/fluent/open-16-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import AppsIcon from '~icons/fluent/apps-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
import { usePortal } from '../store'
import { protocolDef } from '@/pages/webvpn/components/app'

// 协议 → 图标（终端 / 文件夹 / 桌面 …）；HTTP 显示地球
const iconOf = (a: any) => protocolDef(a.protocol || 'http').icon
const remoteLabel = (a: any) => (a.protocol && a.protocol !== 'http' ? String(a.protocol).toUpperCase() : '')

const p = usePortal()
const { webvpnApps, webvpnLoading } = p

// 访问地址：后端按应用设置给出首选地址 url，以及可用的子域名地址 / 路径地址
const display = (u: string) => (u ? (u.startsWith('/') ? window.location.host + u : u.replace(/^https?:\/\//, '')) : '')
const primaryOf = (a: any): string => a.url || a.subdomain_url || a.path_url || ''
// 另一种可用的访问方式（两者皆可时提供切换入口）
const altOf = (a: any): { url: string; label: string } | null => {
  const primary = primaryOf(a)
  if (a.subdomain_url && a.path_url) {
    return primary === a.path_url ? { url: a.subdomain_url, label: '子域名' } : { url: a.path_url, label: '路径' }
  }
  return null
}
const modeLabel = (a: any) => (primaryOf(a) && primaryOf(a) === a.path_url ? '路径访问' : '子域名访问')

const kw = ref('')
const list = computed(() => {
  const k = kw.value.trim().toLowerCase()
  if (!k) return webvpnApps.value
  return webvpnApps.value.filter(a => [a.name, a.note, a.backend].some(v => String(v || '').toLowerCase().includes(k)))
})
</script>

<template>
  <div>
    <PageHeader title="应用" description="无需安装客户端，点击即可在浏览器中访问内网应用、终端与文件">
      <template #meta><span v-if="webvpnApps.length" class="ax-badge text-secondary">{{ webvpnApps.length }}</span></template>
      <NInput v-if="webvpnApps.length > 6" v-model:value="kw" placeholder="搜索应用" clearable class="w-200px!">
        <template #prefix><Search class="text-tertiary" /></template>
      </NInput>
      <button class="rl-icon-btn" title="刷新" :disabled="webvpnLoading" @click="p.loadWebVpnApps()"><ArrowClockwise /></button>
    </PageHeader>

    <NSpin :show="webvpnLoading && !webvpnApps.length">
      <EmptyState v-if="!list.length" :title="kw ? '没有匹配的应用' : '暂无可访问的应用'" :description="kw ? undefined : '管理员为你开放 WebVPN 应用后会显示在这里'" :icon="AppsIcon" />
      <div v-else class="grid grid-cols-1 gap-12px sm:grid-cols-2 xl:grid-cols-3">
        <div v-for="a in list" :key="a.name" class="app group">
          <a :href="primaryOf(a) || undefined" target="_blank" rel="noopener noreferrer" :title="modeLabel(a) + '：' + display(primaryOf(a))" class="app__main">
            <span class="flex h-40px w-40px flex-shrink-0 items-center justify-center rounded-10px bg-fill text-xl text-secondary group-hover:text-primary"><component :is="iconOf(a)" /></span>
            <span class="min-w-0 flex-1">
              <span class="flex items-center gap-6px">
                <span class="truncate text-md font-semibold">{{ a.note || a.name }}</span>
                <span v-if="remoteLabel(a)" class="ax-badge h-18px! flex-shrink-0 px-6px! text-secondary">{{ remoteLabel(a) }}</span>
              </span>
              <span class="rl-mono block truncate text-sm! text-secondary">{{ display(primaryOf(a)) || '暂无可用地址' }}</span>
            </span>
            <Open class="flex-shrink-0 text-tertiary group-hover:text-primary" />
          </a>
          <a
            v-if="altOf(a)" :href="altOf(a)!.url" target="_blank" rel="noopener noreferrer" class="app__alt"
            :title="display(altOf(a)!.url)"
          >{{ altOf(a)!.label }}</a>
        </div>
      </div>
    </NSpin>
  </div>
</template>

<style scoped>
.app {
  display: flex; align-items: center; gap: 8px; padding: 0 12px 0 0;
  border: 1px solid var(--color-border-default); border-radius: 12px; background: var(--color-primary);
  color: var(--color-text-primary); transition: border-color .15s;
}
.app:hover { border-color: var(--color-border-strong); }
.app__main { display: flex; flex: 1; min-width: 0; align-items: center; gap: 12px; padding: 14px 4px 14px 16px; color: inherit; }
.app__alt {
  flex-shrink: 0; padding: 2px 8px; border: 1px solid var(--color-border-default); border-radius: 6px;
  font-size: 12px; color: var(--color-text-secondary);
}
.app__alt:hover { color: var(--color-text-primary); border-color: var(--color-border-strong); }
</style>
