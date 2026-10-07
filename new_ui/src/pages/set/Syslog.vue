<script setup lang="ts">
// 系统日志：WebSocket 实时流（级别 / 关键字过滤、暂停、跟随滚动、清空、下载）+ 按日期查询历史日志
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { NButton, NDatePicker, NInput, NSelect } from 'naive-ui'
import dayjs from 'dayjs'
import Search from '~icons/fluent/search-20-regular'
import Pause from '~icons/fluent/pause-20-regular'
import Play from '~icons/fluent/play-20-regular'
import ArrowDown from '~icons/fluent/arrow-down-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import History from '~icons/fluent/history-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import AxTabs from '@/components/AxTabs.vue'
import EmptyState from '@/components/EmptyState.vue'
import SyslogView from './components/SyslogView.vue'
import { type SyslogEntry, useSyslogWs } from './components/SyslogWs'
import { errMsg, get } from '@/api/http'
import { ui } from '@/utils/ui'

type Row = SyslogEntry & { id: number }
const LEVELS = ['Trace', 'Debug', 'Info', 'Warn', 'Error', 'Fatal'].map(v => ({ label: v, value: v }))
const MAX_LOGS = 2000
let seq = 0

const mode = ref<'live' | 'history'>('live')
const modeTabs = [{ label: '实时', value: 'live' as const }, { label: '历史', value: 'history' as const }]

// 下载为 .log 文本
function downloadLines(rows: SyslogEntry[], name: string) {
  if (!rows.length) return ui.message.warning('没有可下载的日志')
  const text = rows.map(r => `${r.time} [${r.level}] ${r.msg}`).join('\n')
  const url = URL.createObjectURL(new Blob([text + '\n'], { type: 'text/plain;charset=utf-8' }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}
const nearBottom = (el: HTMLElement) => el.scrollHeight - el.scrollTop - el.clientHeight < 60

// —— 实时 ——
const logs = ref<Row[]>([])
const level = ref<string | null>(null)
const kw = ref('')
const follow = ref(true)
const liveView = ref<InstanceType<typeof SyslogView>>()

const { live, connected, connect, setLive } = useSyslogWs(e => {
  logs.value.push({ ...e, id: ++seq })
  if (logs.value.length > MAX_LOGS) logs.value.splice(0, logs.value.length - MAX_LOGS)
})

const filtered = computed(() => {
  const k = kw.value.toLowerCase()
  return logs.value.filter(l => (!level.value || l.level === level.value) && (!k || l.msg.toLowerCase().includes(k) || l.time.includes(k)))
})

function scrollLive() {
  const el = liveView.value?.el
  if (el) el.scrollTop = el.scrollHeight
}
watch(() => [filtered.value.length, logs.value[logs.value.length - 1]?.id], () => { if (follow.value) nextTick(scrollLive) })

let lastTop = 0
function onLiveScroll(el: HTMLElement) {
  // 回到底部恢复跟随；用户向上滚动则暂停跟随
  if (nearBottom(el)) follow.value = true
  else if (el.scrollTop < lastTop) follow.value = false
  lastTop = el.scrollTop
}
function toggleFollow() {
  follow.value = !follow.value
  if (follow.value) nextTick(scrollLive)
}

// —— 历史 ——
const historyEnabled = ref(false)
const historyDates = ref<Set<string>>(new Set())
const hDate = ref<string | null>(null)
const hLevel = ref<string | null>(null)
const hKw = ref('')
const hLogs = ref<Row[]>([])
const hTotal = ref(0)
const hPage = ref(1)
const H_PAGE_SIZE = 500
const hLoading = ref(false)
const hNoMore = ref(false)
const historyView = ref<InstanceType<typeof SyslogView>>()

async function checkHistory() {
  try {
    historyEnabled.value = !!(await get('/api/v1/set/syslog/history_enabled'))?.enabled
    if (!historyEnabled.value) return
    hDate.value = dayjs().format('YYYY-MM-DD')
    const d = await get('/api/v1/set/syslog/history_dates').catch(() => null)
    historyDates.value = new Set(d?.dates || [])
  } catch {
    historyEnabled.value = false
  }
}
// 有日志文件的日期可选；拿不到列表时不限制
const dateDisabled = (ts: number) => historyDates.value.size > 0 && !historyDates.value.has(dayjs(ts).format('YYYY-MM-DD'))

async function fetchPage(page: number) {
  const params: Record<string, any> = { date: hDate.value, page, page_size: H_PAGE_SIZE }
  if (hLevel.value) params.level = hLevel.value
  if (hKw.value) params.keyword = hKw.value
  const d = await get('/api/v1/set/syslog/history_list', params)
  const rows: SyslogEntry[] = d?.datas || []
  hTotal.value = d?.count || 0
  hLogs.value.push(...rows.map(r => ({ ...r, id: ++seq })))
  hPage.value = page
  if (rows.length < H_PAGE_SIZE) hNoMore.value = true
}

// 加载当天全部日志并滚到末尾（最新）
async function loadHistory() {
  if (!hDate.value) return
  hLoading.value = true
  hLogs.value = []
  hNoMore.value = false
  hTotal.value = 0
  try {
    for (let p = 1; p <= 5000 && !hNoMore.value; p++) await fetchPage(p)
    await nextTick()
    requestAnimationFrame(() => {
      const el = historyView.value?.el
      if (el) el.scrollTop = el.scrollHeight
    })
  } catch (e) {
    ui.message.error(errMsg(e, '加载历史日志失败'))
  } finally {
    hLoading.value = false
  }
}

async function onHistoryScroll(el: HTMLElement) {
  if (!nearBottom(el) || hLoading.value || hNoMore.value) return
  hLoading.value = true
  try { await fetchPage(hPage.value + 1) } catch (e) { ui.message.error(errMsg(e, '加载历史日志失败')) } finally { hLoading.value = false }
}

let historyLoaded = false
watch(mode, v => {
  if (v === 'history' && historyEnabled.value && !historyLoaded) {
    historyLoaded = true
    loadHistory()
  }
  if (v === 'live' && follow.value) nextTick(scrollLive)
})

onMounted(() => {
  connect()
  checkHistory()
})
</script>

<template>
  <div>
    <PageHeader title="系统日志" description="服务运行日志：实时推送，或按日期查询已落盘的历史日志">
      <template #meta>
        <template v-if="mode === 'live'">
          <StatusDot v-if="!live" tone="muted">已暂停</StatusDot>
          <StatusDot v-else :tone="connected ? 'success' : 'warning'" :pulse="connected">{{ connected ? '已连接' : '连接中…' }}</StatusDot>
        </template>
        <StatusDot v-else tone="muted">历史 · {{ hDate || '—' }}</StatusDot>
      </template>
    </PageHeader>

    <!-- 同一面板：工具栏左侧切换 实时 / 历史，其余控件随模式变化 -->
    <RlPanel :padded="false">
      <template #toolbar>
        <AxTabs v-model="mode" :options="modeTabs" size="small" />
        <div class="mx-4px h-20px w-1px bg-[var(--color-border-default)]" />
        <template v-if="mode === 'live'">
          <NSelect v-model:value="level" :options="LEVELS" placeholder="全部级别" clearable class="w-130px!" />
          <NInput v-model:value="kw" placeholder="搜索关键字" clearable class="w-240px!">
            <template #prefix><Search class="text-secondary" /></template>
          </NInput>
          <div class="flex-1" />
          <span class="text-sm text-secondary">{{ filtered.length }} / {{ logs.length }} 条<template v-if="logs.length >= MAX_LOGS">（仅保留最近 {{ MAX_LOGS }} 条）</template></span>
          <NButton @click="setLive(!live)">
            <template #icon><Play v-if="!live" /><Pause v-else /></template>
            {{ live ? '暂停' : '继续' }}
          </NButton>
          <NButton :type="follow ? 'default' : 'info'" :secondary="!follow" @click="toggleFollow">
            <template #icon><ArrowDown /></template>
            {{ follow ? '跟随中' : '跟随滚动' }}
          </NButton>
          <button class="rl-icon-btn" title="下载当前筛选结果" @click="downloadLines(filtered, `remlink-syslog-${dayjs().format('YYYYMMDD-HHmmss')}.log`)"><ArrowDownload /></button>
          <button class="rl-icon-btn" title="清空" @click="logs = []"><Delete /></button>
        </template>
        <template v-else-if="historyEnabled">
          <NDatePicker v-model:formatted-value="hDate" type="date" value-format="yyyy-MM-dd" :is-date-disabled="dateDisabled" :clearable="false" class="w-160px!" @update:formatted-value="loadHistory" />
          <NSelect v-model:value="hLevel" :options="LEVELS" placeholder="全部级别" clearable class="w-130px!" @update:value="loadHistory" />
          <NInput v-model:value="hKw" placeholder="搜索关键字，回车查询" clearable class="w-240px!" @keyup.enter="loadHistory" @clear="hKw = ''; loadHistory()">
            <template #prefix><Search class="text-secondary" /></template>
          </NInput>
          <NButton :loading="hLoading" @click="loadHistory">查询</NButton>
          <div class="flex-1" />
          <span class="text-sm text-secondary">{{ hLogs.length }} 条（共 {{ hTotal }} 条）</span>
          <button class="rl-icon-btn" title="下载已加载的日志" @click="downloadLines(hLogs, `remlink-syslog-${hDate}.log`)"><ArrowDownload /></button>
        </template>
      </template>

      <!-- 实时：v-show 保留滚动位置 -->
      <SyslogView
        v-show="mode === 'live'"
        ref="liveView" :entries="filtered" :keyword="kw"
        :empty-text="logs.length ? '无匹配日志' : '等待日志…'"
        :empty-hint="!logs.length && live && !connected ? '正在建立连接…' : ''"
        @scroll="onLiveScroll"
      />

      <!-- 历史 -->
      <template v-if="mode === 'history'">
        <EmptyState v-if="!historyEnabled" :icon="History" title="历史日志不可用" description="未配置文件日志路径（log_path），系统日志只在内存中实时推送。可在「软件配置」中设置 log_path 后启用。" />
        <SyslogView
          v-else
          ref="historyView" :entries="hLogs" :keyword="hKw"
          :empty-text="hLoading ? '加载中…' : '当日无日志或无匹配记录'"
          @scroll="onHistoryScroll"
        >
          <div v-if="hLogs.length" class="py-8px text-center text-sm text-tertiary">{{ hLoading ? '加载中…' : hNoMore ? '已经到底了' : '' }}</div>
        </SyslogView>
      </template>
    </RlPanel>
  </div>
</template>
