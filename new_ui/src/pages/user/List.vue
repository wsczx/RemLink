<script setup lang="ts">
// 用户列表：增删改、批量导入 / 模板下载、OTP 二维码、批量邮件 / 删除、重置流量配额
import { computed, h, onMounted, ref } from 'vue'
import {
  NButton, NDataTable, NDropdown, NIcon, NInput, NModal, NPagination, NSelect, NSpin, type DataTableColumns,
  type DataTableRowKey,
} from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import Add from '~icons/fluent/add-20-regular'
import MoreHorizontal from '~icons/fluent/more-horizontal-20-regular'
import ArrowUpload from '~icons/fluent/arrow-upload-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import Mail from '~icons/fluent/mail-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import People from '~icons/fluent/people-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import UserEditDrawer from './components/UserEditDrawer.vue'
import { ApiError, errMsg, get, http, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'
import { USER_TYPES, fmtTraffic, typeLabel } from './components/shared'

interface UserRow {
  id: number
  type: string
  username: string
  nickname: string
  email: string
  phone: string
  otp_secret: string
  disable_otp: boolean
  groups: string[] | null
  status: number
  change_pwd: boolean
  limittime?: string
  traffic_used: number
  updated_at: string
}

// —— 列表与筛选 ——
const loading = ref(false)
const rows = ref<UserRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const stats = ref({ total: 0, local: 0, external: 0, active: 0, disable: 0 })

const keyword = ref('')
const fType = ref<string | null>(null)
const fGroup = ref<string | null>(null)
const fStatus = ref<string | null>(null)
const fExpiry = ref<string | null>(null)

const groupNames = ref<string[]>([])
const policyList = ref<{ id: number; name: string }[]>([])

const typeOptions = USER_TYPES
const groupOptions = computed(() => groupNames.value.map(g => ({ label: g, value: g })))
const statusOptions = [{ label: '启用', value: '1' }, { label: '停用', value: '0' }]
const expiryOptions = [{ label: '未过期', value: 'valid' }, { label: '已过期', value: 'expired' }]

async function load(p = page.value) {
  page.value = p
  loading.value = true
  try {
    const d = await get('/api/v1/user/list', {
      page: p,
      prefix: keyword.value.trim(),
      type: fType.value || '',
      group: fGroup.value || '',
      status: fStatus.value || '',
      expiry: fExpiry.value || '',
      page_size: pageSize.value,
    })
    rows.value = d?.datas || []
    total.value = d?.count || 0
    // 统计按当前筛选条件全量聚合（后端计算），不受分页影响
    stats.value = {
      total: d?.stats_total || 0,
      local: d?.stats_local || 0,
      external: d?.stats_external || 0,
      active: d?.stats_active || 0,
      disable: d?.stats_disable || 0,
    }
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
const search = () => load(1)
function changePageSize(s: number) {
  pageSize.value = s
  load(1)
}

async function loadMeta() {
  get('/api/v1/group/names').then(d => { groupNames.value = d?.datas || [] }).catch(e => ui.message.error(errMsg(e)))
  get('/api/v1/policy/names').then(d => { policyList.value = d?.datas || [] }).catch(() => {})
}

// —— 状态 ——
const isExpired = (r: UserRow) => r.status === 1 && !!r.limittime && new Date(r.limittime) < new Date()
function statusOf(r: UserRow): { tone: 'success' | 'warning' | 'error'; label: string } {
  if (r.status === 0) return { tone: 'error', label: '停用' }
  if (isExpired(r)) return { tone: 'warning', label: '已过期' }
  return { tone: 'success', label: '启用' }
}

// —— 勾选与批量 ——
const checked = ref<DataTableRowKey[]>([])

async function batchEmail() {
  if (!checked.value.length) return
  if (!(await confirm(`确定给选中的 ${checked.value.length} 个用户发送账号邮件吗？`, '批量发送邮件', false))) return
  try {
    await post('/api/v1/user/batch/send_email', { user_ids: checked.value })
    ui.message.success('批量发送邮件成功')
  } catch (e) {
    ui.message.error(errMsg(e, '批量发送邮件失败'))
  }
}

async function batchDelete() {
  if (!checked.value.length) return
  if (!(await confirm(`确定删除选中的 ${checked.value.length} 个用户吗？此操作不可恢复。`, '批量删除'))) return
  try {
    await post('/api/v1/user/batch/delete', { user_ids: checked.value })
    ui.message.success('批量删除成功')
  } catch (e) {
    // 部分成功：后端 msg 含成功 / 失败 / 跳过数量
    if (e instanceof ApiError) ui.message.warning(e.message || '批量删除失败', { duration: 4000 })
    else { ui.message.error(errMsg(e, '批量删除失败')); return }
  }
  checked.value = []
  load()
}

// —— 单行操作 ——
async function removeUser(r: UserRow) {
  if (!(await confirm(`确定删除用户「${r.username}」吗？删除后不可恢复。`, '删除用户'))) return
  try {
    await post(`/api/v1/user/del?id=${r.id}`)
    ui.message.success('已删除')
    checked.value = checked.value.filter(k => k !== r.id)
    load(1)
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

async function resetTraffic(r: UserRow) {
  if (!(await confirm(`确定重置用户「${r.username}」的流量配额计数吗？`, '重置流量配额', false))) return
  try {
    await post('/api/v1/user/reset_traffic', null, { params: { username: r.username } })
    ui.message.success('流量配额已重置')
    load()
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

// —— OTP 二维码 ——
const otp = ref({ show: false, loading: false, username: '', nickname: '', img: '' })
async function showOtp(r: UserRow) {
  otp.value = { show: true, loading: true, username: r.username, nickname: r.nickname, img: '' }
  try {
    // 后端直接返回 base64 字符串（非 JSON 包装）
    const b64 = await get<string>('/api/v1/user/otp_qr', { id: r.id, b64: '1' })
    otp.value.img = 'data:image/png;base64,' + b64
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    otp.value.loading = false
  }
}

// —— 编辑抽屉 ——
const editShow = ref(false)
const editId = ref(0)
function openEdit(id = 0) {
  editId.value = id
  editShow.value = true
}

// —— 导入 / 模板 ——
const fileInput = ref<HTMLInputElement | null>(null)
const importing = ref(false)
async function onImport(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const fd = new FormData()
  fd.append('file', file)
  importing.value = true
  try {
    const msg = await post<string>('/api/v1/user/uploaduser', fd)
    ui.message.success(msg || '导入成功')
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    importing.value = false
    load(1)
  }
}

async function downloadTemplate() {
  try {
    const res = await http.get('/api/v1/user/uploaduser_template', { responseType: 'blob' })
    const url = URL.createObjectURL(new Blob([res.data]))
    const a = document.createElement('a')
    a.href = url
    a.download = '批量添加用户模版.xlsx'
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    ui.message.error('下载模版失败')
  }
}

// —— 表格列 ——
const rowMenu = (r: UserRow) => [
  ...(!r.disable_otp ? [{ label: '查看 OTP 二维码', key: 'otp' }] : []),
  { label: '重置流量配额', key: 'traffic' },
  { type: 'divider', key: 'd' },
  { label: () => h('span', { class: 'text-error' }, '删除用户'), key: 'delete' },
]
function onRowMenu(key: string, r: UserRow) {
  if (key === 'otp') showOtp(r)
  else if (key === 'traffic') resetTraffic(r)
  else if (key === 'delete') removeUser(r)
}

const sub = (t: string) => h('div', { class: 'truncate text-sm text-secondary' }, t)
const muted = () => h('span', { class: 'text-quaternary' }, '—')

const columns: DataTableColumns<UserRow> = [
  { type: 'selection', width: 44 },
  {
    title: '用户', key: 'username', minWidth: 200, sorter: (a, b) => a.username.localeCompare(b.username),
    render: r => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'truncate font-semibold' }, [
        r.username,
        r.nickname ? h('span', { class: 'ml-6px font-normal text-secondary' }, r.nickname) : null,
      ]),
      sub(r.email || '未设置邮箱'),
    ]),
  },
  {
    title: 'ID', key: 'id', width: 72, sorter: (a, b) => a.id - b.id,
    render: r => h('span', { class: 'rl-mono text-secondary' }, r.id),
  },
  {
    title: '类型', key: 'type', width: 88,
    render: r => h('span', { class: ['ax-badge', r.type === 'local' ? 'text-secondary' : 'text-info'] }, typeLabel(r.type)),
  },
  { title: '手机号', key: 'phone', width: 130, render: r => (r.phone ? h('span', { class: 'rl-mono' }, r.phone) : muted()) },
  {
    title: '用户组', key: 'groups', minWidth: 160,
    render: r => (r.groups?.length
      ? h('div', { class: 'flex flex-wrap gap-4px' }, r.groups.map(g => h('span', { class: 'rl-kbd' }, g)))
      : muted()),
  },
  {
    title: 'OTP', key: 'otp_secret', width: 110,
    render: r => {
      if (r.disable_otp) return h('span', { class: 'text-sm text-tertiary' }, '已禁用')
      if (!r.otp_secret) return muted()
      return h(NButton, { text: true, size: 'small', onClick: () => showOtp(r) },
        { default: () => h('span', { class: 'rl-mono' }, r.otp_secret.substring(0, 6) + '…') })
    },
  },
  {
    title: '状态', key: 'status', width: 120,
    render: r => {
      const s = statusOf(r)
      return h('div', { class: 'flex flex-col gap-2px' }, [
        h(StatusDot, { tone: s.tone }, { default: () => s.label }),
        r.change_pwd ? h('span', { class: 'text-sm text-warning' }, '需改密') : null,
      ])
    },
  },
  {
    title: '已用流量', key: 'traffic_used', width: 110, sorter: (a, b) => (a.traffic_used || 0) - (b.traffic_used || 0),
    render: r => h('span', { class: r.traffic_used > 0 ? 'tabular-nums' : 'text-quaternary' }, fmtTraffic(r.traffic_used)),
  },
  {
    title: '更新时间', key: 'updated_at', width: 160,
    render: r => h('span', { class: 'text-secondary tabular-nums' }, formatDate(r.updated_at)),
  },
  {
    title: '', key: 'actions', width: 96, align: 'right',
    render: r => h('div', { class: 'flex items-center justify-end gap-4px' }, [
      h(NButton, { text: true, size: 'small', onClick: () => openEdit(r.id) }, { default: () => '编辑' }),
      h(NDropdown, { trigger: 'click', options: rowMenu(r) as any, onSelect: (k: string) => onRowMenu(k, r) }, {
        default: () => h('button', { class: 'rl-icon-btn' }, h(NIcon, null, { default: () => h(MoreHorizontal) })),
      }),
    ]),
  },
]

onMounted(() => {
  loadMeta()
  load(1)
})
</script>

<template>
  <div>
    <PageHeader title="用户" description="管理 VPN 账号、所属用户组、OTP 双因素与有效期。外部身份源用户的资料由对应身份源维护。">
      <NButton @click="downloadTemplate">
        <template #icon><NIcon><ArrowDownload /></NIcon></template>
        下载模版
      </NButton>
      <NButton :loading="importing" @click="fileInput?.click()">
        <template #icon><NIcon><ArrowUpload /></NIcon></template>
        批量导入
      </NButton>
      <input ref="fileInput" type="file" accept=".xlsx,.xls" class="hidden" @change="onImport">
      <NButton type="primary" @click="openEdit()">
        <template #icon><NIcon><Add /></NIcon></template>
        新建用户
      </NButton>
    </PageHeader>

    <div class="grid gap-16px">
      <div class="grid grid-cols-2 gap-16px lg:grid-cols-4">
        <StatCard label="用户总数" :value="stats.total" hint="按当前筛选条件统计" />
        <StatCard label="本地用户" :value="stats.local" />
        <StatCard label="外部用户" :value="stats.external" hint="LDAP / RADIUS / 企微 / 飞书 / 钉钉" />
        <StatCard label="已启用" :value="stats.active" tone="success" :hint="`停用 ${stats.disable}`" />
      </div>

      <RlPanel :padded="false">
        <template #toolbar>
          <template v-if="checked.length">
            <span class="text-md font-semibold">已选 {{ checked.length }} 项</span>
            <NButton size="small" secondary @click="batchEmail">
              <template #icon><NIcon><Mail /></NIcon></template>
              发送账号邮件
            </NButton>
            <NButton size="small" secondary type="error" @click="batchDelete">
              <template #icon><NIcon><Delete /></NIcon></template>
              批量删除
            </NButton>
            <NButton size="small" text @click="checked = []">取消选择</NButton>
          </template>
          <template v-else>
            <NInput
              v-model:value="keyword" placeholder="用户名 / 姓名 / 邮箱 / 手机号" clearable class="w-240px!"
              @keydown.enter="search" @clear="search"
            >
              <template #prefix><NIcon><Search /></NIcon></template>
            </NInput>
            <NSelect v-model:value="fType" :options="typeOptions" placeholder="全部类型" clearable class="w-120px!" @update:value="search" />
            <NSelect v-model:value="fGroup" :options="groupOptions" placeholder="全部用户组" clearable filterable class="w-160px!" @update:value="search" />
            <NSelect v-model:value="fStatus" :options="statusOptions" placeholder="全部状态" clearable class="w-120px!" @update:value="search" />
            <NSelect v-model:value="fExpiry" :options="expiryOptions" placeholder="有效期" clearable class="w-120px!" @update:value="search" />
          </template>
          <div class="flex-1" />
          <span class="text-sm text-secondary">共 {{ total }} 条</span>
          <button class="rl-icon-btn" title="刷新" @click="load()"><ArrowClockwise /></button>
        </template>

        <NDataTable
          v-model:checked-row-keys="checked"
          :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: UserRow) => r.id"
          size="small" :scroll-x="1360"
        >
          <template #empty>
            <EmptyState :icon="People" title="没有匹配的用户" description="调整筛选条件，或新建 / 批量导入用户。" />
          </template>
        </NDataTable>

        <template #footer>
          <span class="text-sm text-secondary">第 {{ page }} 页</span>
          <NPagination
            :page="page" :page-size="pageSize" :item-count="total" :page-sizes="[10, 20, 50, 100, 200, 500]"
            show-size-picker show-quick-jumper @update:page="load" @update:page-size="changePageSize"
          />
        </template>
      </RlPanel>
    </div>

    <UserEditDrawer v-model:show="editShow" :user-id="editId" :group-names="groupNames" :policy-list="policyList" @saved="load(1)" />

    <NModal v-model:show="otp.show" preset="card" title="OTP 密钥" class="w-400px!" :bordered="false">
      <div class="flex flex-col items-center gap-10px text-center">
        <div class="text-md">
          <span class="font-semibold">{{ otp.username }}</span>
          <span v-if="otp.nickname" class="ml-6px text-secondary">{{ otp.nickname }}</span>
        </div>
        <NSpin :show="otp.loading">
          <div class="h-220px w-220px flex items-center justify-center rounded-8px b-1px b-solid b-[var(--color-border-default)] bg-white p-8px">
            <img v-if="otp.img" :src="otp.img" alt="OTP 二维码" class="h-full w-full">
          </div>
        </NSpin>
        <div class="text-sm text-secondary">使用 Google Authenticator 或同类应用扫描二维码</div>
      </div>
    </NModal>
  </div>
</template>
