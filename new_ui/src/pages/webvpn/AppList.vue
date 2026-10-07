<script setup lang="ts">
// WebVPN 应用：通过子域名接入内网 Web / SSH / SFTP 应用，按用户 / 用户组 / 路径 / 来源 IP 授权
import { h, onMounted, ref, type VNodeChild } from 'vue'
import { NButton, NDataTable, NInput, NPagination, NTooltip, type DataTableColumns } from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import Globe from '~icons/fluent/globe-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import { get, post, errMsg } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import AppDrawer from './components/AppDrawer.vue'
import { CRED_MODE_LABEL, accessModeOf, accessOf, appPayload, pathAddr, protocolDef, protocolOf, subdomainAddr, type CredMode, type WebVpnApp } from './components/app'

const loading = ref(false)
const keyword = ref('')
const rows = ref<WebVpnApp[]>([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const domain = ref('')
const allUsers = ref<string[]>([])
const allGroups = ref<string[]>([])

const drawerShow = ref(false)
const editing = ref<WebVpnApp | null>(null)

async function load() {
  loading.value = true
  try {
    const d = await get('/api/v1/webvpn/app/list', { page: page.value, page_size: pageSize.value, name: keyword.value })
    rows.value = d?.datas || []
    total.value = d?.count || 0
  } catch (e) {
    ui.message.error(errMsg(e, '获取列表失败'))
  } finally {
    loading.value = false
  }
}
function search() { page.value = 1; load() }

// 下拉候选：WebVPN 域名、用户、用户组（失败静默）
function loadMeta() {
  get('/api/v1/webvpn/domain').then(d => { domain.value = d?.domain || '' }).catch(() => {})
  get('/api/v1/group/list', { page_size: 200 }).then(d => { allGroups.value = (d?.datas || []).map((g: any) => g.name) }).catch(() => {})
  get('/api/v1/user/list', { page_size: 200 }).then(d => { allUsers.value = (d?.datas || []).map((u: any) => u.username) }).catch(() => {})
}

onMounted(() => { loadMeta(); load() })

function openEdit(row: WebVpnApp | null) {
  editing.value = row
  drawerShow.value = true
}

async function toggle(row: WebVpnApp) {
  const next = row.status === 1 ? 0 : 1
  try {
    await post('/api/v1/webvpn/app/set', appPayload({ ...row, status: next }))
    ui.message.success(next === 1 ? '已启用' : '已禁用')
    load()
  } catch (e) {
    ui.message.error(errMsg(e, '操作失败'))
  }
}

async function remove(row: WebVpnApp) {
  if (!(await confirm(`确认删除应用「${row.note || row.name}」？`, '删除应用'))) return
  try {
    await post('/api/v1/webvpn/app/del', { id: row.id })
    ui.message.success('已删除')
    if (rows.value.length === 1 && page.value > 1) page.value--
    load()
  } catch (e) {
    ui.message.error(errMsg(e, '删除失败'))
  }
}

// 授权名单：最多显示 3 个，其余折叠到 +N（悬停查看全部）
function chips(list?: string[]): VNodeChild {
  if (!list?.length) return h('span', { class: 'text-tertiary' }, '全部用户')
  const shown = list.slice(0, 3)
  const nodes = shown.map(v => h('span', { class: 'rl-kbd text-primary!' }, v))
  if (list.length > 3) {
    nodes.push(h(NTooltip, null, {
      trigger: () => h('span', { class: 'rl-kbd cursor-default' }, `+${list.length - 3}`),
      default: () => list.join('、'),
    }))
  }
  return h('div', { class: 'flex flex-wrap gap-4px' }, nodes)
}

// 额外限制摘要：路径 / 来源 IP / 跨站
function limits(r: WebVpnApp): VNodeChild {
  const items: string[] = []
  if (r.allow_path?.length) items.push(`路径 ${r.allow_path.length}`)
  if (r.ip_allow_list?.length) items.push(`来源 IP ${r.ip_allow_list.length}`)
  if (r.allow_cross_site) items.push(`跨站 ${r.cors_allowed_origins?.length || 0}`)
  if (!items.length) return h('span', { class: 'text-tertiary' }, '—')
  const tip = [
    r.allow_path?.length ? `允许路径：${r.allow_path.join(', ')}` : '',
    r.ip_allow_list?.length ? `来源 IP：${r.ip_allow_list.join(', ')}` : '',
    r.allow_cross_site ? `跨域来源：${(r.cors_allowed_origins || []).join(', ') || '仅同源'}` : '',
  ].filter(Boolean)
  return h(NTooltip, null, {
    trigger: () => h('span', { class: 'cursor-default text-secondary' }, items.join(' · ')),
    default: () => tip.map(t => h('div', null, t)),
  })
}

// 协议图标 + 徽标
function protoIcon(r: WebVpnApp): VNodeChild {
  const d = protocolDef(protocolOf(r))
  return h(NTooltip, null, {
    trigger: () => h('span', { class: 'relative flex h-32px w-32px flex-shrink-0 items-center justify-center rounded-8px bg-fill text-lg text-secondary' }, [h(d.icon)]),
    default: () => d.label,
  })
}

// 非 HTTP 应用的目标：协议徽标 + user@host:port + 凭据模式 / 录像
function remoteTarget(r: WebVpnApp): VNodeChild {
  const p = protocolOf(r)
  const port = r.target_port || protocolDef(p).port
  const user = r.cred_mode !== 'passthrough' && r.cred_user ? r.cred_user + '@' : ''
  return h('div', { class: 'min-w-0' }, [
    h('div', { class: 'flex min-w-0 items-center gap-6px' }, [
      h('span', { class: 'ax-badge text-info h-18px! px-6px! flex-shrink-0' }, p.toUpperCase()),
      h('span', { class: 'rl-mono truncate', title: `${user}${r.target_host}:${port}` }, `${user}${r.target_host}:${port}`),
    ]),
    h('div', { class: 'flex items-center gap-6px text-sm text-secondary' }, [
      h('span', null, CRED_MODE_LABEL[(r.cred_mode || 'prompt') as CredMode]),
      r.record ? h('span', { class: 'ax-badge text-error h-18px! px-6px!' }, '录像') : null,
      !r.host_key_fp && (p === 'ssh' || p === 'sftp') ? h('span', { class: 'text-tertiary' }, '· 指纹待记录') : null,
    ]),
  ])
}

// 访问地址：子域名（前置）与路径（后置，位于 VPN 主域名下）
function accessCell(r: WebVpnApp): VNodeChild {
  const { sub, path } = accessOf(r, domain.value)
  const line = (label: string, addr: string) => h('div', { class: 'flex min-w-0 items-center gap-6px' }, [
    h('span', { class: 'ax-badge text-secondary h-18px! px-6px! flex-shrink-0' }, label),
    h('span', { class: 'rl-mono truncate text-sm!', title: addr }, addr),
  ])
  const rows: VNodeChild[] = []
  if (sub) rows.push(line('子域名', subdomainAddr(r.name, domain.value)))
  if (path) rows.push(line('路径', pathAddr(r.name, protocolOf(r))))
  if (!rows.length) {
    return h('span', { class: 'text-sm text-warning' }, accessModeOf(r) === 'subdomain' && !domain.value ? '未配置 WebVPN 域名，暂不可访问' : '—')
  }
  return h('div', { class: 'grid gap-2px' }, rows)
}

const columns: DataTableColumns<WebVpnApp> = [
  {
    title: '应用', key: 'note', minWidth: 220,
    render: r => h('div', { class: 'flex min-w-0 items-center gap-10px' }, [
      protoIcon(r),
      h('div', { class: 'min-w-0' }, [
      h('div', { class: 'truncate font-semibold' }, r.note || r.name),
      h('div', { class: 'truncate text-sm text-secondary' }, [
        h('span', { class: 'rl-mono text-sm! text-primary' }, r.name),
        h('span', { class: 'ml-6px text-tertiary' }, `#${r.id}`),
      ]),
      ]),
    ]),
  },
  { title: '访问地址', key: 'access', minWidth: 240, render: r => accessCell(r) },
  {
    title: '目标', key: 'backend', minWidth: 240,
    render: r => protocolOf(r) !== 'http' ? remoteTarget(r) : h('div', { class: 'min-w-0' }, [
      h('div', { class: 'rl-mono truncate', title: r.backend }, r.backend),
      (r.host_rewrite || r.skip_verify)
        ? h('div', { class: 'flex items-center gap-6px text-sm text-secondary' }, [
            r.host_rewrite ? h('span', { class: 'truncate' }, ['Host → ', h('span', { class: 'rl-mono text-sm!' }, r.host_rewrite)]) : null,
            r.skip_verify ? h('span', { class: 'ax-badge text-warning h-18px! px-6px!' }, '跳过证书校验') : null,
          ])
        : null,
    ]),
  },
  { title: '授权用户', key: 'users', minWidth: 160, render: r => chips(r.users) },
  { title: '授权用户组', key: 'groups', minWidth: 160, render: r => chips(r.groups) },
  { title: '访问限制', key: 'limits', width: 150, render: r => limits(r) },
  {
    title: '状态', key: 'status', width: 90,
    render: r => h(StatusDot, { tone: r.status === 1 ? 'success' : 'muted' }, () => (r.status === 1 ? '启用' : '禁用')),
  },
  {
    title: '', key: 'actions', width: 160, align: 'right',
    render: r => h('div', { class: 'flex justify-end gap-12px' }, [
      h(NButton, { text: true, size: 'small', onClick: () => openEdit(r) }, () => '编辑'),
      h(NButton, { text: true, size: 'small', onClick: () => toggle(r) }, () => (r.status === 1 ? '禁用' : '启用')),
      h(NButton, { text: true, size: 'small', type: 'error', onClick: () => remove(r) }, () => '删除'),
    ]),
  },
]
</script>

<template>
  <div>
    <PageHeader title="WebVPN 应用" description="通过子域名或 VPN 主域名下的路径，让用户在浏览器中访问内网 Web 应用、终端、文件与远程桌面，无需安装客户端">
      <template #meta>
        <span v-if="domain" class="rl-kbd" title="WebVPN 域名">*.{{ domain }}</span>
      </template>
      <NButton type="primary" @click="openEdit(null)">
        <template #icon><Add /></template>添加应用
      </NButton>
    </PageHeader>

    <RlPanel :padded="false">
      <template #toolbar>
        <NInput v-model:value="keyword" placeholder="搜索应用名称 / 子域名" clearable class="w-260px!" @keyup.enter="search" @clear="search">
          <template #prefix><Search /></template>
        </NInput>
        <div class="flex-1" />
        <span class="text-sm text-secondary">共 {{ total }} 个</span>
        <button class="rl-icon-btn" title="刷新" @click="load"><ArrowClockwise /></button>
      </template>
      <NDataTable
        :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: WebVpnApp) => r.id"
        size="small" :scroll-x="1180"
      >
        <template #empty>
          <EmptyState :icon="Globe" :title="keyword ? '没有匹配的应用' : '还没有 WebVPN 应用'" :description="keyword ? '换个关键词试试。' : '添加内网 Web 应用后，授权用户可在门户中直接访问。'">
            <NButton v-if="!keyword" @click="openEdit(null)">添加应用</NButton>
          </EmptyState>
        </template>
      </NDataTable>
      <template v-if="total > 10" #footer>
        <span />
        <NPagination
          v-model:page="page" v-model:page-size="pageSize" :item-count="total" show-size-picker :page-sizes="[10, 20, 50]"
          @update:page="load" @update:page-size="search"
        />
      </template>
    </RlPanel>

    <AppDrawer v-model:show="drawerShow" :app="editing" :domain="domain" :all-users="allUsers" :all-groups="allGroups" @saved="load" />
  </div>
</template>
