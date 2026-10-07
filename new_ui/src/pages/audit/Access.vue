<script setup lang="ts">
// 访问日志：用户经隧道访问内网资源的审计事件流（原「安全审计 - 用户访问日志」）
// 接口：/set/audit/list、/api/v1/set/audit/export，参数以 search[...] 嵌套提交（与旧版一致）
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { type DataTableColumns, NButton, NDataTable, NInput, NPagination, NSelect, NSwitch } from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import ArrowSort from '~icons/fluent/arrow-sort-down-lines-20-regular'
import ArrowRight from '~icons/fluent/arrow-right-12-regular'
import Globe from '~icons/fluent/globe-search-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import TimeRangePicker from './components/TimeRangePicker.vue'
import FilterChips from './components/FilterChips.vue'
import EventVolume from './components/EventVolume.vue'
import EventFields from './components/EventFields.vue'
import { type EventField, type FilterField, type FilterValues, type TimeSel, dayRange, downloadText, isEmpty, resolveRange } from './components/shared'
import { flagCode, ipCell, timeCell, twoLine, userLabel } from './components/cells'
import { errMsg, get } from '@/api/http'
import { ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'

interface AccessRow {
  id: number
  username: string
  nickname?: string
  group_name: string
  src: string
  src_port: number
  dst: string
  dst_port: number
  protocol: number
  access_proto: number
  info: string
  created_at: string
  country_code?: string
}

const PROTO = ['', 'UDP', 'TCP', 'HTTPS', 'HTTP', 'DNS', 'SSH', 'FTP', 'SMTP', 'IMAP', 'POP3']
const MAX_EXPORT = 1000000

// IP 校验：v4 / v6（与旧版规则一致）
const isIPv4 = (v: string) => /^((\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.){3}(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])$/.test(v)
function isIPv6(v: string) {
  if (!v.includes(':') || !/^[0-9a-fA-F:]+$/.test(v) || (v.match(/::/g) || []).length > 1) return false
  const parts = v.split(':')
  const nonEmpty = parts.filter(Boolean)
  if (nonEmpty.some(p => p.length > 4)) return false
  return v.includes('::') ? nonEmpty.length < 8 : parts.length === 8 && nonEmpty.length === 8
}
const ipRule = (v: string) => (isIPv4(v) || isIPv6(v) ? '' : '请输入正确的 IP 地址')
const portRule = (v: string) => (/^\d{1,5}$/.test(v) && +v <= 65535 ? '' : '端口范围 0–65535')

const fields: FilterField[] = [
  { key: 'group_name', label: '用户组', type: 'text', placeholder: '用户组名称（精确匹配）' },
  { key: 'src', label: '源 IP', type: 'text', mono: true, placeholder: '如 10.0.0.12', validate: ipRule },
  { key: 'src_port', label: '源端口', type: 'text', mono: true, placeholder: '如 51820', validate: portRule },
  { key: 'dst', label: '目的 IP', type: 'text', mono: true, placeholder: '如 192.168.1.10', validate: ipRule },
  { key: 'dst_port', label: '目的端口', type: 'text', mono: true, placeholder: '如 443', validate: portRule },
  { key: 'access_proto', label: '协议', type: 'select', options: PROTO.slice(1).map((t, i) => ({ label: t, value: String(i + 1) })) },
  { key: 'info', label: '访问目标', type: 'text', placeholder: '域名或详情关键字（模糊匹配）' },
]
const blankFilters = (): FilterValues => Object.fromEntries(fields.map(f => [f.key, null]))

const route = useRoute()
const rows = ref<AccessRow[]>([])
const count = ref(0)
const page = ref(1)
const pageSize = ref(10)
const loading = ref(false)
const exporting = ref(false)
const sortDesc = ref(true)
const kw = ref(typeof route.query.username === 'string' ? route.query.username : '')
const appliedKw = ref(kw.value)
const time = ref<TimeSel>({ preset: 'all', custom: null })
const filters = ref<FilterValues>(blankFilters())
const range = computed(() => resolveRange(time.value, 'datetime'))

const str = (v: unknown) => (isEmpty(v) ? '' : String(v).trim())
// search[...] 参数；date 为空时传 ['', '']（后端据此跳过时间条件）
const searchParams = (date?: [string, string] | null) => ({
  username: appliedKw.value,
  group_name: str(filters.value.group_name),
  src: str(filters.value.src),
  dst: str(filters.value.dst),
  src_port: str(filters.value.src_port),
  dst_port: str(filters.value.dst_port),
  access_proto: str(filters.value.access_proto),
  info: str(filters.value.info),
  date: date === undefined ? resolveRange(time.value, 'datetime') || ['', ''] : date || ['', ''],
  sort: sortDesc.value ? 1 : 2,
})

async function load(p = page.value, silent = false) {
  if (!silent) loading.value = true
  try {
    const d = await get('/api/v1/set/audit/list', { page: p, search: searchParams() })
    rows.value = d?.datas || []
    count.value = d?.count || 0
    pageSize.value = d?.page_size || 10
    if (p !== page.value || !silent) expanded.value = []
    page.value = p
  } catch (e) {
    if (!silent) ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
function search() {
  appliedKw.value = kw.value.trim()
  load(1)
}
function resetAll() {
  kw.value = ''
  appliedKw.value = ''
  time.value = { preset: 'all', custom: null }
  filters.value = blankFilters()
  load(1)
}
const dirty = computed(() => !!appliedKw.value || time.value.preset !== 'all' || Object.values(filters.value).some(v => !isEmpty(v)))
watch(time, () => load(1))
function toggleSort() {
  sortDesc.value = !sortDesc.value
  load(1)
}
function setFilter(key: string, v: string | number) {
  if (isEmpty(v)) return
  filters.value = { ...filters.value, [key]: String(v) }
  load(1)
}
function filterUser(u: string) {
  kw.value = u
  search()
}

// 自动刷新（实时）：静默拉取当前页
const auto = ref(false)
const interval = ref(10)
let timer: ReturnType<typeof setInterval> | null = null
function applyAuto() {
  if (timer) clearInterval(timer)
  timer = null
  if (!auto.value) return
  load(page.value, true)
  timer = setInterval(() => load(page.value, true), interval.value * 1000)
}
watch([auto, interval], applyAuto)
onBeforeUnmount(() => { if (timer) clearInterval(timer) })

async function exportCsv() {
  if (count.value > MAX_EXPORT) return ui.message.error('导出的数据量超过 1,000,000 条，请调整搜索条件后再导出')
  exporting.value = true
  try {
    // 成功返回 CSV 文本；失败返回 {code,msg}，由 request 抛出 ApiError
    const csv = await get<string>('/api/v1/set/audit/export', { search: searchParams() })
    downloadText('﻿' + csv, `remlink_audit_log_${Date.now()}.csv`, 'text/csv;charset=utf-8')
    ui.message.success('成功导出 CSV 文件')
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    exporting.value = false
  }
}

// 柱状图：逐日 count（自动刷新时不重算）
const volKey = computed(() => JSON.stringify([appliedKw.value, filters.value]))
const volFetch = async (end: string, days: number) => {
  const d = await get('/api/v1/set/audit/daily', { search: searchParams(null), end, days })
  return d?.days || []
}
const volEnd = computed(() => (time.value.preset === 'custom' ? range.value?.[1]?.slice(0, 10) : undefined))
const vol = ref<InstanceType<typeof EventVolume>>()
function refresh() {
  load()
  vol.value?.reload()
}
function pickDay(day: string) { time.value = { preset: 'custom', custom: dayRange(day, 'datetime') } }

const protoOf = (r: AccessRow) => {
  let p = r.access_proto
  if (p === 0) p = r.protocol === 6 ? 2 : r.protocol === 17 ? 1 : 0
  return PROTO[p] || ''
}
const ipProto = (n: number) => (n === 6 ? 'TCP' : n === 17 ? 'UDP' : String(n))
const endpoint = (addr: string, port: number) => {
  if (!addr) return port ? `:${port}` : ''
  const host = addr.includes(':') ? `[${addr}]` : addr
  return port ? `${host}:${port}` : host
}
// 访问目标：域名优先，无域名回退 IP:端口
const targetOf = (r: AccessRow) => r.info || endpoint(r.dst, r.dst_port)

const expanded = ref<number[]>([])
function toggleRow(id: number, e: MouseEvent) {
  if ((e.target as HTMLElement).closest('.n-data-table-expand-trigger') || window.getSelection()?.toString()) return
  expanded.value = expanded.value.includes(id) ? expanded.value.filter(k => k !== id) : [...expanded.value, id]
}

function detailFields(r: AccessRow): EventField[] {
  const ap = r.access_proto ? String(r.access_proto) : ''
  return [
    { label: '用户', value: r.username, mono: true, onFilter: () => filterUser(r.username) },
    { label: '姓名', value: r.nickname },
    { label: '用户组', value: r.group_name, onFilter: () => setFilter('group_name', r.group_name) },
    { label: '源 IP', value: r.src, mono: true, flag: flagCode(r.src, r.country_code), onFilter: () => setFilter('src', r.src) },
    { label: '源端口', value: r.src_port || '', mono: true, onFilter: () => setFilter('src_port', r.src_port) },
    { label: '目的 IP', value: r.dst, mono: true, onFilter: () => setFilter('dst', r.dst) },
    { label: '目的端口', value: r.dst_port || '', mono: true, onFilter: () => setFilter('dst_port', r.dst_port) },
    { label: 'IP 协议', value: ipProto(r.protocol) },
    { label: '访问协议', value: protoOf(r), onFilter: ap ? () => setFilter('access_proto', ap) : undefined },
    { label: '时间', value: formatDate(r.created_at), mono: true },
    { label: '事件 ID', value: r.id, mono: true },
    { label: '访问目标', value: targetOf(r), wide: true, mono: !r.info, onFilter: r.info ? () => setFilter('info', r.info) : undefined },
  ]
}

const columns = computed<DataTableColumns<AccessRow>>(() => [
  { type: 'expand', width: 36, renderExpand: r => h(EventFields, { fields: detailFields(r), raw: r }) },
  { title: '时间', key: 'created_at', width: 150, render: r => timeCell(r.created_at) },
  {
    title: '访问目标', key: 'info', minWidth: 300,
    render: r => h('div', { class: 'flex min-w-0 items-center gap-8px' }, [
      h('span', { class: 'ax-badge w-58px flex-shrink-0 justify-center text-info' }, protoOf(r) || '—'),
      h('span', { class: ['truncate font-semibold', !r.info && 'rl-mono'], title: targetOf(r) }, targetOf(r)),
    ]),
  },
  { title: '用户', key: 'username', minWidth: 170, render: r => twoLine(userLabel(r.username, r.nickname) || '—', r.group_name) },
  {
    title: '连接', key: 'src', width: 380,
    render: r => h('div', { class: 'flex min-w-0 items-center gap-8px' }, [
      h('div', { class: 'min-w-0 max-w-170px' }, [ipCell(endpoint(r.src, r.src_port), r.country_code)]),
      h(ArrowRight, { class: 'flex-shrink-0 text-tertiary' }),
      h('span', { class: 'rl-mono truncate text-secondary', title: endpoint(r.dst, r.dst_port) }, endpoint(r.dst, r.dst_port) || '—'),
    ]),
  },
])

onMounted(() => load(1))
</script>

<template>
  <div>
    <PageHeader title="访问日志" description="用户经隧道访问内网资源的记录：谁、从哪、访问了什么。">
      <template #meta>
        <StatusDot v-if="auto" tone="success" pulse>实时 · 每 {{ interval }} 秒</StatusDot>
      </template>
      <div class="rl-live">
        <NSwitch v-model:value="auto" size="small" />
        <span class="text-sm">自动刷新</span>
        <NSelect
          v-if="auto" v-model:value="interval" size="tiny" class="w-74px!" :consistent-menu-width="false"
          :options="[{ label: '5 秒', value: 5 }, { label: '10 秒', value: 10 }, { label: '30 秒', value: 30 }]"
        />
      </div>
      <button class="rl-icon-btn" title="刷新" @click="refresh"><ArrowClockwise /></button>
      <NButton :loading="exporting" @click="exportCsv">
        <template #icon><ArrowDownload /></template>
        导出 CSV
      </NButton>
    </PageHeader>

    <RlPanel :padded="false">
      <template #toolbar>
        <TimeRangePicker v-model="time" mode="datetime" />
        <NInput v-model:value="kw" placeholder="用户名或姓名，回车搜索" clearable :maxlength="30" class="w-220px!" @keyup.enter="search" @clear="kw = ''; search()">
          <template #prefix><Search class="text-secondary" /></template>
        </NInput>
        <FilterChips v-model="filters" :fields="fields" @change="load(1)" />
        <NButton v-if="dirty" text size="small" class="ml-4px" @click="resetAll">清除全部</NButton>
        <div class="flex-1" />
        <NButton quaternary size="small" @click="toggleSort">
          <template #icon><ArrowSort :class="!sortDesc && 'rotate-180'" /></template>
          {{ sortDesc ? '最新在前' : '最早在前' }}
        </NButton>
      </template>

      <EventVolume ref="vol" :fetcher="volFetch" :dep-key="volKey" :end="volEnd" :selected="range" @pick="pickDay" />

      <NDataTable
        v-model:expanded-row-keys="expanded"
        :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: AccessRow) => r.id"
        :row-props="(r: AccessRow) => ({ class: 'cursor-pointer', onClick: (e: MouseEvent) => toggleRow(r.id, e) })"
        size="small" :scroll-x="1100" remote
      >
        <template #empty>
          <EmptyState :icon="Globe" title="没有匹配的访问记录" :description="dirty ? '试试放宽时间范围或移除部分筛选条件' : '用户经隧道访问内网资源后记录会出现在这里（由软件配置 audit_interval 控制，-1 为关闭）'" />
        </template>
      </NDataTable>

      <template #footer>
        <span class="text-sm text-secondary">共 {{ count.toLocaleString() }} 条记录</span>
        <NPagination :page="page" :page-size="pageSize" :item-count="count" @update:page="p => load(p)" />
      </template>
    </RlPanel>
  </div>
</template>

<style scoped>
.rl-live {
  display: inline-flex; align-items: center; gap: 8px; height: 36px; padding: 0 12px;
  border: 1px solid var(--color-border-default); border-radius: 8px;
}
</style>
