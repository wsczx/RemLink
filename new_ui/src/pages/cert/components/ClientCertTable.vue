<script setup lang="ts">
// 客户端证书列表：状态筛选 / 搜索 / 下载 / 邮件 / 启停 / 续期 / 删除 / 设备绑定 / 最大设备数 / 批量操作
import { computed, h, onMounted, ref } from 'vue'
import {
  type DataTableColumns, type DataTableRowKey, NButton, NDataTable, NDropdown, NInput, NModal, NPagination, NSwitch, NTooltip,
} from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import MoreHorizontal from '~icons/fluent/more-horizontal-20-regular'
import LinkDismiss from '~icons/fluent/link-dismiss-20-regular'
import Edit from '~icons/fluent/edit-20-regular'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import Certificate from '~icons/fluent/certificate-20-regular'
import CertPrompt from './CertPrompt.vue'
import { type CertStats as Stats, type CertUser, type ClientCert, saveBlob, userLabel } from './client'
import { daysLeft, expiryText, expiryTone } from './pem'
import { errMsg, get, http, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'

const props = defineProps<{ users: CertUser[] }>()
const emit = defineEmits<{ stats: [s: Stats] }>()
const nickOf = (u: string) => props.users.find(x => x.username === u)?.nickname

// —— 列表 ——
const list = ref<ClientCert[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const q = ref({ username: '', groupname: '', status: null as number | null })
// 各状态数量（page_size=1 只取 total）
const stats = ref<Stats>({ all: 0, ok: 0, disabled: 0, expired: 0 })
async function loadStats() {
  const count = (status?: number) => get('/api/v1/set/client_cert/list', { page_size: 1, page_index: 1, ...(status != null ? { status } : {}) }).then(d => d?.total || 0).catch(() => 0)
  const [all, ok, disabled, expired] = await Promise.all([count(), count(0), count(1), count(2)])
  stats.value = { all, ok, disabled, expired }
  emit('stats', stats.value)
}
const chips = computed(() => [
  { label: '全部', value: null, n: stats.value.all },
  { label: '启用', value: 0, n: stats.value.ok },
  { label: '禁用', value: 1, n: stats.value.disabled },
  { label: '已过期', value: 2, n: stats.value.expired },
])
function setStatus(v: number | null) { q.value.status = v; search() }
const STATUS: Record<number, { text: string; tone: 'success' | 'warning' | 'error' }> = {
  0: { text: '启用', tone: 'success' }, 1: { text: '禁用', tone: 'warning' }, 2: { text: '过期', tone: 'error' },
}

async function load() {
  loading.value = true
  try {
    const params: Record<string, any> = { page_size: pageSize.value, page_index: page.value }
    if (q.value.username) params.username = q.value.username
    if (q.value.groupname) params.groupname = q.value.groupname
    if (q.value.status != null) params.status = q.value.status
    const d = await get('/api/v1/set/client_cert/list', params)
    list.value = d?.list || []
    total.value = d?.total || 0
    loadStats()
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
function search() { page.value = 1; load() }
function resetSearch() { q.value = { username: '', groupname: '', status: null }; search() }

// —— 选择 ——
const rowKey = (r: ClientCert) => `${r.username}\u0000${r.groupname}`
const checked = ref<DataTableRowKey[]>([])
const selected = computed(() => list.value.filter(r => checked.value.includes(rowKey(r))))
const pairs = (rows: ClientCert[]) => rows.map(r => ({ username: r.username, groupname: r.groupname }))

// 表单提交（后端按 multipart/form 读取）
function form(row: ClientCert, extra: Record<string, string> = {}) {
  const fd = new FormData()
  fd.append('username', row.username)
  fd.append('groupname', row.groupname)
  for (const [k, v] of Object.entries(extra)) fd.append(k, v)
  return fd
}
async function act(url: string, fd: FormData, okMsg: string) {
  try {
    await post(url, fd)
    ui.message.success(okMsg)
    load()
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

async function toggleStatus(r: ClientCert) {
  const a = r.status === 0 ? '禁用' : '启用'
  if (await confirm(`确定要${a}用户 ${r.username} 的证书吗？`, `${a}证书`, r.status === 0)) act('/api/v1/set/client_cert/changecertstatus', form(r), `证书${a}成功`)
}
async function renew(r: ClientCert) {
  if (!(await confirm(`确定要为 ${r.username}（组：${r.groupname}）续期证书吗？旧证书将被替换。`, '续期证书', false))) return
  const fd = new FormData()
  fd.append('username', r.username)
  fd.append('group_name', r.groupname)
  act('/api/v1/set/client_cert/renew', fd, '证书续期成功')
}
async function remove(r: ClientCert) {
  if (await confirm(`确定要删除用户 ${r.username} 的证书吗？`, '删除证书')) act('/api/v1/set/client_cert/delete', form(r), '删除成功')
}
async function unbind(r: ClientCert, id: string) {
  if (await confirm(`确定要解绑设备「${id}」吗？`, '解绑设备')) act('/api/v1/set/client_cert/unbind_device', form(r, { device_id: id }), '设备解绑成功')
}
async function toggleBinding(r: ClientCert) {
  const on = !r.device_binding_enabled
  const msg = on ? '开启设备绑定后，仅 Cisco AnyConnect 客户端支持该证书，确定要开启吗？' : '关闭设备绑定将清空所有已绑定的设备，确定要关闭吗？'
  if (await confirm(msg, on ? '开启设备绑定' : '关闭设备绑定'))
    act('/api/v1/set/client_cert/update_device_binding', form(r, { device_binding_enabled: String(on) }), `设备绑定${on ? '开启' : '关闭'}成功`)
}

// —— 弹窗：下载密码 / 最大设备数 / 批量下载 ——
const prompt = ref<{ kind: 'download' | 'max' | 'batch'; row?: ClientCert } | null>(null)
const promptShow = ref(false)
const promptBusy = ref(false)
function openPrompt(kind: 'download' | 'max' | 'batch', row?: ClientCert) {
  prompt.value = { kind, row }
  promptShow.value = true
}
async function onPrompt(v: string | number) {
  const p = prompt.value
  if (!p) return
  promptBusy.value = true
  try {
    if (p.kind === 'download' && p.row) await download(p.row, String(v))
    else if (p.kind === 'batch') await batchDownload(String(v))
    else if (p.kind === 'max' && p.row) {
      await post('/api/v1/set/client_cert/update_max_devices', form(p.row, { max_devices: String(v) }))
      ui.message.success('修改成功')
      load()
    }
    promptShow.value = false
  } catch (e) {
    ui.message.error(e instanceof Error && !(e as any).response ? e.message : errMsg(e))
  } finally {
    promptBusy.value = false
  }
}

async function download(r: ClientCert, password: string) {
  const resp = await http.get('/api/v1/set/client_cert/download', {
    params: { username: r.username, groupname: r.groupname, password },
    responseType: 'blob',
  })
  const ct = resp.headers['content-type'] as string | undefined
  await saveBlob(resp.data, ct, ct?.includes('application/x-pem-file') ? `${r.username}.cer` : `${r.username}.p12`)
  ui.message.success('证书下载成功')
}
function onDownload(r: ClientCert) {
  // CSR 签发的证书没有私钥，直接下载 PEM
  if (r.is_csr_based) download(r, '').catch(e => ui.message.error(e?.message || '下载失败'))
  else openPrompt('download', r)
}
async function batchDownload(password: string) {
  const resp = await http.post('/api/v1/set/client_cert/batch_download', { certs: pairs(selected.value), password }, { responseType: 'blob' })
  await saveBlob(resp.data, resp.headers['content-type'] as string | undefined || 'application/zip', 'certificates.zip')
  ui.message.success('证书下载成功')
  checked.value = []
}

async function batchDelete() {
  const rows = selected.value
  if (!rows.length) return
  const names = rows.map(r => `${r.username}/${r.groupname}`).join('、')
  if (!(await confirm(`确定要删除以下 ${rows.length} 个证书吗？\n${names}`, '批量删除证书'))) return
  try {
    await post('/api/v1/set/client_cert/batch_delete', { certs: pairs(rows) })
    ui.message.success('删除成功')
    checked.value = []
    load()
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

// —— 发送邮件 ——
const mailShow = ref(false)
const mailCerts = ref<{ username: string; groupname: string }[]>([])
const mailPwd = ref('')
const mailBusy = ref(false)
function openMail(rows: ClientCert[]) {
  if (!rows.length) return ui.message.warning('请先选择要发送邮件的证书记录')
  mailCerts.value = pairs(rows)
  mailPwd.value = ''
  mailShow.value = true
}
async function sendMail() {
  mailBusy.value = true
  try {
    await post('/api/v1/set/client_cert/send_mail', { certs: mailCerts.value, password: mailPwd.value || '' })
    ui.message.success('发送成功')
    mailShow.value = false
    checked.value = []
  } catch (e) {
    ui.message.error(errMsg(e, '请求失败'))
  } finally {
    mailBusy.value = false
  }
}

// —— 表格列 ——
const rowMenu = (r: ClientCert) => [
  { label: '发送邮件', key: 'mail' },
  { label: r.status === 0 ? '禁用' : '启用', key: 'toggle', disabled: r.status === 2 },
  { label: '续期', key: 'renew', disabled: r.status === 2 },
  { type: 'divider', key: 'd' },
  { label: () => h('span', { class: 'text-error' }, '删除'), key: 'delete' },
]
function onMenu(key: string, r: ClientCert) {
  if (key === 'mail') openMail([r])
  else if (key === 'toggle') toggleStatus(r)
  else if (key === 'renew') renew(r)
  else if (key === 'delete') remove(r)
}

const columns = computed<DataTableColumns<ClientCert>>(() => [
  { type: 'selection', width: 44 },
  {
    title: '用户', key: 'username', minWidth: 180,
    sorter: (a, b) => a.username.localeCompare(b.username),
    render: r => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'truncate font-semibold' }, userLabel(r.username, nickOf(r.username))),
      h('div', { class: 'truncate text-sm text-secondary' }, r.groupname || '—'),
    ]),
  },
  {
    title: '设备绑定', key: 'device_binding_enabled', width: 96,
    render: r => h(NSwitch, { size: 'small', value: r.device_binding_enabled, onUpdateValue: () => toggleBinding(r) }),
  },
  {
    title: '已绑定设备', key: 'device_id', minWidth: 220,
    render: r => {
      const ids = r.device_id || []
      if (!ids.length) return h('span', { class: 'text-tertiary' }, '未绑定')
      return h('div', { class: 'flex flex-col gap-2px' }, ids.map(id => h('div', { class: 'flex min-w-0 items-center gap-4px' }, [
        h('span', { class: 'rl-mono min-w-0 truncate text-sm', title: id }, id),
        h(NTooltip, null, {
          trigger: () => h('button', { class: 'rl-icon-btn h-24px! w-24px! flex-shrink-0 text-md!', onClick: () => unbind(r, id) }, h(LinkDismiss)),
          default: () => '解绑此设备',
        }),
      ])))
    },
  },
  {
    title: '最大设备', key: 'max_devices', width: 96,
    render: r => h('div', { class: 'flex items-center gap-4px' }, [
      h('span', null, r.max_devices),
      h('button', { class: 'rl-icon-btn h-24px! w-24px! text-md!', title: '修改最大设备数', onClick: () => openPrompt('max', r) }, h(Edit)),
    ]),
  },
  {
    title: '创建时间', key: 'created_at', width: 168,
    sorter: (a, b) => +new Date(a.created_at) - +new Date(b.created_at),
    render: r => formatDate(r.created_at),
  },
  {
    title: '到期', key: 'not_after', width: 180,
    sorter: (a, b) => +new Date(a.not_after) - +new Date(b.not_after),
    render: r => {
      const d = daysLeft(r.not_after)
      return h('div', null, [
        h('div', null, formatDate(r.not_after, 'YYYY-MM-DD HH:mm')),
        h('div', { class: 'text-sm' }, h(StatusDot, { tone: expiryTone(d) }, () => expiryText(d))),
      ])
    },
  },
  {
    title: '状态', key: 'status', width: 84,
    render: r => h(StatusDot, { tone: STATUS[r.status]?.tone || 'muted' }, () => STATUS[r.status]?.text || '未知'),
  },
  {
    title: '', key: 'ops', width: 110, align: 'right',
    render: r => h('div', { class: 'flex items-center justify-end gap-4px' }, [
      h(NButton, { text: true, onClick: () => onDownload(r) }, () => '下载'),
      h(NDropdown, { trigger: 'click', options: rowMenu(r) as any, onSelect: (k: string) => onMenu(k, r) }, () =>
        h('button', { class: 'rl-icon-btn' }, h(MoreHorizontal))),
    ]),
  },
])

const promptProps = computed(() => {
  const k = prompt.value?.kind
  if (k === 'max') return { title: '修改最大设备数', label: '新的最大设备数', kind: 'number' as const, initial: prompt.value?.row?.max_devices || 1, confirmText: '保存' }
  if (k === 'batch') return { title: `批量下载证书（${selected.value.length} 个）`, label: 'P12 证书密码', kind: 'password' as const, placeholder: '留空则不设置密码', confirmText: '下载' }
  return { title: `下载证书 · ${prompt.value?.row?.username || ''}`, label: '证书密码', kind: 'password' as const, placeholder: '留空则不使用密码', confirmText: '下载' }
})

onMounted(load)
defineExpose({ reload: load })
</script>

<template>
  <RlPanel title="已签发证书" :padded="false">
    <template #extra>
      <div class="flex flex-wrap items-center gap-4px">
        <button
          v-for="c in chips" :key="String(c.value)" type="button" class="cl-chip" :class="q.status === c.value && 'cl-chip--on'"
          @click="setStatus(c.value)"
        >
          {{ c.label }}<span class="cl-chip__n">{{ c.n }}</span>
        </button>
      </div>
    </template>

    <template #toolbar>
      <template v-if="checked.length">
        <span class="text-md font-semibold">已选 {{ checked.length }} 项</span>
        <NButton size="small" @click="openMail(selected)">发送邮件</NButton>
        <NButton size="small" @click="openPrompt('batch')">批量下载</NButton>
        <NButton size="small" type="error" secondary @click="batchDelete">批量删除</NButton>
        <NButton size="small" text class="ml-xs" @click="checked = []">取消选择</NButton>
      </template>
      <template v-else>
        <NInput v-model:value="q.username" placeholder="用户名" clearable class="w-180px!" @keyup.enter="search" @clear="search">
          <template #prefix><Search class="text-secondary" /></template>
        </NInput>
        <NInput v-model:value="q.groupname" placeholder="用户组" clearable class="w-160px!" @keyup.enter="search" @clear="search" />
        <NButton @click="search">搜索</NButton>
        <NButton text @click="resetSearch">重置</NButton>
      </template>
      <div class="flex-1" />
      <span class="text-sm text-secondary">共 {{ total }} 条</span>
      <button class="rl-icon-btn" title="刷新" @click="load"><ArrowClockwise /></button>
    </template>

    <NDataTable
      v-model:checked-row-keys="checked"
      :columns="columns" :data="list" :loading="loading" :bordered="false" :row-key="rowKey"
      size="small" :scroll-x="1180"
    >
      <template #empty>
        <EmptyState :icon="Certificate" :title="q.username || q.groupname || q.status != null ? '没有匹配的证书' : '还没有签发客户端证书'" :description="q.username || q.groupname || q.status != null ? '调整筛选条件后重试' : '点击右上角「签发证书」为用户签发，或按用户组批量签发'" />
      </template>
    </NDataTable>

    <template #footer>
      <span />
      <NPagination
        v-model:page="page" v-model:page-size="pageSize" :item-count="total" :page-sizes="[10, 20, 50, 100]" show-size-picker
        @update:page="load" @update:page-size="() => { page = 1; load() }"
      />
    </template>
  </RlPanel>

  <CertPrompt v-model:show="promptShow" v-bind="promptProps" :loading="promptBusy" @confirm="onPrompt" />

  <!-- 发送证书邮件 -->
  <NModal v-model:show="mailShow" preset="card" title="发送证书邮件" class="w-480px! max-w-[calc(100vw-32px)]" :bordered="false">
    <div class="mb-6px text-sm font-semibold">P12 密码</div>
    <NInput v-model:value="mailPwd" type="password" show-password-on="click" placeholder="留空则不设置密码" :input-props="{ autocomplete: 'new-password' }" />
    <div class="mt-4px text-sm text-secondary">安装证书时需要输入的密码，留空允许无密码安装。</div>
    <div class="mb-6px mt-16px text-sm font-semibold">发送对象（{{ mailCerts.length }}）</div>
    <div class="flex max-h-160px flex-wrap gap-4px overflow-auto">
      <span v-for="(c, i) in mailCerts" :key="i" class="rl-kbd">{{ c.username }} / {{ c.groupname }}</span>
    </div>
    <template #footer>
      <div class="flex justify-end gap-sm">
        <NButton @click="mailShow = false">取消</NButton>
        <NButton type="primary" :loading="mailBusy" @click="sendMail">确认发送</NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.cl-chip { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; border: 1px solid transparent; border-radius: 14px; background: transparent; color: var(--color-text-secondary); font: inherit; font-size: 13px; cursor: pointer; }
.cl-chip:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.cl-chip--on { border-color: var(--color-border-default); background: var(--color-secondary); color: var(--color-text-primary); font-weight: 600; }
.cl-chip__n { font-size: 12px; color: var(--color-text-tertiary); font-variant-numeric: tabular-nums; }
</style>
