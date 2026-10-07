<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NDataTable, NIcon, NProgress, NSelect, type DataTableColumns } from 'naive-ui'
import { useIntervalFn } from '@vueuse/core'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'
import PersonAvailable from '~icons/fluent/person-available-20-regular'
import ArrowDown from '~icons/fluent/arrow-download-20-regular'
import ArrowUp from '~icons/fluent/arrow-upload-20-regular'
import People from '~icons/fluent/people-20-regular'
import Board from '~icons/fluent/developer-board-20-regular'
import Memory from '~icons/fluent/memory-16-regular'
import HardDrive from '~icons/fluent/hard-drive-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import TrendUp from '~icons/fluent/arrow-trending-20-regular'
import TrendDown from '~icons/fluent/arrow-trending-down-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import UnderlineTabs from '@/components/UnderlineTabs.vue'
import CountryFlag from '@/components/CountryFlag.vue'
import EmptyState from '@/components/EmptyState.vue'
import AxLineChart, { type LineSeries } from '@/components/AxLineChart.vue'
import AxDonut from '@/components/AxDonut.vue'
import AxMap from '@/components/AxMap.vue'
import { get, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import { countryName, hostOf, isPrivateIp } from '@/utils/net'
import { chartColors } from '@/theme/theme'
import { useThemeStore } from '@/stores/theme'

dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

type Action = 'online' | 'network'
type Scope = 'rt' | '1h' | '24h' | '7d' | '30d'

const scopes: { label: string; value: Scope }[] = [
  { label: '实时', value: 'rt' },
  { label: '最近 1 小时', value: '1h' },
  { label: '最近 1 天', value: '24h' },
  { label: '最近 1 周', value: '7d' },
]

const theme = useThemeStore()
const scope = ref<Scope>('rt')
const counts = reactive({ online: 0, user: 0, group: 0, ip_map: 0 })
const sys = ref<any>(null)
const raw = reactive<Record<Action, any[]>>({ online: [], network: [] })
const sessions = ref<any[]>([])
const geo = ref<{ code: string; name?: string; value: number }[]>([])
const loading = ref(false)
const updatedAt = ref(0)
const now = ref(Date.now())
useIntervalFn(() => { now.value = Date.now() }, 15000)
const updatedText = computed(() => (updatedAt.value ? `更新于 ${dayjs(updatedAt.value).from(now.value)}` : ''))

const reqId: Record<Action, number> = { online: 0, network: 0 }
const ts = (s: string) => new Date(s).getTime()
const toMbps = (bytes: number) => (bytes ? +(bytes / 1024 ** 2 * 8).toFixed(2) : 0)

async function fetchStats(action: Action) {
  const id = ++reqId[action]
  const data = await get('/api/v1/statsinfo/list', { action, scope: scope.value })
  if (id === reqId[action]) raw[action] = data?.datas || []
}

// 并发拉取，任何一项失败只提示一次，不影响其他区块
async function refresh() {
  loading.value = true
  const jobs = [
    fetchStats('online'),
    fetchStats('network'),
    get('/api/v1/set/home').then(d => d?.counts && Object.assign(counts, d.counts)),
    get('/api/v1/set/system').then(d => { sys.value = d }),
    get('/api/v1/user/online').then(d => { sessions.value = d?.datas || [] }),
    // 地理分布接口可选：未实现时地图保持空白
    get('/api/v1/statsinfo/geo').then(d => { geo.value = d?.datas || [] }).catch(() => {}),
  ]
  const res = await Promise.allSettled(jobs)
  const failed = res.find(r => r.status === 'rejected') as PromiseRejectedResult | undefined
  if (failed && failed.reason?.response?.status !== 401) ui.message.error(errMsg(failed.reason))
  updatedAt.value = now.value = Date.now()
  loading.value = false
}

function changeScope(s: Scope) {
  scope.value = s
  refresh()
}

const colors = computed(() => chartColors[theme.isDark ? 'dark' : 'light'])
const last = <T,>(arr: T[]) => arr[arr.length - 1]

// 窗口内首尾变化率
function delta(values: number[]) {
  if (values.length < 2 || !values[0]) return null
  return ((values[values.length - 1] - values[0]) / values[0]) * 100
}

const series = computed(() => {
  const online = raw.online.map(d => d.num as number)
  const down = raw.network.map(d => toMbps(d.down))
  const up = raw.network.map(d => toMbps(d.up))
  const pts = (src: any[], vals: number[]) => src.map((d, i) => [ts(d.created_at), vals[i]] as [number, number])
  return { online, down, up, onlinePts: pts(raw.online, online), downPts: pts(raw.network, down), upPts: pts(raw.network, up) }
})

// —— 指标卡（迷你图统一 info 蓝）——
const kpis = computed(() => {
  const s = series.value
  const mini = (name: string, data: [number, number][]): LineSeries[] => [{ name, color: colors.value.info, data }]
  return [
    { label: '在线会话', icon: PersonAvailable, value: last(s.online) ?? counts.online, unit: '', delta: delta(s.online), to: '/admin/user/online', series: mini('在线', s.onlinePts) },
    { label: '下行带宽', icon: ArrowDown, value: last(s.down) ?? 0, unit: 'Mbps', delta: delta(s.down), to: '', series: mini('下行', s.downPts) },
    { label: '上行带宽', icon: ArrowUp, value: last(s.up) ?? 0, unit: 'Mbps', delta: delta(s.up), to: '', series: mini('上行', s.upPts) },
  ]
})

// —— 流量趋势大图 ——
type Trend = 'online' | 'network'
const trend = ref<Trend>('network')
const trendTabs: { label: string; value: Trend }[] = [
  { label: '带宽', value: 'network' },
  { label: '在线', value: 'online' },
]
const trendSeries = computed<LineSeries[]>(() => {
  const s = series.value
  const p = colors.value.palette
  if (trend.value === 'online') return [{ name: '在线会话', color: colors.value.info, data: s.onlinePts, areaColor: true, bold: true }]
  return [
    { name: '下行', color: p[0], data: s.downPts, areaColor: true, bold: true },
    { name: '上行', color: p[1], data: s.upPts, bold: true },
  ]
})
const trendLegend = computed(() => trendSeries.value.map(s => ({ name: s.name, color: s.color })))

// —— 节点健康 ——
function level(p: number): 'success' | 'warning' | 'error' {
  return p >= 90 ? 'error' : p >= 75 ? 'warning' : 'success'
}
const meters = computed(() => {
  const s = sys.value
  if (!s) return []
  return [
    { label: 'CPU', icon: Board, value: +(s.cpu?.percent ?? 0), sub: `${s.cpu?.core ?? '-'} 核 · ${s.cpu?.ghz ?? ''}` },
    { label: '内存', icon: Memory, value: +(s.mem?.percent ?? 0), sub: `${s.mem?.free ?? '-'} 可用 / 共 ${s.mem?.total ?? '-'}` },
    { label: '磁盘', icon: HardDrive, value: +(s.disk?.percent ?? 0), sub: `${s.disk?.free ?? '-'} 可用 / 共 ${s.disk?.total ?? '-'}` },
  ]
})
const health = computed(() => {
  const worst = Math.max(0, ...meters.value.map(m => m.value))
  return sys.value ? level(worst) : 'muted'
})
function fmtUptime(sec?: number) {
  const s = Math.max(0, Math.floor(sec || 0))
  const d = Math.floor(s / 86400)
  const hh = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  return [d && `${d} 天`, hh && `${hh} 小时`, `${m} 分钟`].filter(Boolean).join(' ')
}
const facts = computed(() => {
  const s = sys.value?.sys
  if (!s) return []
  return [
    { k: '运行时间', v: fmtUptime(s.uptime) },
    { k: '系统负载', v: s.load },
    { k: '版本', v: s.appVersion },
    { k: '平台', v: [s.goOs, s.goArch].filter(Boolean).join('/') },
  ]
})

// —— 来源地区排行 ——
const geoTotal = computed(() => geo.value.reduce((s, d) => s + d.value, 0))
const geoRank = computed(() => [...geo.value]
  .sort((a, b) => b.value - a.value)
  .slice(0, 8)
  .map(d => ({ ...d, label: countryName(d.code) || d.name || d.code, pct: geoTotal.value ? (d.value / geoTotal.value) * 100 : 0 })))

// —— 在线分布 ——
function tally(key: string, fallback: string) {
  const m = new Map<string, number>()
  for (const s of sessions.value) {
    const k = s[key] || fallback
    m.set(k, (m.get(k) || 0) + 1)
  }
  return [...m.entries()].map(([name, value]) => ({ name, value })).sort((a, b) => b.value - a.value)
}
const byGroup = computed(() => tally('group', '未分组'))
const byDevice = computed(() => tally('device_type', '未知'))

// —— 最近会话 ——
function regionOf(r: any) {
  if (r.country_code) return String(r.country_code)
  return isPrivateIp(hostOf(r.remote_addr)) ? 'LAN' : ''
}
const columns: DataTableColumns<any> = [
  {
    title: '用户', key: 'username', minWidth: 160,
    render: r => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'truncate font-semibold' }, r.nickname ? `${r.username}（${r.nickname}）` : r.username),
      h('div', { class: 'truncate text-sm text-secondary' }, r.group || '未分组'),
    ]),
  },
  {
    title: '来源', key: 'remote_addr', minWidth: 180,
    render: r => h('div', { class: 'flex min-w-0 items-center gap-8px' }, [
      h(CountryFlag, { code: regionOf(r) }),
      h('div', { class: 'min-w-0' }, [
        h('div', { class: 'rl-mono truncate' }, hostOf(r.remote_addr) || '—'),
        h('div', { class: 'truncate text-sm text-secondary' }, r.transport_protocol || ''),
      ]),
    ]),
  },
  { title: '分配 IP', key: 'ip', minWidth: 120, render: r => h('span', { class: 'rl-mono' }, r.ip || '—') },
  {
    title: '实时带宽', key: 'bw', minWidth: 120,
    render: r => h('div', { class: 'tabular-nums' }, [h('div', `↓ ${r.bandwidth_down || '0'}`), h('div', { class: 'text-sm text-secondary' }, `↑ ${r.bandwidth_up || '0'}`)]),
  },
  {
    title: '终端', key: 'device_type', minWidth: 120,
    render: r => h('div', { class: 'min-w-0' }, [h('div', { class: 'truncate' }, r.device_type || '—'), h('div', { class: 'truncate text-sm text-secondary' }, r.platform_version)]),
  },
]

onMounted(refresh)
// 实时模式每 10 秒刷新
useIntervalFn(() => { if (scope.value === 'rt' && !loading.value) refresh() }, 10000)
</script>

<template>
  <div>
    <PageHeader title="仪表盘" description="节点运行概况">
      <template #meta>
        <StatusDot :tone="health" :pulse="health === 'success'">{{ sys?.sys?.hostname || '本节点' }}</StatusDot>
      </template>
      <span class="text-sm text-tertiary">{{ updatedText }}</span>
      <NSelect :value="scope" :options="scopes" class="w-140px!" @update:value="changeScope" />
      <button class="rl-icon-btn" title="刷新" :disabled="loading" @click="refresh">
        <ArrowClockwise :class="loading && 'animate-spin'" />
      </button>
    </PageHeader>

    <div class="grid gap-16px">
      <!-- 指标卡 -->
      <div class="grid gap-16px sm:grid-cols-2 xl:grid-cols-4">
        <StatCard v-for="k in kpis" :key="k.label" :label="k.label" :icon="k.icon" :value="k.value" :unit="k.unit" :to="k.to || undefined">
          <template #delta>
            <span v-if="k.delta !== null" class="ml-6px inline-flex items-center gap-2px text-sm" :class="k.delta >= 0 ? 'text-success' : 'text-error'">
              <NIcon :size="14"><TrendUp v-if="k.delta >= 0" /><TrendDown v-else /></NIcon>{{ Math.abs(k.delta).toFixed(1) }}%
            </span>
          </template>
          <div class="h-44px">
            <AxLineChart :series="k.series" :show-x-axis="false" :show-y-axis="false" :tooltip="false" />
          </div>
        </StatCard>
        <StatCard label="用户总数" :icon="People" :value="counts.user" to="/admin/user/list">
          <div class="grid grid-cols-2 gap-8px text-sm">
            <RouterLink to="/admin/group/list" class="rounded-8px bg-fill px-10px py-6px hover:bg-action-hover">
              <div class="text-tertiary">用户组</div>
              <div class="text-md font-semibold tabular-nums">{{ counts.group }}</div>
            </RouterLink>
            <RouterLink to="/admin/user/ip_map" class="rounded-8px bg-fill px-10px py-6px hover:bg-action-hover">
              <div class="text-tertiary">IP 映射</div>
              <div class="text-md font-semibold tabular-nums">{{ counts.ip_map }}</div>
            </RouterLink>
          </div>
        </StatCard>
      </div>

      <!-- 流量趋势 + 节点健康 -->
      <div class="grid gap-16px xl:grid-cols-3">
        <RlPanel title="流量趋势" :subtitle="scopes.find(s => s.value === scope)?.label" class="xl:col-span-2">
          <template #extra>
            <UnderlineTabs v-model="trend" :options="trendTabs" class="b-b-0!" />
          </template>
          <div class="mb-8px flex items-center gap-16px text-sm text-secondary">
            <span v-for="l in trendLegend" :key="l.name" class="inline-flex items-center gap-6px">
              <span class="h-2px w-12px rounded-full" :style="{ background: l.color }" />{{ l.name }}
            </span>
            <span class="ml-auto">{{ trend === 'network' ? '单位 Mbps' : '单位 会话' }}</span>
          </div>
          <div class="h-280px">
            <AxLineChart :series="trendSeries" :unit="trend === 'network' ? 'Mbps' : ''" />
          </div>
        </RlPanel>

        <RlPanel title="节点健康" :subtitle="sys?.sys?.platform">
          <template #extra>
            <RouterLink to="/admin/set/system" class="text-sm text-secondary hover:text-primary!">详情</RouterLink>
          </template>
          <div v-if="!sys" class="py-40px text-center text-secondary">加载中…</div>
          <template v-else>
            <div class="grid gap-16px">
              <div v-for="m in meters" :key="m.label">
                <div class="flex items-center gap-6px">
                  <NIcon :size="16" class="text-secondary"><component :is="m.icon" /></NIcon>
                  <span>{{ m.label }}</span>
                  <span class="ml-auto font-semibold tabular-nums" :class="`text-${level(m.value)}`">{{ m.value }}%</span>
                </div>
                <NProgress type="line" :percentage="m.value" :status="level(m.value)" :show-indicator="false" :height="6" class="mt-6px" />
                <div class="mt-4px truncate text-sm text-tertiary">{{ m.sub }}</div>
              </div>
            </div>
            <dl class="m-0 mt-20px grid grid-cols-[auto_1fr] gap-x-16px gap-y-8px b-t-1px b-t-solid b-[var(--color-border-default)] pt-16px text-sm">
              <template v-for="f in facts" :key="f.k">
                <dt class="text-secondary">{{ f.k }}</dt>
                <dd class="m-0 truncate text-right tabular-nums">{{ f.v || '—' }}</dd>
              </template>
            </dl>
          </template>
        </RlPanel>
      </div>

      <!-- 来源地区 + 在线分布 -->
      <div class="grid gap-16px xl:grid-cols-3">
        <RlPanel title="来源地区" :subtitle="`按国家 / 地区统计在线会话，共 ${geoTotal} 个`" :padded="false" class="xl:col-span-2">
          <div class="grid lg:grid-cols-[minmax(0,1fr)_280px]">
            <div class="h-340px p-12px">
              <AxMap :data="geo" unit="在线" />
            </div>
            <div class="b-t-1px b-t-solid b-[var(--color-border-default)] p-16px lg:b-l-1px lg:b-l-solid lg:b-t-0">
              <div class="mb-10px text-sm text-secondary">排行 Top 8</div>
              <div v-if="!geoRank.length" class="py-32px text-center text-sm text-tertiary">暂无地区数据</div>
              <div v-for="(g, i) in geoRank" :key="g.code" class="py-6px">
                <div class="flex items-center gap-8px">
                  <span class="w-14px text-sm text-tertiary tabular-nums">{{ i + 1 }}</span>
                  <CountryFlag :code="g.code" />
                  <span class="min-w-0 flex-1 truncate">{{ g.label }}</span>
                  <span class="font-semibold tabular-nums">{{ g.value }}</span>
                  <span class="w-42px text-right text-sm text-tertiary tabular-nums">{{ g.pct.toFixed(0) }}%</span>
                </div>
                <div class="ml-22px mt-4px h-4px overflow-hidden rounded-full bg-fill">
                  <div class="h-full rounded-full" :style="{ width: `${g.pct}%`, background: colors.info }" />
                </div>
              </div>
            </div>
          </div>
        </RlPanel>

        <RlPanel title="在线分布" subtitle="按当前在线会话统计" :padded="false">
          <div class="py-12px">
            <div class="px-20px text-sm text-secondary">用户组</div>
            <AxDonut :data="byGroup" center-label="在线" />
          </div>
          <div class="b-t-1px b-t-solid b-[var(--color-border-default)] py-12px">
            <div class="px-20px text-sm text-secondary">终端类型</div>
            <AxDonut :data="byDevice" center-label="终端" />
          </div>
        </RlPanel>
      </div>

      <!-- 最近会话 -->
      <RlPanel title="最近会话" :subtitle="`当前 ${sessions.length} 个在线会话`" :padded="false">
        <template #extra>
          <RouterLink to="/admin/user/online" class="text-sm text-secondary hover:text-primary!">查看全部 →</RouterLink>
        </template>
        <NDataTable
          :columns="columns" :data="sessions.slice(0, 8)" :bordered="false" size="small"
          :row-key="(r: any) => r.token" :scroll-x="760"
        >
          <template #empty><EmptyState title="当前没有在线会话" description="用户连接 VPN 后会显示在这里" /></template>
        </NDataTable>
      </RlPanel>
    </div>
  </div>
</template>
