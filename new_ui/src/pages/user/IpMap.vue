<script setup lang="ts">
// IP 映射：MAC ↔ IP 绑定 / 保留，支持筛选、编辑、单条与批量删除
import { computed, h, onMounted, ref } from 'vue'
import {
  NButton, NDataTable, NIcon, NInput, NPagination, NSelect, type DataTableColumns, type DataTableRowKey,
} from 'naive-ui'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import Add from '~icons/fluent/add-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import Search from '~icons/fluent/search-20-regular'
import Routing from '~icons/fluent/arrow-routing-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import IpMapDrawer from './components/IpMapDrawer.vue'
import { errMsg, get, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { fmtTime } from './components/shared'

interface IpMapRow {
  id: number
  ip_addr: string
  ip_addr6?: string
  mac_addr: string
  group: string
  unique_mac: boolean
  username: string
  nickname: string
  keep: boolean
  note: string
  last_login: string
}

const PAGE_SIZE = 10 // 后端固定每页 10 条
const loading = ref(false)
const rows = ref<IpMapRow[]>([])
const total = ref(0)
const page = ref(1)

const blankFilters = () => ({ ip: '', mac: '', username: '', group: '', keep: null as string | null })
const filters = ref(blankFilters())
const keepOptions = [{ label: '已保留', value: '1' }, { label: '未保留', value: '0' }]

async function load(p = page.value) {
  page.value = p
  loading.value = true
  const f = filters.value
  const params: Record<string, any> = { page: p }
  if (f.ip) params.ip = f.ip.trim()
  if (f.mac) params.mac = f.mac.trim()
  if (f.username) params.username = f.username.trim()
  if (f.group) params.group = f.group.trim()
  if (f.keep) params.keep = f.keep
  try {
    const d = await get('/api/v1/user/ip_map/list', params)
    rows.value = d?.datas || []
    total.value = d?.count || 0
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
const search = () => load(1)
function resetFilters() {
  filters.value = blankFilters()
  load(1)
}
const hasFilter = computed(() => Object.values(filters.value).some(Boolean))

// 保留 / 唯一 MAC / 绑定用户为当前页统计
const stats = computed(() => ({
  keep: rows.value.filter(r => r.keep).length,
  uniqueMac: new Set(rows.value.map(r => r.mac_addr)).size,
  bound: new Set(rows.value.filter(r => r.username).map(r => r.username)).size,
}))

// —— 勾选 / 删除 ——
const checked = ref<DataTableRowKey[]>([])

async function batchDelete() {
  const ids = checked.value
  if (!ids.length) return
  if (!(await confirm(`确定删除选中的 ${ids.length} 条 IP 映射吗？删除后不可恢复。`, '批量删除'))) return
  try {
    await post('/api/v1/user/ip_map/batch_del', { ids })
    ui.message.success('已删除')
    checked.value = []
    load(1)
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

async function remove(r: IpMapRow) {
  if (!(await confirm(`确定删除 IP 映射 ${r.ip_addr} 吗？`, '删除映射'))) return
  try {
    await post(`/api/v1/user/ip_map/del?id=${r.id}`)
    ui.message.success('已删除')
    checked.value = checked.value.filter(k => k !== r.id)
    load(1)
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

// —— 编辑 ——
const editShow = ref(false)
const editId = ref(0)
function openEdit(id = 0) {
  editId.value = id
  editShow.value = true
}

const muted = (t = '—') => h('span', { class: 'text-quaternary' }, t)
const str = (k: keyof IpMapRow) => (a: IpMapRow, b: IpMapRow) => String(a[k] ?? '').localeCompare(String(b[k] ?? ''))

const columns: DataTableColumns<IpMapRow> = [
  { type: 'selection', width: 44 },
  {
    title: 'IP 地址', key: 'ip_addr', minWidth: 170, sorter: str('ip_addr'),
    render: r => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'rl-mono font-semibold' }, r.ip_addr),
      r.ip_addr6 ? h('div', { class: 'rl-mono truncate text-sm text-secondary' }, r.ip_addr6) : null,
    ]),
  },
  {
    title: 'MAC 地址', key: 'mac_addr', width: 200, sorter: str('mac_addr'),
    render: r => h('div', { class: 'flex items-center gap-6px' }, [
      h('span', { class: 'rl-mono' }, r.mac_addr),
      r.unique_mac ? h('span', { class: 'ax-badge text-success', title: '唯一 MAC' }, '唯一') : null,
    ]),
  },
  {
    title: '组 / 出口', key: 'group', width: 130, sorter: str('group'),
    render: r => (r.group ? h('span', { class: 'rl-kbd' }, r.group) : muted('全局')),
  },
  {
    title: '绑定用户', key: 'username', minWidth: 150, sorter: str('username'),
    render: r => (r.username
      ? h('span', { class: 'truncate' }, [r.username, r.nickname ? h('span', { class: 'ml-6px text-secondary' }, r.nickname) : null])
      : muted('未绑定')),
  },
  {
    title: 'IP 保留', key: 'keep', width: 96, sorter: (a, b) => Number(b.keep) - Number(a.keep),
    render: r => h(StatusDot, { tone: r.keep ? 'success' : 'muted' }, { default: () => (r.keep ? '保留' : '未保留') }),
  },
  {
    title: '备注', key: 'note', minWidth: 140, ellipsis: { tooltip: true }, sorter: str('note'),
    render: r => r.note || muted(),
  },
  {
    title: '最后登录', key: 'last_login', width: 160, sorter: str('last_login'),
    render: r => h('span', { class: 'text-secondary tabular-nums' }, fmtTime(r.last_login)),
  },
  {
    title: 'ID', key: 'id', width: 64, sorter: (a, b) => a.id - b.id,
    render: r => h('span', { class: 'rl-mono text-tertiary' }, r.id),
  },
  {
    title: '', key: 'actions', width: 100, align: 'right',
    render: r => h('div', { class: 'flex justify-end gap-12px' }, [
      h(NButton, { text: true, size: 'small', onClick: () => openEdit(r.id) }, { default: () => '编辑' }),
      h(NButton, { text: true, size: 'small', type: 'error', onClick: () => remove(r) }, { default: () => '删除' }),
    ]),
  },
]

onMounted(() => load(1))
</script>

<template>
  <div>
    <PageHeader title="IP 映射" description="客户端 MAC 与 VPN 地址的绑定关系。开启保留后，该地址固定分配给对应设备。">
      <NButton type="primary" @click="openEdit()">
        <template #icon><NIcon><Add /></NIcon></template>
        新建映射
      </NButton>
    </PageHeader>

    <div class="grid gap-16px">
      <div class="grid grid-cols-2 gap-16px lg:grid-cols-4">
        <StatCard label="映射总数" :value="total" :hint="hasFilter ? '按当前筛选条件' : undefined" />
        <StatCard label="IP 保留" :value="stats.keep" hint="当前页" />
        <StatCard label="唯一 MAC" :value="stats.uniqueMac" hint="当前页" />
        <StatCard label="已绑定用户" :value="stats.bound" hint="当前页" />
      </div>

      <RlPanel :padded="false">
        <template #toolbar>
          <template v-if="checked.length">
            <span class="text-md font-semibold">已选 {{ checked.length }} 项</span>
            <NButton size="small" secondary type="error" @click="batchDelete">
              <template #icon><NIcon><Delete /></NIcon></template>
              批量删除
            </NButton>
            <NButton size="small" text @click="checked = []">取消选择</NButton>
          </template>
          <template v-else>
            <NInput v-model:value="filters.ip" placeholder="IP 地址" clearable class="w-160px!" @keydown.enter="search" @clear="search">
              <template #prefix><NIcon><Search /></NIcon></template>
            </NInput>
            <NInput v-model:value="filters.mac" placeholder="MAC 地址" clearable class="w-160px!" @keydown.enter="search" @clear="search" />
            <NInput v-model:value="filters.username" placeholder="用户名 / 姓名" clearable class="w-160px!" @keydown.enter="search" @clear="search" />
            <NInput v-model:value="filters.group" placeholder="组 / 出口" clearable class="w-140px!" @keydown.enter="search" @clear="search" />
            <NSelect v-model:value="filters.keep" :options="keepOptions" placeholder="保留状态" clearable class="w-120px!" @update:value="search" />
            <NButton @click="search">查询</NButton>
            <NButton v-if="hasFilter" text @click="resetFilters">重置</NButton>
          </template>
          <div class="flex-1" />
          <span class="text-sm text-secondary">共 {{ total }} 条</span>
          <button class="rl-icon-btn" title="刷新" @click="load()"><ArrowClockwise /></button>
        </template>

        <NDataTable
          v-model:checked-row-keys="checked"
          :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: IpMapRow) => r.id"
          size="small" :scroll-x="1240"
        >
          <template #empty>
            <EmptyState :icon="Routing" title="暂无 IP 映射" :description="hasFilter ? '没有匹配的记录，试试调整筛选条件。' : '客户端首次连接后会自动生成映射，也可以手动新建。'" />
          </template>
        </NDataTable>

        <template #footer>
          <span class="text-sm text-secondary">每页 {{ PAGE_SIZE }} 条</span>
          <NPagination :page="page" :page-size="PAGE_SIZE" :item-count="total" @update:page="load" />
        </template>
      </RlPanel>
    </div>

    <IpMapDrawer v-model:show="editShow" :map-id="editId" @saved="load(1)" />
  </div>
</template>
