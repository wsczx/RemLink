<script setup lang="ts">
// 在线会话：每 10 秒自动刷新；支持按字段搜索、显示休眠会话、重连 / 下线
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { NButton, NDataTable, NIcon, NInput, NSelect, NSwitch, type DataTableColumns } from 'naive-ui'
import { useDebounceFn } from '@vueuse/core'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import Phone from '~icons/fluent/phone-20-regular'
import Desktop from '~icons/fluent/desktop-20-regular'
import PlugDisconnected from '~icons/fluent/plug-disconnected-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import CountryFlag from '@/components/CountryFlag.vue'
import EmptyState from '@/components/EmptyState.vue'
import { errMsg, get, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { hostOf, isPrivateIp } from '@/utils/net'
import { fmtTime } from './components/shared'

interface OnlineRow {
  token: string
  is_active: boolean
  username: string
  nickname: string
  group: string
  mac_addr: string
  unique_mac: boolean
  ip: string
  remote_addr: string
  transport_protocol: string
  client: string
  device_type: string
  platform_version: string
  bandwidth_up: string
  bandwidth_down: string
  bandwidth_up_all: string
  bandwidth_down_all: string
  traffic_quota: string
  traffic_used: string
  traffic_reset: string
  last_login: string
  country_code?: string
}

const loading = ref(true)
const rows = ref<OnlineRow[]>([])
const searchCate = ref('username')
const searchText = ref('')
const showSleeper = ref(false)

const cateOptions = [
  { label: '用户名', value: 'username' },
  { label: '登录组', value: 'group' },
  { label: 'MAC 地址', value: 'mac_addr' },
  { label: 'VPN IP', value: 'ip' },
  { label: '来源地址', value: 'remote_addr' },
]

async function load() {
  try {
    const d = await get('/api/v1/user/online', {
      search_cate: searchCate.value,
      search_text: searchText.value,
      show_sleeper: showSleeper.value,
      one_offline: false,
    })
    rows.value = d?.datas || []
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
const loadDebounced = useDebounceFn(load, 300)

let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  load()
  timer = setInterval(load, 10000)
})
onBeforeUnmount(() => clearInterval(timer))

// 统计：在线仅计 is_active；休眠为已掉线待清理 / 重连
const stats = computed(() => {
  const active = rows.value.filter(r => r.is_active)
  return {
    online: active.length,
    mobile: active.filter(r => r.client === 'mobile').length,
    desktop: active.filter(r => r.client !== 'mobile').length,
    sleeper: rows.value.length - active.length,
  }
})

async function reline(r: OnlineRow) {
  try {
    await post(`/api/v1/user/reline?token=${encodeURIComponent(r.token)}`)
    ui.message.success('已断开连接，客户端将自动重连')
    load()
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

async function offline(r: OnlineRow) {
  if (!(await confirm(`确定将用户「${r.username}」下线吗？`, '下线确认'))) return
  try {
    await post(`/api/v1/user/offline?token=${encodeURIComponent(r.token)}`)
    ui.message.success('已下线')
    load()
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

const clientLabel = (r: OnlineRow) =>
  [r.device_type, r.platform_version].filter(Boolean).join(' ') || (r.client === 'mobile' ? '移动端' : '桌面端')
const resetLabel = (p: string) => ({ daily: '/日', weekly: '/周', monthly: '/月' } as Record<string, string>)[p] || ''

// 来源国旗：优先后端 country_code，否则内网显示 LAN，其他未知
function flagCode(r: OnlineRow) {
  if (r.country_code) return r.country_code
  return isPrivateIp(hostOf(r.remote_addr)) ? 'LAN' : ''
}

const str = (k: keyof OnlineRow) => (a: OnlineRow, b: OnlineRow) => String(a[k] ?? '').localeCompare(String(b[k] ?? ''))
const muted = (t = '—') => h('span', { class: 'text-quaternary' }, t)
const pair = (a: string, b: string) => h('span', { class: 'tabular-nums whitespace-nowrap' }, [
  h('span', { class: 'text-tertiary' }, '↑ '), a || '0',
  h('span', { class: 'mx-6px text-quaternary' }, '/'),
  h('span', { class: 'text-tertiary' }, '↓ '), b || '0',
])

const columns: DataTableColumns<OnlineRow> = [
  {
    title: '用户', key: 'username', minWidth: 190, fixed: 'left', sorter: str('username'),
    render: r => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'truncate font-semibold' }, [
        r.username,
        r.nickname ? h('span', { class: 'ml-6px font-normal text-secondary' }, r.nickname) : null,
      ]),
      h('div', { class: 'flex items-center gap-4px truncate text-sm text-secondary' }, [
        h(NIcon, { size: 14 }, { default: () => h(r.client === 'mobile' ? Phone : Desktop) }),
        clientLabel(r),
      ]),
    ]),
  },
  {
    title: '状态', key: 'is_active', width: 88,
    render: r => h(StatusDot, { tone: r.is_active ? 'success' : 'muted', pulse: r.is_active }, { default: () => (r.is_active ? '在线' : '休眠') }),
  },
  {
    title: '登录组', key: 'group', width: 120, sorter: str('group'),
    render: r => (r.group ? h('span', { class: 'rl-kbd' }, r.group) : muted()),
  },
  {
    title: 'VPN IP', key: 'ip', width: 140, sorter: str('ip'),
    render: r => h('span', { class: 'rl-mono' }, r.ip || '—'),
  },
  {
    title: '来源地址', key: 'remote_addr', width: 210, sorter: str('remote_addr'),
    render: r => h('div', { class: 'flex items-center gap-8px min-w-0' }, [
      h(CountryFlag, { code: flagCode(r) }),
      h('span', { class: 'rl-mono truncate' }, r.remote_addr || '—'),
    ]),
  },
  {
    title: 'MAC 地址', key: 'mac_addr', width: 190, sorter: str('mac_addr'),
    render: r => h('div', { class: 'flex items-center gap-6px' }, [
      h('span', { class: 'rl-mono' }, r.mac_addr || '—'),
      r.unique_mac ? h('span', { class: 'ax-badge text-success', title: '唯一 MAC' }, '唯一') : null,
    ]),
  },
  {
    title: '协议', key: 'transport_protocol', width: 80, sorter: str('transport_protocol'),
    render: r => (r.transport_protocol ? h('span', { class: 'rl-kbd' }, r.transport_protocol) : muted()),
  },
  { title: '实时速率', key: 'bw', width: 180, render: r => pair(r.bandwidth_up, r.bandwidth_down) },
  { title: '累计流量', key: 'bw_all', width: 190, render: r => pair(r.bandwidth_up_all, r.bandwidth_down_all) },
  {
    title: '流量配额', key: 'quota', width: 160,
    render: r => (r.traffic_quota
      ? h('span', { class: 'tabular-nums whitespace-nowrap' }, [
          h('span', { class: 'font-semibold' }, r.traffic_used),
          h('span', { class: 'mx-4px text-quaternary' }, '/'),
          h('span', { class: 'text-secondary' }, r.traffic_quota),
          r.traffic_reset ? h('span', { class: 'ml-2px text-sm text-tertiary' }, resetLabel(r.traffic_reset)) : null,
        ])
      : muted('不限')),
  },
  {
    title: '登录时间', key: 'last_login', width: 160, sorter: str('last_login'),
    render: r => h('span', { class: 'text-secondary tabular-nums' }, fmtTime(r.last_login)),
  },
  {
    title: '', key: 'actions', width: 110, align: 'right', fixed: 'right',
    render: r => h('div', { class: 'flex justify-end gap-12px' }, [
      h(NButton, { text: true, size: 'small', disabled: !r.is_active, onClick: () => reline(r) }, { default: () => '重连' }),
      h(NButton, { text: true, size: 'small', type: 'error', disabled: !r.is_active, onClick: () => offline(r) }, { default: () => '下线' }),
    ]),
  },
]
</script>

<template>
  <div>
    <PageHeader title="在线会话" description="当前已建立的 VPN 隧道。重连会断开数据通道让客户端自动重建；下线将结束会话。">
      <NButton @click="load">
        <template #icon><NIcon><ArrowClockwise /></NIcon></template>
        刷新
      </NButton>
    </PageHeader>

    <div class="grid gap-16px">
      <div class="grid grid-cols-2 gap-16px lg:grid-cols-4">
        <StatCard label="当前在线" :value="stats.online" tone="success" />
        <StatCard label="移动端" :value="stats.mobile" :icon="Phone" />
        <StatCard label="桌面端" :value="stats.desktop" :icon="Desktop" />
        <StatCard label="休眠会话" :value="stats.sleeper" :hint="showSleeper ? '已掉线，等待清理或重连' : '开启「显示休眠」后统计'" />
      </div>

      <RlPanel :padded="false">
        <template #toolbar>
          <NSelect v-model:value="searchCate" :options="cateOptions" class="w-120px!" @update:value="load" />
          <NInput v-model:value="searchText" placeholder="搜索…" clearable class="w-240px!" @update:value="loadDebounced">
            <template #prefix><NIcon><Search /></NIcon></template>
          </NInput>
          <label class="flex cursor-pointer items-center gap-6px text-md text-secondary">
            <NSwitch v-model:value="showSleeper" size="small" @update:value="load" />
            显示休眠
          </label>
          <div class="flex-1" />
          <span class="text-sm text-tertiary">每 10 秒自动刷新</span>
          <span class="text-sm text-secondary">共 {{ rows.length }} 个会话</span>
          <button class="rl-icon-btn" title="刷新" @click="load"><ArrowClockwise /></button>
        </template>

        <NDataTable
          :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: OnlineRow) => r.token"
          size="small" :scroll-x="1820"
        >
          <template #empty>
            <EmptyState :icon="PlugDisconnected" title="暂无在线会话" description="用户建立 VPN 连接后会显示在这里。" />
          </template>
        </NDataTable>
      </RlPanel>
    </div>
  </div>
</template>
