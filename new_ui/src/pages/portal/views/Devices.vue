<script setup lang="ts">
// 我的设备：当前账号的在线设备（30 秒自动刷新），可远程断开
import { NButton, NPopconfirm, NSpin } from 'naive-ui'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import Desktop from '~icons/fluent/desktop-20-regular'
import Phone from '~icons/fluent/phone-20-regular'
import ArrowUp from '~icons/fluent/arrow-up-16-regular'
import ArrowDown from '~icons/fluent/arrow-down-16-regular'
import PlugDisconnected from '~icons/fluent/plug-disconnected-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusDot from '@/components/StatusDot.vue'
import { usePortal } from '../store'

const p = usePortal()
const { devices, devicesLoading, pageVisible } = p

function deviceLabel(d: any) {
  if (d.device_type) return d.platform_version ? `${d.device_type} ${d.platform_version}` : d.device_type
  if (d.client === 'mobile') return '移动设备'
  if (d.client === 'pc') return '桌面设备'
  return d.transport === 'UDP' ? 'DTLS 设备' : 'TCP 设备'
}

function isMobile(d: any) {
  if (d.client === 'mobile') return true
  if (d.client === 'pc') return false
  const dt = (d.device_type || '').toLowerCase()
  return ['ios', 'iphone', 'ipad', 'android'].some(k => dt.includes(k))
}

// 已连接时长
function duration(t?: string) {
  if (!t) return ''
  const d = new Date(t.replace(/-/g, '/'))
  if (isNaN(d.getTime())) return ''
  const ms = Date.now() - d.getTime()
  if (ms < 0) return ''
  const min = Math.floor(ms / 60000)
  if (min < 1) return '刚刚上线'
  if (min < 60) return `已连接 ${min} 分钟`
  const h = Math.floor(min / 60), m = min % 60
  if (h < 24) return `已连接 ${h} 小时${m > 0 ? ` ${m} 分` : ''}`
  const day = Math.floor(h / 24), rh = h % 24
  return `已连接 ${day} 天${rh > 0 ? ` ${rh} 小时` : ''}`
}
</script>

<template>
  <div>
    <PageHeader title="我的设备" description="当前使用你的账号连接 VPN 的设备，发现陌生设备请立即断开并修改密码">
      <template #meta>
        <span v-if="devices.length" class="ax-badge text-success">{{ devices.length }} 在线</span>
      </template>
      <button class="rl-icon-btn" title="刷新" :disabled="devicesLoading" @click="p.loadDevices()"><ArrowClockwise /></button>
    </PageHeader>

    <RlPanel :padded="false">
      <NSpin :show="devicesLoading && !devices.length">
        <EmptyState v-if="!devices.length" title="暂无在线设备" description="使用客户端连接 VPN 后，设备会显示在这里" :icon="PlugDisconnected">
          <RouterLink v-if="pageVisible.clients" to="/portal/clients"><NButton>下载客户端</NButton></RouterLink>
        </EmptyState>
        <ul v-else class="m-0 list-none p-0">
          <li v-for="d in devices" :key="d.token" class="dev-row flex flex-wrap items-center gap-x-20px gap-y-10px px-20px py-16px">
            <div class="flex min-w-220px flex-1 items-center gap-12px">
              <div class="flex h-40px w-40px flex-shrink-0 items-center justify-center rounded-10px bg-fill text-xl text-secondary">
                <Phone v-if="isMobile(d)" /><Desktop v-else />
              </div>
              <div class="min-w-0">
                <div class="flex items-center gap-8px">
                  <span class="truncate text-md font-semibold">{{ deviceLabel(d) }}</span>
                  <span class="ax-badge" :class="d.transport === 'UDP' ? 'text-success' : 'text-secondary'">{{ d.transport }}</span>
                </div>
                <StatusDot class="mt-2px text-sm" tone="success" pulse>{{ duration(d.last_login) || '在线' }}</StatusDot>
              </div>
            </div>

            <dl class="m-0 grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-10px gap-y-2px text-sm">
              <dt class="text-tertiary">VPN 地址</dt><dd class="rl-mono m-0 truncate">{{ d.ip }}</dd>
              <dt class="text-tertiary">来源</dt><dd class="rl-mono m-0 truncate">{{ d.remote_addr }}</dd>
              <template v-if="d.mac_addr"><dt class="text-tertiary">MAC</dt><dd class="rl-mono m-0 truncate">{{ d.mac_addr }}</dd></template>
            </dl>

            <div class="min-w-140px text-sm">
              <div class="flex items-center gap-12px">
                <span class="inline-flex items-center gap-2px" title="实时上行速率"><ArrowUp class="text-tertiary" />{{ d.bandwidth_up }}</span>
                <span class="inline-flex items-center gap-2px" title="实时下行速率"><ArrowDown class="text-tertiary" />{{ d.bandwidth_down }}</span>
              </div>
              <div class="mt-2px text-tertiary">累计 ↑{{ d.bandwidth_up_all }} ↓{{ d.bandwidth_down_all }}</div>
            </div>

            <NPopconfirm @positive-click="p.kickDevice(d)">
              <template #trigger>
                <NButton size="small" type="error" secondary :loading="d._kicking">断开</NButton>
              </template>
              确定断开此设备的连接？
            </NPopconfirm>
          </li>
        </ul>
      </NSpin>
      <template v-if="devices.length" #footer>
        <span class="text-sm text-tertiary">每 30 秒自动刷新</span>
      </template>
    </RlPanel>
  </div>
</template>

<style scoped>
.dev-row + .dev-row { border-top: 1px solid var(--color-border-default); }
</style>
