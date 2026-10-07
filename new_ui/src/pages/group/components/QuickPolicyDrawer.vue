<script setup lang="ts">
// 快速新建策略：只含基础信息、DNS 与路由；高级项（ACL / 域名拆分 / FakeDNS）去策略页配置
import { ref, watch } from 'vue'
import {
  NButton, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NRadioButton, NRadioGroup, NSwitch,
  type FormInst, type FormRules,
} from 'naive-ui'
import { post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import { useAuthStore } from '@/stores/auth'
import FormSection from '@/pages/policy/components/FormSection.vue'
import ValRowsEditor from '@/pages/policy/components/ValRowsEditor.vue'
import { numFilter, type ValData } from '@/pages/policy/components/policy'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ created: [] }>()
const auth = useAuthStore()

const blank = () => ({
  name: '', note: '', status: 1, allow_lan: true,
  bandwidth_format: '0', bandwidth_up_format: '0',
  client_dns: [{ val: '114.114.114.114', note: '' }] as ValData[],
  route_include: [{ val: 'all', note: '' }] as ValData[],
  route_exclude: [] as ValData[],
})
const form = ref(blank())
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const rules: FormRules = {
  name: [
    { required: true, message: '请输入策略名称', trigger: 'blur' },
    { max: 60, message: '长度不超过 60 个字符', trigger: 'blur' },
  ],
}

watch(show, v => { if (v) { form.value = blank(); formRef.value?.restoreValidation() } })

async function create() {
  try { await formRef.value?.validate() } catch { return }
  const f = form.value
  const bw = parseFloat(f.bandwidth_format) || 0
  const bwUp = parseFloat(f.bandwidth_up_format) || 0
  saving.value = true
  try {
    await post('/api/v1/policy/set', {
      name: f.name,
      note: f.note,
      status: f.status,
      allow_lan: f.allow_lan,
      bandwidth: bw > 0 ? Math.round(bw * 125000) : 0, // Mbps → Byte/s
      bandwidth_up: bwUp > 0 ? Math.round(bwUp * 125000) : 0,
      bandwidth_format: f.bandwidth_format,
      bandwidth_up_format: f.bandwidth_up_format,
      client_dns: f.client_dns.filter(d => d.val),
      route_include: f.route_include.filter(r => r.val),
      route_exclude: f.route_exclude.filter(r => r.val),
    })
    ui.message.success('策略创建成功，已自动选中')
    show.value = false
    emit('created')
  } catch (e) {
    ui.message.error(errMsg(e, '策略创建失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="600" :mask-closable="false">
    <NDrawerContent title="快速新建策略" closable :native-scrollbar="false">
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="top" require-mark-placement="right-hanging">
        <FormSection title="基础信息">
          <div class="grid grid-cols-2 gap-x-16px">
            <NFormItem label="策略名称" path="name">
              <NInput v-model:value="form.name" placeholder="如：办公策略、运维策略" />
            </NFormItem>
            <NFormItem label="状态">
              <NRadioGroup v-model:value="form.status">
                <NRadioButton :value="1">启用</NRadioButton>
                <NRadioButton :value="0">停用</NRadioButton>
              </NRadioGroup>
            </NFormItem>
            <NFormItem label="下行带宽">
              <NInput :value="form.bandwidth_format" placeholder="0 表示不限速" @update:value="v => form.bandwidth_format = numFilter(v)">
                <template #suffix><span class="text-tertiary">Mbps</span></template>
              </NInput>
            </NFormItem>
            <NFormItem label="上行带宽">
              <NInput :value="form.bandwidth_up_format" placeholder="0 表示不限速" @update:value="v => form.bandwidth_up_format = numFilter(v)">
                <template #suffix><span class="text-tertiary">Mbps</span></template>
              </NInput>
            </NFormItem>
            <NFormItem label="备注" class="col-span-2">
              <NInput v-model:value="form.note" placeholder="可选" />
            </NFormItem>
          </div>
          <div class="flex items-center justify-between gap-md">
            <div>
              <div class="text-md">本地网络</div>
              <div class="mt-2px text-sm text-secondary">允许访问本地局域网</div>
            </div>
            <NSwitch v-model:value="form.allow_lan" />
          </div>
        </FormSection>

        <FormSection title="DNS 服务器">
          <ValRowsEditor v-model="form.client_dns" val-label="DNS 地址" val-placeholder="如 114.114.114.114" add-text="添加 DNS" :min-rows="1" />
        </FormSection>

        <FormSection title="包含路由" description="决定哪些流量走 VPN 隧道，all 表示全局代理">
          <ValRowsEditor v-model="form.route_include" val-label="CIDR 地址" val-placeholder="CIDR 地址或 all" add-text="添加包含路由" :min-rows="1" />
        </FormSection>

        <FormSection title="排除路由" description="不走 VPN 隧道的网段">
          <ValRowsEditor v-model="form.route_exclude" val-label="CIDR 地址" val-placeholder="CIDR 地址" add-text="添加排除路由" empty-text="未配置排除路由" />
        </FormSection>

        <div class="mt-20px rounded-8px bg-fill px-12px py-10px text-sm text-secondary">
          ACL 访问控制<template v-if="auth.showFakeDNS">、域名拆分隧道、FakeDNS</template>等高级功能请前往「策略」页面配置。
        </div>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-sm">
          <NButton @click="show = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="create">新建并选择</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>
