<script setup lang="ts">
// 策略管理：带宽 / 流量配额 / DNS / 路由 / ACL / 域名拆分 / FakeDNS 模版，供用户组与用户引用
import { computed, h, onMounted, ref } from 'vue'
import {
  NButton, NCheckbox, NCheckboxGroup, NDataTable, NDrawer, NDrawerContent, NDropdown, NInput, NModal,
  NPagination, NSpin, type DataTableColumns, type DataTableRowKey,
} from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import More from '~icons/fluent/more-horizontal-20-regular'
import Search from '~icons/fluent/search-20-regular'
import ShieldTask from '~icons/fluent/shield-task-20-regular'
import Checkmark from '~icons/fluent/checkmark-circle-20-regular'
import Gauge from '~icons/fluent/gauge-20-regular'
import DataUsage from '~icons/fluent/data-usage-20-regular'
import Globe from '~icons/fluent/globe-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import { get, post, errMsg } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'
import { useAuthStore } from '@/stores/auth'
import PolicyForm from './components/PolicyForm.vue'
import { convertBandwidth, convertTraffic, defaultPolicy, resetLabel, type PolicyModel } from './components/policy'

const auth = useAuthStore()
const PAGE_SIZE = 10

// —— 列表 ——
const loading = ref(false)
const rows = ref<any[]>([])
const page = ref(1)
const count = ref(0)
const stats = ref<Record<string, number>>({})

async function load(p = page.value) {
  page.value = p
  loading.value = true
  try {
    const d = await get('/api/v1/policy/list', { page: p })
    rows.value = d?.datas || []
    count.value = d?.count || 0
    stats.value = d || {}
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

const bw = (v: number) => (v > 0 ? `${convertBandwidth(v, 'BYTE', 'Mbps')} Mbps` : '')
const unlimited = () => h('span', { class: 'text-tertiary' }, '不限')

const rowMenu = [
  { label: '复制策略', key: 'copy' },
  { label: '应用到用户组', key: 'applyGroups' },
  { label: '应用到用户', key: 'applyUsers' },
  { type: 'divider', key: 'd' },
  { label: '删除策略', key: 'delete', props: { style: 'color: var(--color-text-error)' } },
]
function onRowCmd(row: any, key: string) {
  if (key === 'copy') copyPolicy(row)
  else if (key === 'applyGroups') openApply(row, 'groups')
  else if (key === 'applyUsers') openApply(row, 'users')
  else if (key === 'delete') delPolicy(row)
}

const columns = computed<DataTableColumns<any>>(() => {
  const cols: DataTableColumns<any> = [
    {
      title: '策略', key: 'name', minWidth: 180, sorter: (a, b) => a.name.localeCompare(b.name),
      render: r => h('div', { class: 'min-w-0' }, [
        h('div', { class: 'truncate font-semibold' }, r.name),
        h('div', { class: 'truncate text-sm text-secondary' }, r.note || `ID ${r.id}`),
      ]),
    },
    {
      title: '下行', key: 'bandwidth', width: 110, sorter: (a, b) => a.bandwidth - b.bandwidth,
      render: r => (r.bandwidth > 0 ? h('span', { class: 'tabular-nums' }, bw(r.bandwidth)) : unlimited()),
    },
    {
      title: '上行', key: 'bandwidth_up', width: 110, sorter: (a, b) => (a.bandwidth_up || 0) - (b.bandwidth_up || 0),
      render: r => (r.bandwidth_up > 0 ? h('span', { class: 'tabular-nums' }, bw(r.bandwidth_up)) : unlimited()),
    },
    {
      title: '流量配额', key: 'traffic_quota', width: 140, sorter: (a, b) => (a.traffic_quota || 0) - (b.traffic_quota || 0),
      render: r => (r.traffic_quota > 0
        ? h('div', [
          h('span', { class: 'tabular-nums' }, `${convertTraffic(r.traffic_quota, 'BYTE', 'GB')} GB`),
          r.traffic_reset ? h('span', { class: 'ml-6px text-sm text-secondary' }, `${resetLabel(r.traffic_reset)}重置`) : null,
        ])
        : unlimited()),
    },
    {
      title: '本地网络', key: 'allow_lan', width: 96,
      render: r => (r.allow_lan ? h('span', '允许') : h('span', { class: 'text-tertiary' }, '禁止')),
    },
  ]
  if (auth.showFakeDNS) {
    cols.push({
      title: 'FakeDNS', key: 'enable_fakedns', width: 96,
      render: r => (r.enable_fakedns ? h('span', { class: 'ax-badge text-info' }, '启用') : h('span', { class: 'text-tertiary' }, '禁用')),
    })
  }
  cols.push(
    {
      title: '引用', key: 'ref', width: 170,
      render: r => h('div', { class: 'flex items-center gap-sm' }, [
        h('span', { class: 'text-secondary' }, [h('b', { class: 'text-primary tabular-nums' }, r.group_count || 0), ' 组']),
        h('span', { class: 'text-secondary' }, [h('b', { class: 'text-primary tabular-nums' }, r.user_count || 0), ' 用户']),
        h(NButton, { text: true, type: 'info', size: 'small', onClick: () => openRef(r) }, () => '详情'),
      ]),
    },
    {
      title: '状态', key: 'status', width: 90, sorter: (a, b) => a.status - b.status,
      render: r => h(StatusDot, { tone: r.status === 1 ? 'success' : 'muted' }, () => (r.status === 1 ? '启用' : '停用')),
    },
    {
      title: '更新时间', key: 'updated_at', width: 170, sorter: (a, b) => String(a.updated_at).localeCompare(String(b.updated_at)),
      render: r => h('span', { class: 'text-secondary tabular-nums' }, formatDate(r.updated_at)),
    },
    {
      title: '', key: 'ops', width: 104, fixed: 'right',
      render: r => h('div', { class: 'flex items-center justify-end gap-4px' }, [
        h(NButton, { text: true, size: 'small', onClick: () => openEdit(r) }, () => '编辑'),
        h(NDropdown, { trigger: 'click', options: rowMenu, onSelect: (k: string) => onRowCmd(r, k) },
          () => h('button', { class: 'rl-icon-btn', title: '更多' }, h(More))),
      ]),
    },
  )
  return cols
})

// —— 新建 / 编辑 ——
const drawer = ref(false)
const isEdit = ref(false)
const saving = ref(false)
const detailLoading = ref(false)
const form = ref<PolicyModel>(defaultPolicy())
const formRef = ref<InstanceType<typeof PolicyForm> | null>(null)

async function openEdit(row?: any) {
  formRef.value?.reset()
  isEdit.value = !!row
  form.value = defaultPolicy()
  drawer.value = true
  if (!row) return
  detailLoading.value = true
  try {
    const d = await get('/api/v1/policy/detail', { id: row.id })
    d.bandwidth_format = String(convertBandwidth(d.bandwidth, 'BYTE', 'Mbps'))
    d.bandwidth_up_format = String(convertBandwidth(d.bandwidth_up || 0, 'BYTE', 'Mbps'))
    d.traffic_quota_format = String(convertTraffic(d.traffic_quota || 0, 'BYTE', 'GB'))
    d.traffic_reset ||= ''
    d.client_dns ??= [{ val: '114.114.114.114', note: '默认dns' }]
    d.route_include ??= [{ val: 'all', note: '默认全局代理' }]
    d.route_exclude ??= []
    d.link_acl ??= []
    form.value = { ...defaultPolicy(), ...d }
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    detailLoading.value = false
  }
}

async function save() {
  if (saving.value || !(await formRef.value?.validate())) return
  const f = form.value
  // 单位换算：Mbps → Byte/s，GB → 字节
  f.bandwidth = convertBandwidth(f.bandwidth_format, 'Mbps', 'BYTE')
  f.bandwidth_up = convertBandwidth(f.bandwidth_up_format, 'Mbps', 'BYTE')
  const gb = parseFloat(f.traffic_quota_format) || 0
  f.traffic_quota = gb > 0 ? Math.round(gb * 1024 ** 3) : 0
  if (f.traffic_quota === 0) f.traffic_reset = ''
  else if (!f.traffic_reset) f.traffic_reset = 'monthly'
  saving.value = true
  try {
    await post('/api/v1/policy/set', f)
    ui.message.success('保存成功')
    drawer.value = false
    load(1)
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

async function copyPolicy(row: any) {
  if (!(await confirm(`确定要复制策略「${row.name}」吗？`, '复制策略', false))) return
  try {
    await post(`/api/v1/policy/copy?id=${row.id}`)
    ui.message.success('策略已复制')
    load(1)
  } catch (e) { ui.message.error(errMsg(e)) }
}

async function delPolicy(row: any) {
  if (!(await confirm(`确定要删除策略「${row.name}」吗？删除后不可恢复。`, '删除策略'))) return
  try {
    await post(`/api/v1/policy/del?id=${row.id}`)
    ui.message.success('删除成功')
    load(1)
  } catch (e) { ui.message.error(errMsg(e)) }
}

// —— 应用到组 / 用户 ——
const applyMode = ref<'groups' | 'users' | null>(null)
const applyPolicy = ref<{ id: number; name: string }>({ id: 0, name: '' })
const applyLoading = ref(false)
const allGroups = ref<{ id: number; name: string; policy_id: number }[]>([])
const allUsers = ref<{ id: number; username: string; nickname: string; policy_id: number }[]>([])
const selGroups = ref<number[]>([])
const selUsers = ref<number[]>([])
const userKw = ref('')
const applyShow = computed({ get: () => !!applyMode.value, set: v => { if (!v) applyMode.value = null } })

async function loadGroups() {
  try {
    const d = await get('/api/v1/group/list', { page: 1, page_size: 9999 })
    allGroups.value = (d?.datas || []).map((g: any) => ({ id: g.id, name: g.name, policy_id: g.policy_id }))
  } catch { /* 忽略 */ }
}
async function loadUsers() {
  try {
    const d = await get('/api/v1/user/list', { page: 1, page_size: 9999 })
    allUsers.value = (d?.datas || []).map((u: any) => ({ id: u.id, username: u.username, nickname: u.nickname, policy_id: u.policy_id }))
  } catch { /* 忽略 */ }
}
const filteredUsers = computed(() => {
  const k = userKw.value.trim().toLowerCase()
  if (!k) return allUsers.value
  return allUsers.value.filter(u => u.username.toLowerCase().includes(k) || (u.nickname || '').toLowerCase().includes(k))
})

function openApply(row: any, mode: 'groups' | 'users') {
  applyPolicy.value = { id: row.id, name: row.name }
  selGroups.value = []
  selUsers.value = []
  userKw.value = ''
  applyMode.value = mode
  mode === 'groups' ? loadGroups() : loadUsers()
}

async function doApply() {
  const isG = applyMode.value === 'groups'
  const ids = isG ? selGroups.value : selUsers.value
  if (!ids.length) { ui.message.warning(isG ? '请选择至少一个用户组' : '请选择至少一个用户'); return }
  applyLoading.value = true
  try {
    await post(isG ? '/api/v1/policy/apply_to_groups' : '/api/v1/policy/apply_to_users',
      isG ? { policy_id: applyPolicy.value.id, group_ids: ids } : { policy_id: applyPolicy.value.id, user_ids: ids })
    ui.message.success('应用成功')
    applyMode.value = null
    load()
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    applyLoading.value = false
  }
}

// —— 引用详情 ——
const refShow = ref(false)
const refLoading = ref(false)
const refRemoving = ref(false)
const refPolicy = ref<{ id: number; name: string }>({ id: 0, name: '' })
const refGroups = ref<any[]>([])
const refUsers = ref<any[]>([])
const selRefGroups = ref<DataTableRowKey[]>([])
const selRefUsers = ref<DataTableRowKey[]>([])

async function openRef(row: { id: number; name: string }) {
  refPolicy.value = { id: row.id, name: row.name }
  selRefGroups.value = []
  selRefUsers.value = []
  refShow.value = true
  refLoading.value = true
  try {
    const d = await get('/api/v1/policy/used_by', { id: row.id })
    refGroups.value = d?.groups || []
    refUsers.value = d?.users || []
  } catch (e) {
    refGroups.value = []
    refUsers.value = []
    ui.message.error(errMsg(e))
  } finally {
    refLoading.value = false
  }
}

async function removeRef(kind: 'groups' | 'users', ids: DataTableRowKey[]) {
  if (!ids.length) return
  refRemoving.value = true
  try {
    await post(kind === 'groups' ? '/api/v1/policy/remove_from_groups' : '/api/v1/policy/remove_from_users',
      kind === 'groups' ? { policy_id: refPolicy.value.id, group_ids: ids } : { policy_id: refPolicy.value.id, user_ids: ids })
    ui.message.success('已移出')
    openRef(refPolicy.value)
    load()
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    refRemoving.value = false
  }
}

const refGroupCols: DataTableColumns<any> = [
  { type: 'selection' },
  { title: '用户组', key: 'name', render: r => h('div', [h('div', { class: 'font-semibold' }, r.name), r.note ? h('div', { class: 'text-sm text-secondary' }, r.note) : null]) },
  { title: 'ID', key: 'id', width: 70 },
  { title: '', key: 'ops', width: 64, render: r => h(NButton, { text: true, type: 'error', size: 'small', disabled: refRemoving.value, onClick: () => removeRef('groups', [r.id]) }, () => '移出') },
]
const refUserCols: DataTableColumns<any> = [
  { type: 'selection' },
  { title: '用户', key: 'username', render: r => h('div', [h('div', { class: 'font-semibold' }, r.username), r.nickname ? h('div', { class: 'text-sm text-secondary' }, r.nickname) : null]) },
  { title: 'ID', key: 'id', width: 70 },
  { title: '', key: 'ops', width: 64, render: r => h(NButton, { text: true, type: 'error', size: 'small', disabled: refRemoving.value, onClick: () => removeRef('users', [r.id]) }, () => '移出') },
]

onMounted(() => load(1))
</script>

<template>
  <PageHeader title="策略" description="带宽、流量配额、DNS、路由与访问控制模版，用户组与用户通过引用策略生效">
    <NButton type="primary" @click="openEdit()"><template #icon><Add /></template>新建策略</NButton>
  </PageHeader>

  <div class="grid gap-16px">
    <div class="grid gap-16px" :class="auth.showFakeDNS ? 'grid-cols-2 lg:grid-cols-5' : 'grid-cols-2 lg:grid-cols-4'">
      <StatCard label="策略总数" :value="count" :icon="ShieldTask" />
      <StatCard label="已启用" :value="stats.stats_active || 0" :icon="Checkmark" />
      <StatCard v-if="auth.showFakeDNS" label="启用 FakeDNS" :value="stats.stats_fakedns || 0" :icon="Globe" />
      <StatCard label="带宽限速" :value="stats.stats_band || 0" :icon="Gauge" hint="设置了上行或下行限速的策略" />
      <StatCard label="流量配额" :value="stats.stats_quota || 0" :icon="DataUsage" hint="设置了流量配额的策略" />
    </div>

    <RlPanel :padded="false">
      <template #toolbar>
        <span class="text-md font-semibold">策略列表</span>
        <div class="flex-1" />
        <span class="text-sm text-secondary">共 {{ count }} 条</span>
        <button class="rl-icon-btn" title="刷新" @click="load()"><ArrowClockwise /></button>
      </template>
      <NDataTable
        :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: any) => r.id"
        size="small" :scroll-x="1180"
      >
        <template #empty>
          <EmptyState title="暂无策略" description="策略定义带宽、路由、DNS 与访问控制，创建后在用户组中引用">
            <NButton @click="openEdit()">新建策略</NButton>
          </EmptyState>
        </template>
      </NDataTable>
      <template #footer>
        <span />
        <NPagination :page="page" :page-size="PAGE_SIZE" :item-count="count" @update:page="load" />
      </template>
    </RlPanel>
  </div>

  <!-- 新建 / 编辑抽屉 -->
  <NDrawer v-model:show="drawer" :width="720" :mask-closable="false" display-directive="show">
    <NDrawerContent :title="isEdit ? `编辑策略 · ${form.name}` : '新建策略'" closable :native-scrollbar="false">
      <NSpin :show="detailLoading">
        <div v-if="isEdit" class="mb-8px text-sm text-secondary">策略 ID <span class="rl-mono">{{ form.id }}</span></div>
        <PolicyForm ref="formRef" v-model="form" />
      </NSpin>
      <template #footer>
        <div class="flex justify-end gap-sm">
          <NButton @click="drawer = false">取消</NButton>
          <NButton type="primary" :loading="saving" :disabled="detailLoading" @click="save">保存</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>

  <!-- 应用到组 / 用户 -->
  <NModal
    v-model:show="applyShow" preset="card" :mask-closable="false" class="w-560px! max-w-[calc(100vw-32px)]"
    :title="applyMode === 'groups' ? '应用策略到用户组' : '应用策略到用户'"
  >
    <div class="mb-12px text-md text-secondary">
      将策略 <b class="text-primary">{{ applyPolicy.name }}</b> 应用到所选{{ applyMode === 'groups' ? '用户组' : '用户' }}，原有策略将被替换。
    </div>
    <template v-if="applyMode === 'groups'">
      <div v-if="allGroups.length" class="apply-list">
        <NCheckboxGroup v-model:value="selGroups">
          <div v-for="g in allGroups" :key="g.id" class="apply-item">
            <NCheckbox :value="g.id" class="min-w-0 flex-1">{{ g.name }}</NCheckbox>
            <span v-if="g.policy_id === applyPolicy.id" class="ax-badge text-success">当前策略</span>
          </div>
        </NCheckboxGroup>
      </div>
      <EmptyState v-else title="暂无可用的用户组" />
    </template>
    <template v-else>
      <NInput v-model:value="userKw" placeholder="搜索用户名或昵称…" clearable class="mb-8px">
        <template #prefix><Search /></template>
      </NInput>
      <div v-if="filteredUsers.length" class="apply-list">
        <NCheckboxGroup v-model:value="selUsers">
          <div v-for="u in filteredUsers" :key="u.id" class="apply-item">
            <NCheckbox :value="u.id" class="min-w-0 flex-1">
              {{ u.username }}<span v-if="u.nickname" class="ml-6px text-secondary">{{ u.nickname }}</span>
            </NCheckbox>
            <span v-if="u.policy_id === applyPolicy.id" class="ax-badge text-success">当前策略</span>
          </div>
        </NCheckboxGroup>
      </div>
      <EmptyState v-else title="暂无匹配的用户" />
    </template>
    <template #footer>
      <div class="flex items-center gap-sm">
        <span class="text-sm text-secondary">已选 {{ applyMode === 'groups' ? selGroups.length : selUsers.length }} 项</span>
        <div class="flex-1" />
        <NButton @click="applyMode = null">取消</NButton>
        <NButton type="primary" :loading="applyLoading" @click="doApply">确认应用</NButton>
      </div>
    </template>
  </NModal>

  <!-- 引用详情 -->
  <NDrawer v-model:show="refShow" :width="640">
    <NDrawerContent :title="`引用详情 · ${refPolicy.name}`" closable :native-scrollbar="false">
      <NSpin :show="refLoading">
        <div class="mb-8px flex items-center gap-sm">
          <span class="text-md font-semibold">用户组</span>
          <span class="text-sm text-secondary">{{ refGroups.length }}</span>
          <div class="flex-1" />
          <NButton
            v-if="refGroups.length" size="small" secondary type="error" :disabled="!selRefGroups.length" :loading="refRemoving"
            @click="removeRef('groups', selRefGroups)"
          >批量移出（{{ selRefGroups.length }}）</NButton>
        </div>
        <div class="ref-box">
          <NDataTable
            v-if="refGroups.length" v-model:checked-row-keys="selRefGroups" :columns="refGroupCols" :data="refGroups"
            :row-key="(r: any) => r.id" :bordered="false" size="small" :max-height="260"
          />
          <div v-else class="py-20px text-center text-sm text-tertiary">无用户组引用</div>
        </div>

        <div class="mb-8px mt-24px flex items-center gap-sm">
          <span class="text-md font-semibold">用户</span>
          <span class="text-sm text-secondary">{{ refUsers.length }}</span>
          <div class="flex-1" />
          <NButton
            v-if="refUsers.length" size="small" secondary type="error" :disabled="!selRefUsers.length" :loading="refRemoving"
            @click="removeRef('users', selRefUsers)"
          >批量移出（{{ selRefUsers.length }}）</NButton>
        </div>
        <div class="ref-box">
          <NDataTable
            v-if="refUsers.length" v-model:checked-row-keys="selRefUsers" :columns="refUserCols" :data="refUsers"
            :row-key="(r: any) => r.id" :bordered="false" size="small" :max-height="260"
          />
          <div v-else class="py-20px text-center text-sm text-tertiary">无用户引用</div>
        </div>
        <div class="mt-12px text-sm text-secondary">移出后，对应用户组 / 用户将不再引用该策略。</div>
      </NSpin>
      <template #footer>
        <NButton @click="refShow = false">关闭</NButton>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.apply-list { max-height: 360px; overflow-y: auto; border: 1px solid var(--color-border-default); border-radius: 8px; }
.apply-list :deep(.n-checkbox-group) { display: block; }
.apply-item { display: flex; align-items: center; gap: 10px; padding: 8px 12px; }
.apply-item + .apply-item { border-top: 1px solid var(--color-border-default); }
.apply-item:hover { background: var(--color-action-hover); }
.ref-box { border: 1px solid var(--color-border-default); border-radius: 8px; overflow: hidden; }
</style>
