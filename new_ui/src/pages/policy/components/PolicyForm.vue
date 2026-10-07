<script setup lang="ts">
// 策略表单：通用 / 路由 / 访问控制 / 域名拆分 / FakeDNS 五段，含批量编辑与「从策略复制」
import { computed, ref, watch } from 'vue'
import {
  NAlert, NButton, NDropdown, NForm, NFormItem, NInput, NModal, NRadioButton, NRadioGroup,
  NSelect, NSwitch, NTabPane, NTabs, type FormInst, type FormRules,
} from 'naive-ui'
import Copy from '~icons/fluent/document-copy-20-regular'
import Edit from '~icons/fluent/edit-20-regular'
import { get, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import { useAuthStore } from '@/stores/auth'
import FormSection from './FormSection.vue'
import ValRowsEditor from './ValRowsEditor.vue'
import AclEditor from './AclEditor.vue'
import { RESET_OPTIONS, isValidCIDR, numFilter, type AclRule, type PolicyModel, type ValData } from './policy'

const form = defineModel<PolicyModel>({ required: true })
const auth = useAuthStore()

const tab = ref('general')
const formRef = ref<FormInst | null>(null)
const MAX_ROUTE_ROWS = 2500
const COLLAPSE_AT = 10

const rules: FormRules = {
  name: [
    { required: true, message: '请输入策略名称', trigger: 'blur' },
    { max: 60, message: '长度小于 60 个字符', trigger: 'blur' },
  ],
  bandwidth_format: { required: true, message: '请输入下行带宽', trigger: 'blur' },
  bandwidth_up_format: { required: true, message: '请输入上行带宽', trigger: 'blur' },
  traffic_quota_format: { required: true, message: '请输入流量配额', trigger: 'blur' },
}

const quotaOn = computed(() => parseFloat(form.value.traffic_quota_format) > 0)

// —— 从其他策略复制（切到路由 / ACL 时懒加载） ——
const otherPolicies = ref<{ id: number; name: string }[]>([])
let othersLoaded = false
async function loadOthers() {
  if (othersLoaded) return
  try {
    const d = await get('/api/v1/policy/list', { page: 1, page_size: 9999 })
    otherPolicies.value = (d?.datas || []).filter((p: any) => p.id !== form.value.id).map((p: any) => ({ id: p.id, name: p.name }))
    othersLoaded = true
  } catch { /* 忽略 */ }
}
watch(tab, t => { if (t === 'route' || t === 'link_acl') loadOthers() })
const copyOptions = computed(() => otherPolicies.value.length
  ? otherPolicies.value.map(p => ({ label: p.name, key: p.id }))
  : [{ label: '暂无其他策略', key: -1, disabled: true }])

const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v))
async function copyRoutesFrom(id: number) {
  try {
    const d = await get('/api/v1/policy/detail', { id })
    if (d.route_include?.length) form.value.route_include = clone(d.route_include)
    if (d.route_exclude?.length) form.value.route_exclude = clone(d.route_exclude)
    ui.message.success('路由已从策略复制')
  } catch (e) { ui.message.error(errMsg(e, '复制失败')) }
}
async function copyAclFrom(id: number) {
  try {
    const d = await get('/api/v1/policy/detail', { id })
    form.value.link_acl = d.link_acl?.length ? clone(d.link_acl) : []
    ui.message.success('ACL 规则已从策略复制')
  } catch (e) { ui.message.error(errMsg(e, '复制失败')) }
}

// —— 路由批量编辑 ——
const routeModal = ref(false)
const routeType = ref<'route_include' | 'route_exclude'>('route_include')
const routeText = ref('')
const routeCount = computed(() => routeText.value.split('\n').filter(l => l.trim()).length)
function openRouteBulk(type: 'route_include' | 'route_exclude') {
  routeType.value = type
  routeText.value = (form.value[type] as ValData[]).map(i => i.val + (i.note ? ',' + i.note : '')).join('\n')
  routeModal.value = true
}
function applyRouteBulk() {
  const arr: ValData[] = []
  for (const line of routeText.value.trim().split('\n')) {
    if (!line.trim()) continue
    const [val, ...rest] = line.split(',')
    const v = val.trim()
    const note = rest.join(',')
    if (routeType.value === 'route_include' && v === 'all') { arr.push({ val: v, note }); continue }
    const c = isValidCIDR(v)
    if (!c.valid) {
      ui.message.error(c.suggestion ? `CIDR 格式错误，建议 ${v} 改为 ${c.suggestion}` : `CIDR 格式错误：${v}`)
      return
    }
    arr.push({ val: v, note })
  }
  form.value[routeType.value] = arr
  routeModal.value = false
}

// —— ACL 批量编辑 ——
const aclModal = ref(false)
const aclText = ref('')
const aclCount = computed(() => aclText.value.split('\n').filter(l => l.trim()).length)
function openAclBulk() {
  aclText.value = (form.value.link_acl || [])
    .map(i => [i.action || 'allow', i.val, i.protocol || 'all', i.port || '0', i.note || ''].join(','))
    .join('\n')
  aclModal.value = true
}
function applyAclBulk() {
  const arr: AclRule[] = []
  const lines = aclText.value.trim() ? aclText.value.trim().split('\n') : []
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (!line) continue
    const p = line.split(',')
    const action = (p[0] || 'allow').trim()
    if (action !== 'allow' && action !== 'deny') {
      ui.message.error(`第 ${i + 1} 行动作无效：${action}，请使用 allow 或 deny`)
      return
    }
    arr.push({
      action,
      val: (p[1] || '').trim(),
      protocol: (p[2] || 'all').trim(),
      port: (p[3] || '0').trim(),
      note: p.length > 4 ? p.slice(4).join(',').trim() : '',
    })
  }
  form.value.link_acl = arr
  aclModal.value = false
}

// 校验失败时切回「通用」分段（所有带规则的字段都在这里）
async function validate() {
  try {
    await formRef.value?.validate()
    return true
  } catch {
    tab.value = 'general'
    return false
  }
}
function reset() {
  tab.value = 'general'
  othersLoaded = false
  otherPolicies.value = []
  formRef.value?.restoreValidation()
}
defineExpose({ validate, reset })
</script>

<template>
  <NForm ref="formRef" :model="form" :rules="rules" label-placement="top" require-mark-placement="right-hanging">
    <NTabs v-model:value="tab" type="line" display-directive="show" animated>
      <!-- 通用 -->
      <NTabPane name="general" tab="通用">
        <FormSection title="基本信息">
          <div class="grid grid-cols-2 gap-x-16px">
            <NFormItem label="策略名称" path="name">
              <NInput v-model:value="form.name" placeholder="如：办公策略、运维策略" />
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
        </FormSection>

        <FormSection title="带宽与流量" description="0 表示不限制。带宽按 Mbps 填写，流量按 GB 填写。">
          <div class="grid grid-cols-2 gap-x-16px">
            <NFormItem label="下行带宽（服务器 → 客户端）" path="bandwidth_format">
              <NInput :value="form.bandwidth_format" @update:value="v => form.bandwidth_format = numFilter(v)">
                <template #suffix><span class="text-tertiary">Mbps</span></template>
              </NInput>
            </NFormItem>
            <NFormItem label="上行带宽（客户端 → 服务器）" path="bandwidth_up_format">
              <NInput :value="form.bandwidth_up_format" @update:value="v => form.bandwidth_up_format = numFilter(v)">
                <template #suffix><span class="text-tertiary">Mbps</span></template>
              </NInput>
            </NFormItem>
            <NFormItem label="流量配额" path="traffic_quota_format">
              <NInput :value="form.traffic_quota_format" @update:value="v => form.traffic_quota_format = numFilter(v)">
                <template #suffix><span class="text-tertiary">GB</span></template>
              </NInput>
            </NFormItem>
            <NFormItem label="重置周期" path="traffic_reset">
              <NSelect v-model:value="form.traffic_reset" :options="RESET_OPTIONS" :disabled="!quotaOn" />
            </NFormItem>
          </div>
          <div class="-mt-8px text-sm text-secondary">上下行累计流量超出配额后断开连接；设置配额但未选周期时默认按月重置。</div>
        </FormSection>

        <FormSection title="本地网络">
          <div class="flex items-center justify-between gap-md">
            <div class="min-w-0">
              <div class="text-md">排除本地网络（Allow Local LAN）</div>
              <div class="mt-2px text-sm text-secondary">开启后，AnyConnect 客户端需勾选 Allow Local LAN 才能生效</div>
            </div>
            <NSwitch v-model:value="form.allow_lan" />
          </div>
        </FormSection>

        <FormSection title="客户端 DNS" description="下发给客户端的 DNS 服务器，按顺序生效">
          <ValRowsEditor v-model="form.client_dns" val-label="DNS 地址" val-placeholder="如 8.8.8.8" add-text="添加 DNS" />
        </FormSection>
      </NTabPane>

      <!-- 路由 -->
      <NTabPane name="route" tab="路由">
        <FormSection title="包含路由" description="走 VPN 隧道的网段；填写 all 表示全局代理">
          <template #extra>
            <NDropdown trigger="click" :options="copyOptions" @select="(k: number) => copyRoutesFrom(k)">
              <NButton size="small" quaternary><template #icon><Copy /></template>从策略复制</NButton>
            </NDropdown>
            <NButton size="small" quaternary @click="openRouteBulk('route_include')"><template #icon><Edit /></template>批量编辑</NButton>
          </template>
          <ValRowsEditor
            v-model="form.route_include" val-label="CIDR 地址" val-placeholder="如 192.168.1.0/24 或 all"
            add-text="添加包含路由" :collapse-at="COLLAPSE_AT" empty-text="未配置包含路由"
          />
        </FormSection>
        <FormSection title="排除路由" description="不走 VPN 隧道的网段">
          <template #extra>
            <NButton size="small" quaternary @click="openRouteBulk('route_exclude')"><template #icon><Edit /></template>批量编辑</NButton>
          </template>
          <ValRowsEditor
            v-model="form.route_exclude" val-label="CIDR 地址" val-placeholder="如 10.0.0.0/8"
            add-text="添加排除路由" :collapse-at="COLLAPSE_AT" empty-text="未配置排除路由"
          />
        </FormSection>
      </NTabPane>

      <!-- 访问控制 -->
      <NTabPane name="link_acl" tab="访问控制">
        <FormSection title="访问控制规则（ACL）" description="规则自上而下匹配，未匹配的流量默认拒绝。协议 all / tcp / udp / icmp；端口 0 表示全部，多个端口逗号分隔（80,443），区间用 8000-9000。">
          <template #extra>
            <NDropdown trigger="click" :options="copyOptions" @select="(k: number) => copyAclFrom(k)">
              <NButton size="small" quaternary><template #icon><Copy /></template>从策略复制</NButton>
            </NDropdown>
            <NButton size="small" quaternary @click="openAclBulk"><template #icon><Edit /></template>批量编辑</NButton>
          </template>
          <AclEditor v-model="form.link_acl" />
        </FormSection>
      </NTabPane>

      <!-- 域名拆分隧道 -->
      <NTabPane name="ds_domains" tab="域名拆分">
        <NAlert type="warning" :bordered="false" class="mb-16px">
          域名拆分隧道仅支持 AnyConnect 的 Windows 和 macOS 桌面客户端，不支持移动端。
        </NAlert>
        <FormSection title="包含域名" description="匹配的域名走 VPN 隧道；多个域名逗号分隔，默认匹配所有子域名">
          <NInput v-model:value="form.ds_include_domains" type="textarea" :rows="4" placeholder="如 baidu.com,163.com" class="rl-mono" />
        </FormSection>
        <FormSection title="排除域名" description="匹配的域名不走 VPN 隧道；多个域名逗号分隔，默认匹配所有子域名">
          <NInput v-model:value="form.ds_exclude_domains" type="textarea" :rows="4" placeholder="如 baidu.com,163.com" class="rl-mono" />
        </FormSection>
      </NTabPane>

      <!-- FakeDNS -->
      <NTabPane v-if="auth.showFakeDNS" name="fakedns" tab="FakeDNS">
        <FormSection title="FakeDNS">
          <div class="flex items-center justify-between gap-md">
            <div class="min-w-0">
              <div class="text-md">启用 FakeDNS</div>
              <div class="mt-2px text-sm text-secondary">通过服务端拦截 DNS 请求实现域名分流，支持所有客户端（含移动端）</div>
            </div>
            <NSwitch v-model:value="form.enable_fakedns" />
          </div>
        </FormSection>
        <FormSection title="上游 DNS" :class="!form.enable_fakedns && 'opacity-60'">
          <NFormItem label="DNS 地址" path="fake_dns_upstream" :show-feedback="false">
            <NInput v-model:value="form.fake_dns_upstream" placeholder="如 8.8.8.8（留空则使用客户端 DNS）" :disabled="!form.enable_fakedns" class="rl-mono" />
          </NFormItem>
          <div v-if="form.enable_fakedns" class="mt-6px text-sm text-warning">若未配置客户端 DNS，需手动配置上游 DNS 地址，否则 FakeDNS 无法生效</div>
          <div class="mt-16px flex items-center justify-between gap-md">
            <div class="min-w-0">
              <div class="text-md">IPv6 优先</div>
              <div class="mt-2px text-sm text-secondary">命中规则的域名优先返回 AAAA（v6 fakeIP），对 A 查询返回 NODATA，引导双栈应用走 v6（仅双栈开启时有效）</div>
            </div>
            <NSwitch v-model:value="form.prefer_ipv6" :disabled="!form.enable_fakedns" />
          </div>
        </FormSection>
        <FormSection title="包含域名（白名单模式）" description="仅拦截列表中的域名，其余正常解析。与「排除域名」二选一，不能同时填写。" :class="!form.enable_fakedns && 'opacity-60'">
          <NInput v-model:value="form.fake_dns_include" type="textarea" :rows="4" :disabled="!form.enable_fakedns" placeholder="如 google.com,youtube.com" class="rl-mono" />
        </FormSection>
        <FormSection title="排除域名（黑名单模式）" description="拦截所有域名，仅排除列表中的域名正常解析。与「包含域名」二选一，不能同时填写。" :class="!form.enable_fakedns && 'opacity-60'">
          <NInput v-model:value="form.fake_dns_exclude" type="textarea" :rows="4" :disabled="!form.enable_fakedns" placeholder="如 baidu.com,163.com" class="rl-mono" />
        </FormSection>
      </NTabPane>
    </NTabs>
  </NForm>

  <!-- 路由批量编辑 -->
  <NModal
    v-model:show="routeModal" preset="card" :title="routeType === 'route_include' ? '批量编辑包含路由' : '批量编辑排除路由'"
    class="w-640px! max-w-[calc(100vw-32px)]" :mask-closable="false"
  >
    <NInput
      v-model:value="routeText" type="textarea" :rows="14" class="rl-mono"
      :placeholder="'每行一条路由，格式：192.168.1.0/24,备注\n或简写：192.168.1.0/24'"
    />
    <div class="mt-8px text-sm text-secondary">
      当前共 <b class="text-primary">{{ routeCount }}</b> 条（AnyConnect 客户端最多支持 {{ MAX_ROUTE_ROWS }} 条）
      <span v-if="routeCount > MAX_ROUTE_ROWS" class="text-error">，已超出上限</span>
    </div>
    <template #footer>
      <div class="flex justify-end gap-sm">
        <NButton @click="routeModal = false">取消</NButton>
        <NButton type="primary" @click="applyRouteBulk">更新路由</NButton>
      </div>
    </template>
  </NModal>

  <!-- ACL 批量编辑 -->
  <NModal v-model:show="aclModal" preset="card" title="批量编辑 ACL 规则" class="w-720px! max-w-[calc(100vw-32px)]" :mask-closable="false">
    <div class="mb-8px text-sm text-secondary">
      每行一条规则，格式 <span class="rl-kbd">动作,CIDR,协议,端口,备注</span>；动作 allow / deny，协议 all / tcp / udp / icmp，端口 0 表示不限。
    </div>
    <NInput
      v-model:value="aclText" type="textarea" :rows="14" class="rl-mono"
      :placeholder="'allow,192.168.1.0/24,tcp,80,公司内网\ndeny,10.0.0.0/8,all,0,禁止内部网段'"
    />
    <div class="mt-8px text-sm text-secondary">当前共 <b class="text-primary">{{ aclCount }}</b> 条规则</div>
    <template #footer>
      <div class="flex justify-end gap-sm">
        <NButton @click="aclModal = false">取消</NButton>
        <NButton type="primary" @click="applyAclBulk">更新 ACL</NButton>
      </div>
    </template>
  </NModal>
</template>
