<script setup lang="ts">
// 管理操作：管理员在后台的配置变更事件流（原「安全审计 - 管理员操作日志」，接口 /api/v1/set/audit/admin_op_log_list）
import { computed, h, onMounted, ref, watch } from 'vue'
import { type DataTableColumns, NButton, NDataTable, NInput, NPagination } from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ShieldTask from '~icons/fluent/shield-task-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import EmptyState from '@/components/EmptyState.vue'
import TimeRangePicker from './components/TimeRangePicker.vue'
import FilterChips from './components/FilterChips.vue'
import EventVolume from './components/EventVolume.vue'
import EventFields from './components/EventFields.vue'
import { type EventField, type FilterField, type FilterValues, type TimeSel, dayRange, isEmpty, resolveRange } from './components/shared'
import { dash, flagCode, ipCell, timeCell } from './components/cells'
import { errMsg, get } from '@/api/http'
import { ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'

interface OpLog { id: number; admin_user: string; op_type: string; op_target: string; detail: string; client_ip: string; created_at: string; country_code?: string }

const rows = ref<OpLog[]>([])
const count = ref(0)
const page = ref(1)
const pageSize = ref(10)
const loading = ref(false)
const opTypes = ref<string[]>([])

const kw = ref('')
const appliedKw = ref('')
const time = ref<TimeSel>({ preset: 'all', custom: null })
const filters = ref<FilterValues>({ op_type: null })
const range = computed(() => resolveRange(time.value, 'date'))
const fields = computed<FilterField[]>(() => [
  { key: 'op_type', label: '操作类型', type: 'select', options: opTypes.value.map(t => ({ label: t, value: t })) },
])

// 后端固定按时间倒序；sort 参数与旧版保持一致照常传
const baseParams = () => ({ op_type: filters.value.op_type || '', keyword: appliedKw.value, sort: 1 })

async function load(p = page.value) {
  loading.value = true
  try {
    const r = range.value
    const d = await get('/api/v1/set/audit/admin_op_log_list', { ...baseParams(), page: p, sdate: r?.[0] || '', edate: r?.[1] || '' })
    rows.value = d?.datas || []
    count.value = d?.count || 0
    pageSize.value = d?.page_size || 10
    opTypes.value = d?.opTypes || []
    page.value = p
    expanded.value = []
  } catch (e) {
    ui.message.error(errMsg(e, '请求出错'))
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
  filters.value = { op_type: null }
  load(1)
}
const dirty = computed(() => !!appliedKw.value || time.value.preset !== 'all' || !isEmpty(filters.value.op_type))
watch(time, () => load(1))
function filterType(t: string) {
  filters.value = { op_type: t }
  load(1)
}
function filterKw(v: string) {
  kw.value = v
  search()
}

const volKey = computed(() => JSON.stringify([appliedKw.value, filters.value]))
const volFetch = async (end: string, days: number) => {
  const d = await get('/api/v1/set/audit/admin_op_log_daily', { ...baseParams(), end, days })
  return d?.days || []
}
const volEnd = computed(() => (time.value.preset === 'custom' ? range.value?.[1] : undefined))
const vol = ref<InstanceType<typeof EventVolume>>()
function refresh() {
  load()
  vol.value?.reload()
}
function pickDay(day: string) { time.value = { preset: 'custom', custom: dayRange(day, 'date') } }

const expanded = ref<number[]>([])
function toggleRow(id: number, e: MouseEvent) {
  if ((e.target as HTMLElement).closest('.n-data-table-expand-trigger, .n-data-table-th') || window.getSelection()?.toString()) return
  expanded.value = expanded.value.includes(id) ? expanded.value.filter(k => k !== id) : [...expanded.value, id]
}

function detailFields(r: OpLog): EventField[] {
  return [
    { label: '操作类型', value: r.op_type, onFilter: () => filterType(r.op_type) },
    { label: '操作目标', value: r.op_target, onFilter: r.op_target ? () => filterKw(r.op_target) : undefined },
    { label: '管理员', value: r.admin_user, mono: true },
    { label: '来源 IP', value: r.client_ip, mono: true, flag: flagCode(r.client_ip, r.country_code) },
    { label: '时间', value: formatDate(r.created_at), mono: true },
    { label: '事件 ID', value: r.id, mono: true },
    { label: '详情', value: r.detail, wide: true },
  ]
}

// 当前页内排序（与旧版一致：除 ID 外的列在本页数据内排序）
const by = (k: keyof OpLog) => (a: OpLog, b: OpLog) => String(a[k] ?? '').localeCompare(String(b[k] ?? ''))
const columns = computed<DataTableColumns<OpLog>>(() => [
  { type: 'expand', width: 36, renderExpand: r => h(EventFields, { fields: detailFields(r), raw: r }) },
  { title: '时间', key: 'created_at', width: 150, sorter: by('created_at'), render: r => timeCell(r.created_at) },
  {
    title: '操作', key: 'op_type', minWidth: 260, sorter: by('op_type'),
    render: r => h('div', { class: 'min-w-0 leading-18px' }, [
      h('div', { class: 'flex min-w-0 items-center gap-8px' }, [
        h('span', { class: 'ax-badge flex-shrink-0 text-info' }, r.op_type),
        r.op_target ? h('span', { class: 'truncate font-semibold', title: r.op_target }, r.op_target) : dash(),
      ]),
    ]),
  },
  { title: '详情', key: 'detail', minWidth: 280, sorter: by('detail'), render: r => (r.detail ? h('div', { class: 'line-clamp-2 text-secondary', title: r.detail }, r.detail) : dash()) },
  { title: '管理员', key: 'admin_user', width: 130, sorter: by('admin_user'), render: r => h('span', { class: 'font-semibold' }, r.admin_user || '—') },
  { title: '来源 IP', key: 'client_ip', width: 200, sorter: by('client_ip'), render: r => ipCell(r.client_ip, r.country_code) },
])

onMounted(() => load(1))
</script>

<template>
  <div>
    <PageHeader title="管理操作" description="管理员在后台进行的每一次配置变更：谁、在何时、改了什么。">
      <button class="rl-icon-btn" title="刷新" @click="refresh"><ArrowClockwise /></button>
    </PageHeader>

    <RlPanel :padded="false">
      <template #toolbar>
        <TimeRangePicker v-model="time" mode="date" />
        <NInput v-model:value="kw" placeholder="操作目标或详情，回车搜索" clearable class="w-240px!" @keyup.enter="search" @clear="kw = ''; search()">
          <template #prefix><Search class="text-secondary" /></template>
        </NInput>
        <FilterChips v-model="filters" :fields="fields" @change="load(1)" />
        <NButton v-if="dirty" text size="small" class="ml-4px" @click="resetAll">清除全部</NButton>
        <div class="flex-1" />
        <span class="text-sm text-tertiary">最新在前</span>
      </template>

      <EventVolume ref="vol" :fetcher="volFetch" :dep-key="volKey" :end="volEnd" :selected="range" @pick="pickDay" />

      <NDataTable
        v-model:expanded-row-keys="expanded"
        :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: OpLog) => r.id"
        :row-props="(r: OpLog) => ({ class: 'cursor-pointer', onClick: (e: MouseEvent) => toggleRow(r.id, e) })"
        size="small" :scroll-x="1050"
      >
        <template #empty>
          <EmptyState :icon="ShieldTask" title="没有匹配的操作记录" :description="dirty ? '试试放宽时间范围或移除部分筛选条件' : '管理员在后台修改配置后，记录会出现在这里'" />
        </template>
      </NDataTable>

      <template #footer>
        <span class="text-sm text-secondary">共 {{ count.toLocaleString() }} 条操作</span>
        <NPagination :page="page" :page-size="pageSize" :item-count="count" @update:page="p => load(p)" />
      </template>
    </RlPanel>
  </div>
</template>
