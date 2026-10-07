<script setup lang="ts">
// 用户行为：认证 / 连接 / 登出事件流（原「安全审计 - 用户活动日志」，接口 /api/v1/set/audit/act_log_list）
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { type DataTableColumns, NButton, NDataTable, NInput, NPagination } from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArrowSort from '~icons/fluent/arrow-sort-down-lines-20-regular'
import PersonClock from '~icons/fluent/person-clock-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import TimeRangePicker from './components/TimeRangePicker.vue'
import FilterChips from './components/FilterChips.vue'
import EventVolume from './components/EventVolume.vue'
import EventFields from './components/EventFields.vue'
import { type EventField, type FilterField, type FilterValues, type TimeSel, dayRange, isEmpty, resolveRange } from './components/shared'
import { dash, flagCode, ipCell, timeCell, twoLine, userLabel } from './components/cells'
import { errMsg, get } from '@/api/http'
import { ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'

interface ActLog {
  id: number
  username: string
  nickname?: string
  group_name: string
  ip_addr: string
  remote_addr: string
  os: number
  client: number
  version: string
  device_type: string
  platform_version: string
  status: number
  info: string
  created_at: string
  country_code?: string
}
interface StatusOp { key: number; value: string; tag: string }

const route = useRoute()
const rows = ref<ActLog[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(10)
const count = ref(0)
const sortDesc = ref(true)
const statusOps = ref<StatusOp[]>([])
const osOps = ref<string[]>([])
const clientOps = ref<string[]>([])

// 查询条件：用户名关键字 + 时间范围 + chip 筛选（后端筛选值 = 下标 + 1）
const kw = ref(typeof route.query.username === 'string' ? route.query.username : '')
const appliedKw = ref(kw.value)
const time = ref<TimeSel>({ preset: 'all', custom: null })
const filters = ref<FilterValues>({ status: null, os: null, client: null })

const fields = computed<FilterField[]>(() => [
  { key: 'status', label: '事件类型', type: 'select', options: statusOps.value.map(s => ({ label: s.value, value: s.key + 1 })) },
  { key: 'os', label: '操作系统', type: 'select', options: osOps.value.map((v, i) => ({ label: v, value: i + 1 })) },
  { key: 'client', label: '客户端', type: 'select', options: clientOps.value.map((v, i) => ({ label: v, value: i + 1 })) },
])

// 除时间与分页外的筛选参数（柱状图复用）
const baseParams = () => ({
  username: appliedKw.value || '',
  status: filters.value.status ?? '',
  os: filters.value.os ?? '',
  client: filters.value.client ?? '',
  sort: sortDesc.value ? 1 : 2,
})
const range = computed(() => resolveRange(time.value, 'date'))

async function load(p = page.value) {
  loading.value = true
  try {
    const r = resolveRange(time.value, 'date')
    const d = await get('/api/v1/set/audit/act_log_list', { ...baseParams(), page: p, sdate: r?.[0] || '', edate: r?.[1] || '' })
    rows.value = d?.datas || []
    count.value = d?.count || 0
    pageSize.value = d?.page_size || 10
    statusOps.value = d?.statusOps || []
    osOps.value = d?.osOps || []
    clientOps.value = d?.clientOps || []
    page.value = p
    expanded.value = []
  } catch (e) {
    ui.message.error(errMsg(e))
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
  filters.value = { status: null, os: null, client: null }
  load(1)
}
const dirty = computed(() => !!appliedKw.value || time.value.preset !== 'all' || Object.values(filters.value).some(v => !isEmpty(v)))
watch(time, () => load(1))
function toggleSort() {
  sortDesc.value = !sortDesc.value
  load(1)
}
function setFilter(key: string, v: string | number) {
  filters.value = { ...filters.value, [key]: v }
  load(1)
}
function filterUser(u: string) {
  kw.value = u
  search()
}

// 柱状图：逐日 count
const volKey = computed(() => JSON.stringify([appliedKw.value, filters.value]))
const volFetch = async (end: string, days: number) => {
  const d = await get('/api/v1/set/audit/act_log_daily', { ...baseParams(), end, days })
  return d?.days || []
}
const volEnd = computed(() => (time.value.preset === 'custom' ? range.value?.[1] : undefined))
const vol = ref<InstanceType<typeof EventVolume>>()
function refresh() {
  load()
  vol.value?.reload()
}
function pickDay(day: string) { time.value = { preset: 'custom', custom: dayRange(day, 'date') } }

const TONE: Record<string, 'error' | 'success' | 'info' | 'muted'> = { danger: 'error', success: 'success', '': 'info', info: 'muted' }
const statusOf = (r: ActLog) => statusOps.value.find(x => x.key === r.status)
const osOf = (r: ActLog) => osOps.value[r.os] ?? ''
const clientOf = (r: ActLog) => clientOps.value[r.client] ?? ''
const deviceOf = (r: ActLog) => (r.device_type ? `${r.device_type} / ${r.platform_version}` : '')

const expanded = ref<number[]>([])
function toggleRow(id: number, e: MouseEvent) {
  // 展开按钮自身已处理；选中文本时不切换
  if ((e.target as HTMLElement).closest('.n-data-table-expand-trigger') || window.getSelection()?.toString()) return
  expanded.value = expanded.value.includes(id) ? expanded.value.filter(k => k !== id) : [...expanded.value, id]
}

function detailFields(r: ActLog): EventField[] {
  const s = statusOf(r)
  return [
    { label: '事件类型', value: s?.value, onFilter: s ? () => setFilter('status', s.key + 1) : undefined },
    { label: '用户', value: r.username, mono: true, onFilter: () => filterUser(r.username) },
    { label: '姓名', value: r.nickname },
    { label: '用户组', value: r.group_name },
    { label: '来源 IP', value: r.remote_addr, mono: true, flag: flagCode(r.remote_addr, r.country_code) },
    { label: '内网 IP', value: r.ip_addr, mono: true },
    { label: '操作系统', value: osOf(r), onFilter: () => setFilter('os', r.os + 1) },
    { label: '设备型号', value: deviceOf(r) },
    { label: '客户端', value: clientOf(r), onFilter: () => setFilter('client', r.client + 1) },
    { label: '客户端版本', value: r.version, mono: true },
    { label: '时间', value: formatDate(r.created_at), mono: true },
    { label: '事件 ID', value: r.id, mono: true },
    { label: '详情', value: r.info, wide: true },
  ]
}

const columns = computed<DataTableColumns<ActLog>>(() => [
  { type: 'expand', width: 36, renderExpand: r => h(EventFields, { fields: detailFields(r), raw: r }) },
  { title: '时间', key: 'created_at', width: 150, render: r => timeCell(r.created_at) },
  {
    title: '事件', key: 'status', minWidth: 240,
    render: r => {
      const s = statusOf(r)
      return h('div', { class: 'min-w-0 leading-18px' }, [
        s ? h(StatusDot, { tone: TONE[s.tag] || 'muted', class: 'font-semibold' }, () => s.value) : dash(),
        h('div', { class: 'truncate pl-12px text-sm text-secondary', title: r.info }, r.info || '—'),
      ])
    },
  },
  { title: '用户', key: 'username', minWidth: 170, render: r => twoLine(userLabel(r.username, r.nickname) || '—', r.group_name) },
  {
    title: '来源', key: 'remote_addr', width: 220,
    render: r => h('div', { class: 'min-w-0 leading-18px' }, [
      ipCell(r.remote_addr, r.country_code),
      r.ip_addr ? h('div', { class: 'truncate pl-29px text-sm text-secondary' }, ['内网 ', h('span', { class: 'rl-mono' }, r.ip_addr)]) : null,
    ]),
  },
  {
    title: '终端', key: 'client', width: 200,
    render: r => twoLine(
      [clientOf(r) || '—', r.version ? h('span', { class: 'rl-mono ml-6px text-sm font-normal text-secondary' }, r.version) : null],
      [osOf(r), deviceOf(r)].filter(Boolean).join(' · '),
      'font-normal',
    ),
  },
])

onMounted(() => load(1))
</script>

<template>
  <div>
    <PageHeader title="用户行为" description="用户认证、连接、登出事件。展开一行可查看终端与来源的完整信息。">
      <button class="rl-icon-btn" title="刷新" @click="refresh"><ArrowClockwise /></button>
    </PageHeader>

    <RlPanel :padded="false">
      <template #toolbar>
        <TimeRangePicker v-model="time" mode="date" />
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
        :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: ActLog) => r.id"
        :row-props="(r: ActLog) => ({ class: 'cursor-pointer', onClick: (e: MouseEvent) => toggleRow(r.id, e) })"
        size="small" :scroll-x="1020" remote
      >
        <template #empty>
          <EmptyState :icon="PersonClock" title="没有匹配的事件" :description="dirty ? '试试放宽时间范围或移除部分筛选条件' : '用户认证或连接后，事件会出现在这里'" />
        </template>
      </NDataTable>

      <template #footer>
        <span class="text-sm text-secondary">共 {{ count.toLocaleString() }} 条事件</span>
        <NPagination :page="page" :page-size="pageSize" :item-count="count" @update:page="load" />
      </template>
    </RlPanel>
  </div>
</template>
