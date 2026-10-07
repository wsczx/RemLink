<script setup lang="ts">
// WebVPN 访问：远程会话（SSH/SFTP/SMB/FTP/RDP/VNC）与 HTTP 应用访问（请求按 30 分钟间隔聚合）统一为「访问记录」
// 列表只放关键信息，点击行在右侧抽屉查看回放、文件操作与请求明细
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  NButton, NCheckbox, NDataTable, NDropdown, NInput, NPagination, NPopover, NSelect, NSwitch, NTooltip, type DataTableColumns,
} from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import Filter from '~icons/fluent/filter-20-regular'
import More from '~icons/fluent/more-horizontal-20-regular'
import VideoClip from '~icons/fluent/video-clip-20-regular'
import VideoOff from '~icons/fluent/video-off-20-regular'
import DocumentSearch from '~icons/fluent/document-search-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import { get, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'
import TimeRangePicker from '@/pages/audit/components/TimeRangePicker.vue'
import { type TimeSel, relTime, resolveRange } from '@/pages/audit/components/shared'
import { ipCell } from '@/pages/audit/components/cells'
import VisitDrawer from './components/VisitDrawer.vue'
import {
  type Visit, PROTO_LABEL, REC_SHORT, downloadBlob, fmtDuration, hasScreen, protoIcon, protocolOptions, recReasonText,
} from './components/visit'

const rows = ref<Visit[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const truncated = ref(false)
const scanLimit = ref(0)

const time = ref<TimeSel>({ preset: '24h', custom: null })
const kw = ref('')
const onlineOnly = ref(false)
const protocol = ref('')
const more = ref({ has_recording: false, has_file_ops: false, has_errors: false })
const moreCount = computed(() => Object.values(more.value).filter(Boolean).length)

const summary = ref({ online: 0, today_visits: 0, today_file_ops: 0 })

function params() {
  const r = resolveRange(time.value, 'datetime')
  const p: Record<string, string | number> = {}
  // 「全部时间」最多回溯 1 年（后端上限）
  p.start = r ? r[0] : formatDate(Date.now() - 365 * 86400e3)
  if (r && time.value.preset === 'custom') p.end = r[1]
  if (kw.value.trim()) p.q = kw.value.trim()
  if (onlineOnly.value) p.online = 1
  if (protocol.value) p.protocol = protocol.value
  for (const [k, v] of Object.entries(more.value)) if (v) p[k] = 1
  return p
}

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    const d = await get('/api/v1/webvpn/visits', { page: page.value, page_size: pageSize.value, ...params() })
    rows.value = d?.datas || []
    total.value = d?.count || 0
    truncated.value = !!d?.truncated
    scanLimit.value = d?.scan_limit || 0
  } catch (e) {
    if (!silent) ui.message.error(errMsg(e, '获取访问记录失败'))
  } finally {
    loading.value = false
  }
}
async function loadSummary() {
  try {
    summary.value = await get('/api/v1/webvpn/visits/summary')
  } catch { /* 摘要失败不打扰 */ }
}
function search() {
  page.value = 1
  load()
}
function refresh() {
  load()
  loadSummary()
}
watch([time, onlineOnly, protocol, more], search, { deep: true })

// 选中的访问（抽屉）
const openId = ref<string | null>(null)

// 静默刷新：页面可见、未打开抽屉时每 15 秒刷新一次（在线状态与时长）
let timer = 0
onMounted(() => {
  refresh()
  timer = window.setInterval(() => {
    if (document.visibilityState !== 'visible' || openId.value) return
    load(true)
    loadSummary()
  }, 15000)
})
onBeforeUnmount(() => clearInterval(timer))

// 导出
const exporting = ref(false)
async function exportVisits() {
  exporting.value = true
  try {
    await downloadBlob('/api/v1/webvpn/visits/export', params(), `webvpn_visits_${formatDate(Date.now(), 'YYYYMMDD_HHmmss')}.csv`)
  } catch (e) {
    ui.message.error(errMsg(e, '导出失败'))
  } finally {
    exporting.value = false
  }
}
// HTTP 逐请求明细（原审计导出接口，按时间范围）
async function exportRequests() {
  const r = resolveRange(time.value, 'datetime')
  exporting.value = true
  try {
    await downloadBlob('/api/v1/webvpn/audit/export', r ? { start_date: r[0], end_date: r[1] } : {}, `webvpn_audit_${formatDate(Date.now(), 'YYYYMMDD_HHmmss')}.csv`)
  } catch (e) {
    ui.message.error(errMsg(e, '导出失败'))
  } finally {
    exporting.value = false
  }
}
const moreMenu = [{ label: '导出 HTTP 逐请求明细', key: 'requests' }]

// ---------- 列 ----------
const dot = (online: boolean) => h('span', {
  class: ['rl-status flex-shrink-0', online ? 'rl-status--success' : 'rl-status--muted'], title: online ? '在线' : '已结束',
}, [h('span', { class: ['rl-status__dot', online && 'rl-status__dot--pulse'] })])

const badge = (cls: string, children: any, title?: string) => h('span', { class: ['ax-badge gap-4px whitespace-nowrap', cls], title }, children)

function badges(v: Visit) {
  const out: any[] = []
  if (v.kind === 'session' && hasScreen(v.protocol)) {
    if (v.recording.available) out.push(badge('text-info', [h(VideoClip), '录像']))
    else {
      out.push(h(NTooltip, { style: 'max-width: 320px' }, {
        trigger: () => badge('text-tertiary', [h(VideoOff), REC_SHORT[v.recording.reason] || '无录像']),
        default: () => recReasonText(v),
      }))
    }
  }
  if (v.file_ops) out.push(badge('text-secondary', `${v.file_ops} 次文件操作`))
  if (v.kind === 'http') {
    out.push(badge('text-secondary', `${v.requests} 个请求`))
    if (v.risk) out.push(badge(v.risk > 1 ? 'text-error' : 'text-warning', v.risk > 1 ? '高危' : '可疑'))
    else if (v.errors) out.push(badge('text-warning', `${v.errors} 个失败`, '状态码 ≥ 400 的请求'))
  }
  return h('div', { class: 'flex flex-wrap justify-end gap-6px' }, out.slice(0, 3))
}

const columns: DataTableColumns<Visit> = [
  {
    title: '用户', key: 'username', minWidth: 170,
    render: v => h('div', { class: 'flex min-w-0 items-center gap-10px' }, [
      dot(v.online),
      h('div', { class: 'min-w-0 leading-18px' }, [
        h('div', { class: 'truncate font-semibold' }, v.username || '—'),
        h('div', { class: 'truncate text-sm text-secondary' }, v.group || '—'),
      ]),
    ]),
  },
  {
    title: '应用', key: 'app_name', minWidth: 190,
    render: v => h('div', { class: 'flex min-w-0 items-center gap-10px' }, [
      h(protoIcon(v.protocol), { class: 'flex-shrink-0 text-lg text-secondary' }),
      h('div', { class: 'min-w-0 leading-18px' }, [
        h('div', { class: 'truncate' }, v.app_name),
        h('div', { class: 'truncate text-sm text-tertiary' }, PROTO_LABEL[v.protocol] || v.protocol.toUpperCase()),
      ]),
    ]),
  },
  { title: '来源', key: 'client_ip', minWidth: 170, render: v => ipCell(v.client_ip, v.country_code) },
  {
    title: '时间', key: 'started_at', width: 150,
    render: v => h('div', { class: 'leading-18px', title: formatDate(v.started_at) }, [
      h('div', null, relTime(v.started_at)),
      h('div', { class: 'text-sm text-tertiary' }, v.online ? `已持续 ${fmtDuration(v.duration)}` : fmtDuration(v.duration)),
    ]),
  },
  { title: '', key: 'badges', minWidth: 200, align: 'right', render: badges },
]

const rowProps = (v: Visit) => ({ class: 'cursor-pointer', onClick: () => { openId.value = v.id } })
const filtered = computed(() => !!(kw.value.trim() || onlineOnly.value || protocol.value || moreCount.value))
function clearFilters() {
  kw.value = ''
  onlineOnly.value = false
  protocol.value = ''
  more.value = { has_recording: false, has_file_ops: false, has_errors: false }
  search()
}
</script>

<template>
  <div>
    <PageHeader title="WebVPN 访问" description="谁在什么时候通过 WebVPN 访问了哪个应用；点击记录查看回放、文件操作与请求">
      <TimeRangePicker v-model="time" mode="datetime" />
      <NButton :loading="exporting" @click="exportVisits">
        <template #icon><ArrowDownload /></template>导出
      </NButton>
      <NDropdown trigger="click" :options="moreMenu" placement="bottom-end" @select="exportRequests">
        <button class="rl-icon-btn" title="更多"><More /></button>
      </NDropdown>
    </PageHeader>

    <!-- 摘要 -->
    <div class="mb-16px flex flex-wrap items-center gap-x-10px gap-y-4px text-md text-secondary">
      <button class="vs-sum" :class="onlineOnly && 'font-semibold'" @click="onlineOnly = !onlineOnly">
        <StatusDot :tone="summary.online ? 'success' : 'muted'" :pulse="!!summary.online">当前在线 <b class="tabular-nums">{{ summary.online }}</b></StatusDot>
      </button>
      <span class="text-quaternary">·</span>
      <span>今日访问 <b class="tabular-nums text-primary">{{ summary.today_visits }}</b></span>
      <span class="text-quaternary">·</span>
      <span>今日文件操作 <b class="tabular-nums text-primary">{{ summary.today_file_ops }}</b></span>
    </div>

    <RlPanel :padded="false">
      <template #toolbar>
        <NInput v-model:value="kw" placeholder="搜索用户 / 应用 / IP" clearable class="w-240px!" @keyup.enter="search" @clear="kw = ''; search()">
          <template #prefix><Search class="text-secondary" /></template>
        </NInput>
        <label class="flex cursor-pointer items-center gap-6px px-4px text-md">
          <NSwitch v-model:value="onlineOnly" size="small" />在线中
        </label>
        <NSelect v-model:value="protocol" :options="protocolOptions" class="w-150px!" />
        <NPopover trigger="click" placement="bottom-start">
          <template #trigger>
            <NButton :type="moreCount ? 'primary' : 'default'" :secondary="!!moreCount">
              <template #icon><Filter /></template>筛选<template v-if="moreCount"> · {{ moreCount }}</template>
            </NButton>
          </template>
          <div class="grid gap-10px py-4px">
            <NCheckbox v-model:checked="more.has_recording">只看有录像的会话</NCheckbox>
            <NCheckbox v-model:checked="more.has_file_ops">只看有文件操作的会话</NCheckbox>
            <NCheckbox v-model:checked="more.has_errors">只看有失败或可疑请求的 HTTP 访问</NCheckbox>
          </div>
        </NPopover>
        <NButton v-if="filtered" text size="small" @click="clearFilters">清除</NButton>
        <div class="flex-1" />
        <span class="text-sm text-secondary">{{ total }} 次访问</span>
        <button class="rl-icon-btn" title="刷新" @click="refresh"><ArrowClockwise /></button>
      </template>
      <div v-if="truncated" class="border-b border-b-solid px-20px py-8px text-sm text-warning b-[var(--color-border-default)]">
        所选时间范围内 HTTP 请求过多，只统计了最近 {{ scanLimit.toLocaleString() }} 条请求；缩小时间范围可查看完整记录。
      </div>
      <NDataTable
        :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(v: Visit) => v.id"
        :row-props="rowProps" size="small" :scroll-x="900"
      >
        <template #empty>
          <EmptyState
            :icon="DocumentSearch" :title="filtered ? '没有匹配的访问' : '暂无访问记录'"
            :description="filtered ? '换个关键词或清除筛选试试' : '用户通过 WebVPN 打开应用后会显示在这里'"
          />
        </template>
      </NDataTable>
      <template v-if="total > pageSize" #footer>
        <span />
        <NPagination
          v-model:page="page" v-model:page-size="pageSize" :item-count="total" show-size-picker :page-sizes="[20, 50, 100]"
          @update:page="() => load()" @update:page-size="search"
        />
      </template>
    </RlPanel>

    <VisitDrawer :id="openId" @close="openId = null" @changed="refresh" />
  </div>
</template>

<style scoped>
.vs-sum { display: inline-flex; padding: 0; border: 0; background: none; font: inherit; color: inherit; cursor: pointer; }
.vs-sum:hover { text-decoration: underline; text-underline-offset: 3px; }
</style>
