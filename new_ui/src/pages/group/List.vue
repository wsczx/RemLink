<script setup lang="ts">
// 用户组：引用策略、内网域名（分割 DNS）、独立 IP 段与出网网卡、多步认证流程
import { computed, h, nextTick, onMounted, ref } from 'vue'
import {
  NButton, NDataTable, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NPagination, NRadioButton,
  NRadioGroup, NSelect, NSpin, NSwitch, NTabPane, NTabs, NTooltip,
  type DataTableColumns, type FormInst, type FormItemRule, type FormRules, type FormValidationError,
} from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ChevronRight from '~icons/fluent/chevron-right-12-regular'
import PeopleTeam from '~icons/fluent/people-team-20-regular'
import Checkmark from '~icons/fluent/checkmark-circle-20-regular'
import ShieldTask from '~icons/fluent/shield-task-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import { get, post, errMsg } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'
import FormSection from '@/pages/policy/components/FormSection.vue'
import ValRowsEditor from '@/pages/policy/components/ValRowsEditor.vue'
import { isValidCIDR6, type ValData } from '@/pages/policy/components/policy'
import AuthFlowEditor from './components/AuthFlowEditor.vue'
import QuickPolicyDrawer from './components/QuickPolicyDrawer.vue'
import { AUTH_LABELS, authConflict, newStep, normalizeStep, type AuthStep } from './components/auth'

interface GroupForm {
  id?: number
  name: string
  note: string
  policy_id: number
  status: number
  hidden: number
  split_dns: ValData[]
  auth_profile: { step: AuthStep[] }
  client_cidr: string
  client_start: string
  client_end: string
  client_gateway: string
  client_cidr6: string
  out_dev: string
}

const PAGE_SIZE = 10
const blank = (): GroupForm => ({
  id: undefined, name: '', note: '', policy_id: 0, status: 1, hidden: 0, split_dns: [],
  auth_profile: { step: [newStep('local')] },
  client_cidr: '', client_start: '', client_end: '', client_gateway: '', client_cidr6: '', out_dev: '',
})

// —— 下拉数据 ——
const policyList = ref<{ id: number; name: string }[]>([])   // 启用中的策略（供选择）
const allPolicyNames = ref<{ id: number; name: string }[]>([]) // 全量（含停用，列表展示用）
const ifaces = ref<string[]>([])

async function loadPolicyList() {
  try { policyList.value = (await get('/api/v1/policy/names'))?.datas || [] } catch { /* 忽略 */ }
}
async function loadAllPolicyNames() {
  try { allPolicyNames.value = (await get('/api/v1/policy/all_names'))?.datas || [] } catch { /* 忽略 */ }
}
async function loadIfaces() {
  try { ifaces.value = (await get('/api/v1/group/ifaces'))?.ifaces || [] } catch { /* 忽略 */ }
}

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
    const d = await get('/api/v1/group/list', { page: p })
    rows.value = d?.datas || []
    count.value = d?.count || 0
    stats.value = d || {}
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

const policyName = (id: number) => allPolicyNames.value.find(p => p.id === id)?.name || ''

const columns = computed<DataTableColumns<any>>(() => [
  {
    title: '用户组', key: 'name', minWidth: 200,
    render: r => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'flex items-center gap-6px' }, [
        h('span', { class: 'truncate font-semibold' }, r.name),
        r.hidden ? h(NTooltip, null, { trigger: () => h('span', { class: 'ax-badge text-secondary' }, '隐藏'), default: () => '不出现在客户端的组选择列表中' }) : null,
      ]),
      h('div', { class: 'truncate text-sm text-secondary' }, r.note || `ID ${r.id}`),
    ]),
  },
  {
    title: '策略', key: 'policy_id', minWidth: 140,
    render: r => {
      if (!r.policy_id) return h('span', { class: 'ax-badge text-warning' }, '待配置')
      const n = policyName(r.policy_id)
      return n ? h('span', n) : h('span', { class: 'text-tertiary' }, '-')
    },
  },
  {
    title: '认证方式', key: 'auth', minWidth: 220,
    render: r => {
      const steps: any[] = r.auth_profile?.step || []
      if (!steps.length) return h('span', { class: 'text-secondary' }, '本地密码')
      const parts: any[] = []
      steps.forEach((s, i) => {
        parts.push(h('span', { class: 'rl-kbd' }, AUTH_LABELS[s.type] || s.type))
        if (i < steps.length - 1) parts.push(h(ChevronRight, { class: 'text-tertiary' }))
      })
      return h('div', { class: 'flex flex-wrap items-center gap-4px' }, parts)
    },
  },
  {
    title: '地址池', key: 'client_cidr', width: 170,
    render: r => (r.client_cidr
      ? h('div', [h('div', { class: 'rl-mono' }, r.client_cidr), r.out_dev ? h('div', { class: 'text-sm text-secondary' }, `出网 ${r.out_dev}`) : null])
      : h('span', { class: 'text-tertiary' }, '全局 IP 池')),
  },
  {
    title: '状态', key: 'status', width: 90,
    render: r => h(StatusDot, { tone: r.status === 1 ? 'success' : 'muted' }, () => (r.status === 1 ? '启用' : '停用')),
  },
  {
    title: '更新时间', key: 'updated_at', width: 170,
    render: r => h('span', { class: 'text-secondary tabular-nums' }, formatDate(r.updated_at)),
  },
  {
    title: '', key: 'ops', width: 110, fixed: 'right',
    render: r => h('div', { class: 'flex items-center justify-end gap-12px' }, [
      h(NButton, { text: true, size: 'small', onClick: () => openEdit(r) }, () => '编辑'),
      h(NButton, { text: true, size: 'small', type: 'error', onClick: () => del(r) }, () => '删除'),
    ]),
  },
])

async function del(row: any) {
  if (!(await confirm(`确定要删除用户组「${row.name}」吗？删除后不可恢复。`, '删除用户组'))) return
  try {
    await post(`/api/v1/group/del?id=${row.id}`)
    ui.message.success('删除成功')
    load(1)
  } catch (e) { ui.message.error(errMsg(e)) }
}

// —— 新建 / 编辑 ——
const drawer = ref(false)
const tab = ref('basic')
const form = ref<GroupForm>(blank())
const formRef = ref<FormInst | null>(null)
const authRef = ref<InstanceType<typeof AuthFlowEditor> | null>(null)
const enableGroupIP = ref(false)
const detailLoading = ref(false)
const saving = ref(false)
const quickPolicy = ref(false)

const isEdit = computed(() => !!form.value.id)
const conflictTip = computed(() => authConflict(form.value.auth_profile.step))
// 已停用的策略不在可选列表中，回显时标注
const policyFallback = (v: string | number) => ({ label: `${policyName(Number(v)) || '#' + v}（已停用）`, value: v })
const policyOptions = computed(() => policyList.value.map(p => ({ label: p.name, value: p.id })))
// 出网网卡：物理网卡清单 + 已保存但当前不在清单中的网卡
const outDevOptions = computed(() => {
  const list = [...ifaces.value]
  if (form.value.out_dev && !list.includes(form.value.out_dev)) list.unshift(form.value.out_dev)
  return [{ label: '留空（使用默认 master_dev）', value: '' }, ...list.map(i => ({ label: i, value: i }))]
})

function toggleGroupIP(v: boolean) {
  enableGroupIP.value = v
  if (!v) Object.assign(form.value, { client_cidr: '', client_start: '', client_end: '', client_gateway: '', client_cidr6: '', out_dev: '' })
}

const requiredIfIP = (msg: string): FormItemRule => ({
  trigger: 'blur',
  validator: (_r, v: string) => (!enableGroupIP.value || v ? true : new Error(msg)),
})
const rules: FormRules = {
  name: [
    { required: true, message: '请输入组名', trigger: 'blur' },
    { max: 30, message: '长度不超过 30 个字符', trigger: 'blur' },
  ],
  policy_id: {
    trigger: 'change',
    validator: (_r, v: number) => (form.value.status === 1 && (!v || v <= 0) ? new Error('启用的用户组必须指定策略') : true),
  },
  client_cidr: [
    requiredIfIP('请输入网段'),
    {
      trigger: 'blur',
      validator: (_r, v: string) => {
        if (!v) return true
        const m = v.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\/(\d{1,2})$/)
        if (!m) return new Error('IPv4 网段格式无效（如 10.0.1.0/24）')
        if ([m[1], m[2], m[3], m[4]].map(Number).some(o => o > 255)) return new Error('IPv4 地址每段须为 0-255')
        if (Number(m[5]) > 32) return new Error('IPv4 前缀须为 0-32')
        return true
      },
    },
  ],
  client_gateway: requiredIfIP('请输入网关地址'),
  client_start: requiredIfIP('请输入起始地址'),
  client_end: requiredIfIP('请输入结束地址'),
  client_cidr6: {
    trigger: 'blur',
    validator: (_r, v: string) => (!v || isValidCIDR6(v, 127).valid ? true : new Error('IPv6 网段格式无效（须为 CIDR，如 2001:db8:10::/64，前缀 0-127）')),
  },
}
const FIELD_TAB: Record<string, string> = { name: 'basic', policy_id: 'basic' }

async function openEdit(row?: any) {
  tab.value = 'basic'
  formRef.value?.restoreValidation()
  form.value = blank()
  enableGroupIP.value = false
  drawer.value = true
  loadPolicyList()
  if (!row) return
  detailLoading.value = true
  try {
    const d = await get('/api/v1/group/detail', { id: row.id })
    const steps: any[] = d.auth_profile?.step || []
    form.value = {
      id: d.id, name: d.name, note: d.note || '',
      policy_id: d.policy_id || 0, status: d.status, hidden: d.hidden || 0,
      split_dns: d.split_dns || [],
      auth_profile: { step: steps.length ? steps.map(normalizeStep) : [newStep('local')] },
      client_cidr: d.client_cidr || '', client_start: d.client_start || '', client_end: d.client_end || '',
      client_gateway: d.client_gateway || '', client_cidr6: d.client_cidr6 || '', out_dev: d.out_dev || '',
    }
    enableGroupIP.value = !!(d.client_cidr && d.client_start && d.client_end && d.client_gateway)
    await nextTick()
    authRef.value?.preload()
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    detailLoading.value = false
  }
}

async function save() {
  if (saving.value) return
  try {
    await formRef.value?.validate()
  } catch (errs) {
    const field = (errs as FormValidationError[])?.[0]?.[0]?.field || ''
    tab.value = FIELD_TAB[field] || 'network'
    return
  }
  if (conflictTip.value) {
    tab.value = 'auth'
    ui.message.error(conflictTip.value)
    return
  }
  saving.value = true
  try {
    const f = form.value
    await post('/api/v1/group/set', {
      ...f,
      auth_profile: { step: f.auth_profile.step.map(({ _key, ...s }) => s) },
    })
    ui.message.success('保存成功')
    drawer.value = false
    await loadAllPolicyNames()
    load(1)
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

// 快速新建策略后：刷新下拉并选中最新（id 最大）的策略
async function onPolicyCreated() {
  await Promise.all([loadPolicyList(), loadAllPolicyNames()])
  if (policyList.value.length) form.value.policy_id = policyList.value.reduce((a, b) => (a.id > b.id ? a : b)).id
}

onMounted(async () => {
  loadPolicyList()
  loadIfaces()
  await loadAllPolicyNames()
  load(1)
})
</script>

<template>
  <PageHeader title="用户组" description="按组分配策略、内网域名、地址池与认证流程，用户通过所属组获得访问权限">
    <NButton type="primary" @click="openEdit()"><template #icon><Add /></template>新建用户组</NButton>
  </PageHeader>

  <div class="grid gap-16px">
    <div class="grid grid-cols-2 gap-16px lg:grid-cols-4">
      <StatCard label="用户组总数" :value="count" :icon="PeopleTeam" />
      <StatCard label="已启用" :value="stats.stats_active || 0" :icon="Checkmark" />
      <StatCard label="已配策略" :value="stats.stats_policy || 0" :icon="ShieldTask" />
      <StatCard label="组合认证" :value="stats.stats_multi_auth || 0" :icon="LockClosed" hint="认证流程包含多个步骤的组" />
    </div>

    <RlPanel :padded="false">
      <template #toolbar>
        <span class="text-md font-semibold">用户组列表</span>
        <div class="flex-1" />
        <span class="text-sm text-secondary">共 {{ count }} 条</span>
        <button class="rl-icon-btn" title="刷新" @click="load()"><ArrowClockwise /></button>
      </template>
      <NDataTable
        :columns="columns" :data="rows" :loading="loading" :bordered="false" :row-key="(r: any) => r.id"
        size="small" :scroll-x="1100"
      >
        <template #empty>
          <EmptyState title="暂无用户组" description="创建用户组并指定策略后，即可将用户加入组">
            <NButton @click="openEdit()">新建用户组</NButton>
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
  <NDrawer v-model:show="drawer" :width="680" :mask-closable="false">
    <NDrawerContent :title="isEdit ? `编辑用户组 · ${form.name}` : '新建用户组'" closable :native-scrollbar="false">
      <NSpin :show="detailLoading">
        <NForm ref="formRef" :model="form" :rules="rules" label-placement="top" require-mark-placement="right-hanging">
          <NTabs v-model:value="tab" type="line" display-directive="show" animated>
            <!-- 基本 -->
            <NTabPane name="basic" tab="基本">
              <FormSection title="基本信息">
                <div class="grid grid-cols-2 gap-x-16px">
                  <NFormItem label="组名" path="name">
                    <NInput v-model:value="form.name" :disabled="isEdit" placeholder="请输入组名" />
                  </NFormItem>
                  <NFormItem label="状态" path="status">
                    <NRadioGroup v-model:value="form.status">
                      <NRadioButton :value="1">启用</NRadioButton>
                      <NRadioButton :value="0">停用</NRadioButton>
                    </NRadioGroup>
                  </NFormItem>
                  <NFormItem label="备注" path="note" class="col-span-2">
                    <NInput v-model:value="form.note" placeholder="备注信息（可选）" />
                  </NFormItem>
                </div>
                <div class="flex items-center justify-between gap-md">
                  <div class="min-w-0">
                    <div class="text-md">对客户端隐藏</div>
                    <div class="mt-2px text-sm text-secondary">开启后该组不出现在客户端的组选择下拉中</div>
                  </div>
                  <NSwitch v-model:value="form.hidden" :checked-value="1" :unchecked-value="0" />
                </div>
              </FormSection>

              <FormSection title="策略" description="策略模版包含带宽、DNS、路由、访问控制等配置。启用状态的用户组必须指定策略。">
                <NFormItem path="policy_id" :show-label="false">
                  <div class="flex w-full items-center gap-sm">
                    <NSelect
                      :value="form.policy_id || null" :options="policyOptions" placeholder="请选择策略" filterable
                      class="min-w-0 flex-1" :fallback-option="policyFallback" @update:value="(v: number | null) => form.policy_id = v ?? 0"
                    />
                    <NButton @click="quickPolicy = true"><template #icon><Add /></template>快速新建</NButton>
                  </div>
                </NFormItem>
              </FormSection>
            </NTabPane>

            <!-- 网络 -->
            <NTabPane name="network" tab="网络">
              <FormSection title="内网域名" description="分割 DNS：留空则使用策略的 DNS 配置；配置后仅指定的域名（含子域名）走策略 DNS 解析。">
                <ValRowsEditor v-model="form.split_dns" val-label="域名" val-placeholder="如 corp.example.com" add-text="添加域名" empty-text="未配置，使用策略 DNS" />
              </FormSection>

              <FormSection title="独立 IP 段" description="为该组单独分配地址段，网关和掩码随之变化；关闭时使用全局 IP 池。">
                <template #extra>
                  <span class="text-sm text-secondary">{{ enableGroupIP ? '组级别分配' : '全局 IP 池' }}</span>
                  <NSwitch :value="enableGroupIP" @update:value="toggleGroupIP" />
                </template>
                <div v-if="enableGroupIP" class="grid grid-cols-2 gap-x-16px">
                  <NFormItem label="网段" path="client_cidr">
                    <NInput v-model:value="form.client_cidr" placeholder="如 10.0.1.0/24" class="rl-mono" />
                  </NFormItem>
                  <NFormItem label="网关地址" path="client_gateway">
                    <NInput v-model:value="form.client_gateway" placeholder="如 10.0.1.1" class="rl-mono" />
                  </NFormItem>
                  <NFormItem label="起始地址" path="client_start">
                    <NInput v-model:value="form.client_start" placeholder="如 10.0.1.100" class="rl-mono" />
                  </NFormItem>
                  <NFormItem label="结束地址" path="client_end">
                    <NInput v-model:value="form.client_end" placeholder="如 10.0.1.200" class="rl-mono" />
                  </NFormItem>
                  <NFormItem label="IPv6 网段" path="client_cidr6">
                    <NInput v-model:value="form.client_cidr6" placeholder="如 2001:db8:10::/64，留空用全局 v6 池" class="rl-mono" />
                  </NFormItem>
                  <NFormItem label="出网网卡" path="out_dev">
                    <NSelect
                      v-model:value="form.out_dev" :options="outDevOptions" filterable tag clearable
                      placeholder="留空使用默认 master_dev，可输入任意网卡名"
                    />
                  </NFormItem>
                  <div class="col-span-2 -mt-8px text-sm text-secondary">
                    网段、网关、起始、结束地址 4 项须全部填写。开启 IPv6 双栈时可指定独立 v6 网段（前缀须小于 128），留空复用全局 v6 池。
                  </div>
                </div>
              </FormSection>
            </NTabPane>

            <!-- 认证 -->
            <NTabPane name="auth" tab="认证">
              <FormSection title="认证流程" description="按顺序组合多个认证步骤实现多因素认证；流程为空时使用本地密码认证。">
                <AuthFlowEditor ref="authRef" v-model="form.auth_profile.step" :group-name="form.name" />
              </FormSection>
            </NTabPane>
          </NTabs>
        </NForm>
      </NSpin>
      <template #footer>
        <div class="flex items-center gap-sm">
          <span v-if="conflictTip" class="text-sm text-error">认证流程存在冲突</span>
          <div class="flex-1" />
          <NButton @click="drawer = false">取消</NButton>
          <NButton type="primary" :loading="saving" :disabled="detailLoading" @click="save">保存</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>

  <QuickPolicyDrawer v-model:show="quickPolicy" @created="onPolicyCreated" />
</template>
