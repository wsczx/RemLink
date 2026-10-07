<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import {
  NDataTable, NDrawer, NDrawerContent, NIcon, NInput, NPagination, NSelect, NTooltip,
  type DataTableColumns, type SelectOption,
} from 'naive-ui'
import dayjs from 'dayjs'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArrowRight from '~icons/fluent/arrow-right-16-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import Plug from '~icons/fluent/plug-connected-20-regular'
import CheckCircle from '~icons/fluent/checkmark-circle-20-regular'
import ShieldError from '~icons/fluent/shield-error-20-regular'
import People from '~icons/fluent/people-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import CountryFlag from '@/components/CountryFlag.vue'
import EmptyState from '@/components/EmptyState.vue'
import AxDonut from '@/components/AxDonut.vue'
import AxSankey, { type SankeyLink, type SankeyNode } from '@/components/AxSankey.vue'
import { get, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import { chartColors, cssVar } from '@/theme/theme'
import { useThemeStore } from '@/stores/theme'
import { countryName, hostOf, isPrivateIp, subnet24 } from '@/utils/net'

type Result = 'connected' | 'auth_ok' | 'auth_fail'

interface FlowEvent {
  id: number
  time: string
  user: string
  nickname: string
  group: string
  src: string
  srcIp: string
  region: string
  // CountryFlag 用：ISO 编码 / 'LAN' / ''（未知）
  regionCode: string
  entry: string
  channel: string
  client: string
  os: string
  version: string
  device: string
  result: Result
  reason: string
  assigned: string
}

const RESULT_META: Record<Result, { label: string; cls: string; color: 'success' | 'info' | 'error' }> = {
  connected: { label: '连接成功', cls: 'text-success', color: 'success' },
  auth_ok: { label: '认证通过', cls: 'text-info', color: 'info' },
  auth_fail: { label: '认证失败', cls: 'text-error', color: 'error' },
}
const STATUS_TO_RESULT: Record<number, Result> = { 0: 'auth_fail', 1: 'auth_ok', 2: 'connected' }

const ranges = [
  { label: '最近 1 小时', value: 1 },
  { label: '最近 1 天', value: 24 },
  { label: '最近 1 周', value: 168 },
  { label: '最近 30 天', value: 720 },
]

const theme = useThemeStore()
const hours = ref(24)
const loading = ref(false)
const events = ref<FlowEvent[]>([])
const truncated = ref(false)
const hostname = ref('')

// 筛选
const keyword = ref('')
const fResult = ref<Result | ''>('')
const fRegion = ref('')
const fEntry = ref('')
const fNet = ref('')
const page = ref(1)
const PAGE_SIZE = 30

// 原始日志 → 链路事件。来源国家依赖后端 GeoIP（country_code 字段），缺失时区分内网/未知
function toEvent(d: any, osOps: string[], clientOps: string[]): FlowEvent {
  const srcIp = hostOf(d.remote_addr)
  const code = (d.country_code || '').toUpperCase()
  const lan = isPrivateIp(srcIp)
  const region = code ? countryName(code) : lan ? '内网' : '未知地区'
  const channel = d.client === 4 ? 'Web 门户' : 'SSL VPN'
  return {
    id: d.id,
    time: d.created_at,
    user: d.username || '(未提供用户名)',
    nickname: d.nickname || '',
    group: d.group_name || '',
    src: d.remote_addr || '',
    srcIp,
    region,
    regionCode: code || (lan ? 'LAN' : ''),
    entry: `${hostname.value || '本节点'} · ${channel}`,
    channel,
    client: clientOps[d.client] || 'Unknown',
    os: osOps[d.os] || 'Unknown',
    version: d.version || '',
    device: [d.device_type, d.platform_version].filter(Boolean).join(' '),
    result: STATUS_TO_RESULT[d.status] ?? 'auth_fail',
    reason: d.info || '',
    assigned: d.status === 2 ? d.ip_addr || '' : '',
  }
}

async function load() {
  loading.value = true
  try {
    if (!hostname.value) {
      const sys = await get('/api/v1/set/system').catch(() => null)
      hostname.value = sys?.sys?.hostname || ''
    }
    const data = await get('/api/v1/set/audit/access_flow', { hours: hours.value, limit: 2000 })
    events.value = (data?.datas || []).map((d: any) => toEvent(d, data.osOps || [], data.clientOps || []))
    truncated.value = !!data?.truncated
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

function changeRange(v: number) {
  hours.value = v
  load()
}

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return events.value.filter(e =>
    (!fResult.value || e.result === fResult.value) &&
    (!fRegion.value || e.region === fRegion.value) &&
    (!fEntry.value || e.entry === fEntry.value) &&
    (!fNet.value || subnet24(e.assigned) === fNet.value) &&
    (!k || [e.user, e.nickname, e.srcIp, e.assigned, e.group].some(s => s.toLowerCase().includes(k))))
})
watch([keyword, fResult, fRegion, fEntry, fNet], () => { page.value = 1 })
const pageRows = computed(() => filtered.value.slice((page.value - 1) * PAGE_SIZE, page.value * PAGE_SIZE))

// —— 概览 ——
const summary = computed(() => {
  const all = filtered.value
  const ok = all.filter(e => e.result === 'connected').length
  const fails = all.filter(e => e.result === 'auth_fail')
  const decided = ok + fails.length
  return [
    { label: '接入尝试', icon: Plug, value: all.length, hint: `${hours.value >= 24 ? hours.value / 24 + ' 天' : hours.value + ' 小时'}内全部记录`, tone: undefined },
    { label: '连接成功', icon: CheckCircle, value: ok, hint: decided ? `成功率 ${((ok / decided) * 100).toFixed(1)}%` : '暂无结果', tone: ok ? 'success' as const : undefined },
    { label: '认证失败', icon: ShieldError, value: fails.length, hint: fails.length ? `来自 ${new Set(fails.map(e => e.srcIp)).size} 个来源 IP` : '没有失败记录', tone: fails.length ? 'error' as const : undefined },
    { label: '独立用户', icon: People, value: new Set(all.map(e => e.user)).size, hint: `分布于 ${new Set(all.map(e => e.region)).size} 个来源地区`, tone: undefined },
  ]
})

const failReasons = computed(() => {
  const m = new Map<string, number>()
  for (const e of filtered.value) if (e.result === 'auth_fail') m.set(e.reason || '未知原因', (m.get(e.reason || '未知原因') || 0) + 1)
  const sorted = [...m.entries()].sort((a, b) => b[1] - a[1])
  const top = sorted.slice(0, 5).map(([name, value]) => ({ name, value }))
  const rest = sorted.slice(5).reduce((s, [, v]) => s + v, 0)
  return rest ? [...top, { name: '其他', value: rest }] : top
})

// —— 链路图：来源地区 → 接入入口 → 结果 → 分配网段 ——
function topN(values: string[], n: number) {
  const m = new Map<string, number>()
  for (const v of values) m.set(v, (m.get(v) || 0) + 1)
  return new Set([...m.entries()].sort((a, b) => b[1] - a[1]).slice(0, n).map(([k]) => k))
}

const flow = computed(() => {
  const evs = filtered.value
  const c = chartColors[theme.isDark ? 'dark' : 'light']
  const neutral = cssVar('--color-text-tertiary') || c.default
  const topRegions = topN(evs.map(e => e.region), 8)
  const topNets = topN(evs.filter(e => e.assigned).map(e => subnet24(e.assigned)), 6)
  const nodes = new Map<string, SankeyNode>()
  const links = new Map<string, SankeyLink>()
  const node = (name: string, label: string, depth: number, color: string) => {
    if (!nodes.has(name)) nodes.set(name, { name, label, depth, color })
    return name
  }
  const link = (s: string, t: string) => {
    const k = `${s}→${t}`
    const l = links.get(k)
    if (l) l.value++
    else links.set(k, { source: s, target: t, value: 1 })
  }
  for (const e of evs) {
    const region = topRegions.has(e.region) ? e.region : '其他地区'
    const src = node(`src:${region}`, region, 0, c.palette[0])
    const entry = node(`entry:${e.entry}`, e.entry, 1, neutral)
    const meta = RESULT_META[e.result]
    const res = node(`res:${e.result}`, meta.label, 2, c[meta.color])
    link(src, entry)
    link(entry, res)
    if (e.assigned) {
      const net = topNets.has(subnet24(e.assigned)) ? subnet24(e.assigned) : '其他网段'
      link(res, node(`net:${net}`, net, 3, c.palette[1]))
    }
  }
  return { nodes: [...nodes.values()], links: [...links.values()] }
})

// 点击链路图节点 = 按该节点筛选
function onSelectNode(name: string) {
  const [kind, ...rest] = name.split(':')
  const v = rest.join(':')
  if (kind === 'src') fRegion.value = v === '其他地区' ? '' : v
  else if (kind === 'entry') fEntry.value = v
  else if (kind === 'res') fResult.value = v as Result
  else if (kind === 'net') fNet.value = v === '其他网段' ? '' : v
}

const resultOptions = [{ label: '全部结果', value: '' }, ...Object.entries(RESULT_META).map(([value, m]) => ({ label: m.label, value }))]
// 地区下拉：带国旗，按出现次数排序
const regionOptions = computed(() => {
  const m = new Map<string, { code: string; n: number }>()
  for (const e of events.value) {
    const x = m.get(e.region)
    if (x) x.n++
    else m.set(e.region, { code: e.regionCode, n: 1 })
  }
  return [
    { label: '全部地区', value: '', code: undefined },
    ...[...m.entries()].sort((a, b) => b[1].n - a[1].n).map(([r, x]) => ({ label: r, value: r, code: x.code })),
  ]
})
const renderRegion = (o: SelectOption) => (o.value
  ? h('span', { class: 'inline-flex items-center gap-6px' }, [h(CountryFlag, { code: o.code as string }), o.label as string])
  : (o.label as string))

const activeChips = computed(() => [
  fRegion.value && { k: 'region', label: `来源：${fRegion.value}` },
  fEntry.value && { k: 'entry', label: `入口：${fEntry.value}` },
  fNet.value && { k: 'net', label: `网段：${fNet.value}` },
].filter(Boolean) as { k: string; label: string }[])
function clearChip(k: string) {
  if (k === 'region') fRegion.value = ''
  if (k === 'entry') fEntry.value = ''
  if (k === 'net') fNet.value = ''
}

// —— 明细表：用户 → 来源 → 入口 → 结果 → 分配 IP ——
const arrowCol = (key: string) => ({
  key, title: '', width: 28, align: 'center' as const,
  render: () => h(NIcon, { size: 14, class: 'text-quaternary' }, { default: () => h(ArrowRight) }),
})
const sub = (text: string) => h('div', { class: 'truncate text-sm text-secondary' }, text)

const columns: DataTableColumns<FlowEvent> = [
  {
    title: '时间', key: 'time', width: 110,
    render: e => h('div', [h('div', { class: 'tabular-nums' }, dayjs(e.time).format('HH:mm:ss')), sub(dayjs(e.time).format('MM-DD'))]),
  },
  {
    title: '用户', key: 'user', minWidth: 150,
    render: e => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'truncate font-semibold' }, [e.user, e.nickname ? h('span', { class: 'font-normal text-secondary' }, `（${e.nickname}）`) : null]),
      sub(e.group || '未关联用户组'),
    ]),
  },
  arrowCol('a1'),
  {
    title: '来源', key: 'src', minWidth: 180,
    render: e => h('div', { class: 'flex min-w-0 items-center gap-8px' }, [
      h(CountryFlag, { code: e.regionCode }),
      h('div', { class: 'min-w-0' }, [h('div', { class: 'rl-mono truncate' }, e.srcIp || '—'), sub(e.region)]),
    ]),
  },
  arrowCol('a2'),
  {
    title: '接入入口', key: 'entry', minWidth: 170,
    render: e => h('div', { class: 'min-w-0' }, [h('div', { class: 'truncate' }, e.entry), sub(`${e.client} · ${e.os}`)]),
  },
  arrowCol('a3'),
  {
    title: '结果', key: 'result', minWidth: 160,
    render: e => h('div', { class: 'min-w-0' }, [
      h(StatusDot, { tone: RESULT_META[e.result].color }, { default: () => RESULT_META[e.result].label }),
      e.reason
        ? h(NTooltip, { disabled: e.reason.length < 14 }, { trigger: () => sub(e.reason), default: () => e.reason })
        : null,
    ]),
  },
  arrowCol('a4'),
  {
    title: '分配 IP', key: 'assigned', width: 140,
    render: e => (e.assigned ? h('span', { class: 'rl-mono' }, e.assigned) : h('span', { class: 'text-tertiary' }, '—')),
  },
]
const rowProps = (e: FlowEvent) => ({ class: 'cursor-pointer', onClick: () => { detail.value = e } })

// —— 详情抽屉 ——
const detail = ref<FlowEvent | null>(null)
const steps = computed<{ title: string; main: string; sub?: string; cls?: string; flag?: string }[]>(() => {
  const d = detail.value
  if (!d) return []
  return [
    { title: '用户', main: d.user + (d.nickname ? `（${d.nickname}）` : ''), sub: d.group ? `用户组：${d.group}` : '未关联用户组' },
    { title: '来源', main: d.src || '—', sub: d.region, flag: d.regionCode },
    { title: '接入入口', main: d.entry, sub: [d.client, d.version, d.os, d.device].filter(Boolean).join(' · ') },
    { title: '结果', main: RESULT_META[d.result].label, sub: d.reason, cls: RESULT_META[d.result].cls },
    { title: '分配地址', main: d.assigned || '未分配', sub: d.assigned ? `网段 ${subnet24(d.assigned)}` : d.result === 'auth_fail' ? '认证未通过，未建立隧道' : '' },
  ]
})

onMounted(load)
</script>

<template>
  <div>
    <PageHeader title="接入链路" description="追踪每次接入从用户、来源、入口到地址分配的完整路径">
      <NSelect :value="hours" :options="ranges" class="w-140px!" @update:value="changeRange" />
      <button class="rl-icon-btn" title="刷新" :disabled="loading" @click="load">
        <ArrowClockwise :class="loading && 'animate-spin'" />
      </button>
    </PageHeader>

    <div class="grid gap-16px">
      <!-- 概览：2×2 指标卡 + 失败原因 -->
      <div class="grid gap-16px xl:grid-cols-3">
        <div class="grid gap-16px sm:grid-cols-2 xl:col-span-2">
          <StatCard v-for="s in summary" :key="s.label" :label="s.label" :icon="s.icon" :value="s.value" :hint="s.hint" :tone="s.tone" />
        </div>
        <RlPanel title="失败原因" subtitle="认证失败按原因归类" :padded="false">
          <div class="py-16px"><AxDonut :data="failReasons" center-label="失败" empty-text="没有认证失败" /></div>
        </RlPanel>
      </div>

      <!-- 链路图 -->
      <RlPanel title="链路分布" subtitle="来源地区 → 接入入口 → 认证结果 → 分配网段，点击节点筛选明细">
        <div v-if="flow.links.length" class="h-360px"><AxSankey :nodes="flow.nodes" :links="flow.links" unit="次" @select="onSelectNode" /></div>
        <EmptyState v-else title="没有接入记录" description="所选时间范围内没有接入记录，可尝试扩大时间范围" />
      </RlPanel>

      <!-- 明细 -->
      <RlPanel :padded="false">
        <template #toolbar>
          <NInput v-model:value="keyword" clearable placeholder="用户 / 来源 IP / 分配 IP" class="w-240px!">
            <template #prefix><NIcon class="text-secondary"><Search /></NIcon></template>
          </NInput>
          <NSelect v-model:value="fResult" :options="resultOptions" class="w-130px!" />
          <NSelect v-model:value="fRegion" :options="regionOptions" :render-label="renderRegion" filterable class="w-170px!" />
          <span v-for="c in activeChips" :key="c.k" class="ax-badge cursor-pointer gap-4px text-primary" @click="clearChip(c.k)">
            {{ c.label }}<NIcon :size="12"><Dismiss /></NIcon>
          </span>
          <div class="flex-1" />
          <span v-if="truncated" class="text-sm text-warning">仅统计最近 2000 条，可缩小时间范围</span>
          <span class="text-sm text-secondary">共 {{ filtered.length }} 条</span>
        </template>
        <NDataTable
          :columns="columns" :data="pageRows" :loading="loading" :bordered="false" size="small"
          :row-key="(e: FlowEvent) => e.id" :row-props="rowProps" :scroll-x="1100"
        >
          <template #empty><EmptyState title="没有匹配的接入记录" description="调整筛选条件或时间范围后重试" /></template>
        </NDataTable>
        <template v-if="filtered.length > PAGE_SIZE" #footer>
          <span class="text-sm text-secondary">点击行查看单次接入详情</span>
          <NPagination v-model:page="page" :page-size="PAGE_SIZE" :item-count="filtered.length" />
        </template>
      </RlPanel>
    </div>

    <!-- 单次接入详情：竖向链路 -->
    <NDrawer :show="!!detail" :width="440" @update:show="v => !v && (detail = null)">
      <NDrawerContent v-if="detail" :title="`${detail.user} 的接入详情`" closable>
        <div class="text-sm text-secondary">{{ dayjs(detail.time).format('YYYY-MM-DD HH:mm:ss') }}</div>
        <div class="mt-md flex flex-col">
          <div v-for="(step, i) in steps" :key="step.title" class="flex gap-md">
            <div class="flex w-14px flex-col items-center">
              <span class="mt-6px h-10px w-10px flex-shrink-0 rounded-full ring-2 ring-[var(--color-border-strong)]" :class="i === 3 ? 'bg-[currentColor] ' + step.cls : 'bg-primary'" />
              <span v-if="i < 4" class="w-1px flex-1 bg-[var(--color-border-default)]" />
            </div>
            <div class="min-w-0 pb-md">
              <div class="text-sm font-bold text-secondary">{{ step.title }}</div>
              <div class="mt-2px flex items-center gap-8px text-lg" :class="step.cls">
                <CountryFlag v-if="step.flag !== undefined" :code="step.flag" />
                <span class="truncate" :class="step.flag !== undefined && 'rl-mono'">{{ step.main }}</span>
              </div>
              <div v-if="step.sub" class="text-sm text-secondary">{{ step.sub }}</div>
            </div>
          </div>
        </div>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
