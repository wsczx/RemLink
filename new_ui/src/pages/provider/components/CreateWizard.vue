<script setup lang="ts">
// 新建认证源向导：基础信息 → 连接配置 → 测试 → 完成
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NForm, NFormItem, NInput, NStep, NSteps, NSwitch, type FormInst, type FormRules } from 'naive-ui'
import ArrowLeft from '~icons/fluent/arrow-left-20-regular'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import { post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import ConfigFields from './ConfigFields.vue'
import CallbackList from './CallbackList.vue'
import TestLoginCard from './TestLoginCard.vue'
import { loadAllProviders } from './api'
import { defaultConfig, isOAuth, isSyncable, isTestable, typeMeta } from './types'

const props = defineProps<{ type: string }>()
const router = useRouter()

const meta = computed(() => typeMeta(props.type))
const step = ref(1)
const saving = ref(false)
const formRef = ref<FormInst | null>(null)
const base = ref({ name: '', status: 1 })
const cfg = ref<Record<string, any>>(defaultConfig(props.type))
const createdId = ref(0)
const callbackBase = ref('')
const existNames = ref<string[]>([])
const tested = ref<boolean | null>(null)

// 预取已有名称（名称唯一）和回调基础地址
loadAllProviders().then(d => {
  existNames.value = d.rows.map(r => r.name)
  callbackBase.value = d.callbackBase
}).catch(() => { /* 忽略，后端仍会校验 */ })

const rules: FormRules = {
  name: [
    { required: true, message: '请输入名称', trigger: 'blur' },
    { max: 60, message: '长度不超过 60 个字符', trigger: 'blur' },
    {
      trigger: 'blur',
      validator: (_r, v: string) => (createdId.value || !existNames.value.includes(v.trim()) ? true : new Error('已存在同名认证源')),
    },
  ],
}

const STEPS = [
  { title: '基础信息', desc: '命名与启用' },
  { title: '连接配置', desc: '填写连接参数' },
  { title: '测试', desc: '验证是否可用' },
  { title: '完成', desc: '投入使用' },
]

async function next1() {
  try { await formRef.value?.validate() } catch { return }
  base.value.name = base.value.name.trim()
  step.value = 2
}

// 第 2 步保存：首次保存为新建，之后返回修改再保存为更新
async function save2() {
  saving.value = true
  try {
    await post('/api/v1/provider/set', { id: createdId.value, name: base.value.name, type: props.type, status: base.value.status, config: cfg.value })
    if (!createdId.value) {
      // 新建接口不返回 id，按唯一名称回查
      const d = await loadAllProviders()
      createdId.value = d.rows.find(r => r.name === base.value.name)?.id || 0
      if (d.callbackBase) callbackBase.value = d.callbackBase
    }
    ui.message.success('已保存')
    step.value = 3
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

const testDisabled = computed(() => {
  if (!createdId.value) return '未能获取新建认证源的 ID，请稍后在详情页测试'
  if (base.value.status !== 1) return '该认证源处于停用状态，启用后才能测试登录'
  return ''
})

const goDetail = () => router.replace(`/admin/provider/${createdId.value}`)
</script>

<template>
  <div class="pb-40px">
    <button type="button" class="pv-back" @click="router.push('/admin/provider/list')"><ArrowLeft />认证源</button>
    <PageHeader class="pt-8px!" :title="`接入${meta?.full || ''}`" :description="meta?.scene ? `适用于：${meta.scene}` : ''" />

    <div class="grid gap-16px">
      <RlPanel>
        <NSteps :current="step" size="small">
          <NStep v-for="s in STEPS" :key="s.title" :title="s.title" :description="s.desc" />
        </NSteps>
      </RlPanel>

      <!-- 1 基础信息 -->
      <RlPanel v-if="step === 1" title="基础信息" subtitle="名称会出现在用户组的认证流程中，创建后不可修改">
        <div class="mb-20px flex items-center gap-12px rounded-10px bg-fill px-14px py-12px">
          <component :is="meta?.icon" class="text-2xl" />
          <div class="min-w-0 flex-1">
            <div class="text-md font-semibold">{{ meta?.full }}</div>
            <div class="text-sm text-secondary">{{ meta?.desc }}</div>
          </div>
          <NButton text size="small" @click="router.push({ path: '/admin/provider/list', hash: '#pv-add' })">更换类型</NButton>
        </div>
        <NForm ref="formRef" :model="base" :rules="rules" label-placement="top" class="max-w-520px">
          <NFormItem label="名称" path="name">
            <NInput v-model:value="base.name" :disabled="!!createdId" :placeholder="type === 'ldap' ? '如：总部 AD' : `如：${meta?.full}扫码`" @keydown.enter="next1" />
          </NFormItem>
          <NFormItem label="创建后立即启用">
            <NSwitch v-model:value="base.status" :checked-value="1" :unchecked-value="0" />
          </NFormItem>
        </NForm>
      </RlPanel>

      <!-- 2 连接配置 -->
      <template v-if="step === 2">
        <RlPanel v-if="isOAuth(type)" title="先在开放平台填写回调地址" subtitle="用户扫码后，平台会跳转回这些地址完成登录" :padded="true">
          <CallbackList :type="type" :base="callbackBase" />
        </RlPanel>
        <ConfigFields v-model:cfg="cfg" :type="type" />
      </template>

      <!-- 3 测试 -->
      <template v-if="step === 3">
        <RlPanel v-if="isTestable(type)" title="用一个真实账号测试" subtitle="使用刚保存的配置连接服务器，并校验该账号的密码">
          <TestLoginCard :provider-id="createdId" :disabled-reason="testDisabled" @result="v => (tested = v)" />
        </RlPanel>
        <RlPanel v-else title="扫码登录需要在客户端验证" subtitle="后台无法模拟扫码，请按下面的清单检查后在 AnyConnect 客户端试登录">
          <ol class="m-0 grid gap-8px pl-18px text-md text-secondary">
            <li>开放平台中的应用已发布，且可见范围包含要登录的员工</li>
            <li>回调地址已按下方填写，域名与 VPN 服务地址一致</li>
            <li>在用户组的认证流程中引用该认证源（下一步会引导）</li>
          </ol>
          <div class="mt-16px"><CallbackList :type="type" :base="callbackBase" /></div>
        </RlPanel>
      </template>

      <!-- 4 完成 -->
      <RlPanel v-if="step === 4">
        <div class="flex flex-col items-center py-24px text-center">
          <CheckmarkCircle class="text-4xl text-success" />
          <div class="mt-10px text-xl font-semibold">「{{ base.name }}」已接入</div>
          <div class="mt-4px text-md text-secondary">还差一步：在用户组的认证流程中引用它，用户才能用它登录。</div>
        </div>
        <div class="mx-auto grid max-w-640px gap-10px">
          <div class="pv-next">
            <span class="pv-next__no">1</span>
            <div class="min-w-0 flex-1">
              <div class="text-md font-semibold">在用户组中引用</div>
              <div class="text-sm text-secondary">编辑用户组 → 认证流程，添加「{{ meta?.label }}」步骤并选择「{{ base.name }}」</div>
            </div>
            <NButton size="small" @click="router.push('/admin/group/list')">前往用户组</NButton>
          </div>
          <div v-if="isSyncable(type)" class="pv-next">
            <span class="pv-next__no">2</span>
            <div class="min-w-0 flex-1">
              <div class="text-md font-semibold">同步用户（可选）</div>
              <div class="text-sm text-secondary">在详情页把{{ meta?.label }}中的用户导入到引用它的用户组</div>
            </div>
            <NButton size="small" :disabled="!createdId" @click="goDetail">去同步</NButton>
          </div>
        </div>
      </RlPanel>

      <!-- 底部导航 -->
      <div class="pv-wizard-bar">
        <span class="text-sm text-secondary">第 {{ step }} / 4 步</span>
        <span v-if="step === 3 && tested === false" class="text-sm text-warning">测试未通过，可返回上一步修改配置</span>
        <div class="flex-1" />
        <template v-if="step === 1">
          <NButton @click="router.push('/admin/provider/list')">取消</NButton>
          <NButton type="primary" @click="next1">下一步</NButton>
        </template>
        <template v-else-if="step === 2">
          <NButton @click="step = 1">上一步</NButton>
          <NButton type="primary" :loading="saving" @click="save2">保存并继续</NButton>
        </template>
        <template v-else-if="step === 3">
          <NButton @click="step = 2">上一步</NButton>
          <NButton type="primary" @click="step = 4">{{ isTestable(type) && tested !== true ? '跳过测试' : '下一步' }}</NButton>
        </template>
        <template v-else>
          <NButton @click="router.push('/admin/provider/list')">返回列表</NButton>
          <NButton type="primary" :disabled="!createdId" @click="goDetail">查看详情</NButton>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pv-back {
  display: inline-flex; align-items: center; gap: 4px; margin-top: 20px; padding: 0; border: 0; background: none;
  font: inherit; font-size: 13px; color: var(--color-text-secondary); cursor: pointer;
}
.pv-back:hover { color: var(--color-text-primary); }
.pv-next {
  display: flex; align-items: center; gap: 12px; padding: 12px 14px;
  border: 1px solid var(--color-border-default); border-radius: 10px;
}
.pv-next__no {
  display: inline-flex; flex-shrink: 0; align-items: center; justify-content: center; width: 24px; height: 24px;
  border-radius: 50%; background: var(--color-fill); font-size: 12px; font-weight: 600;
}
.pv-wizard-bar {
  position: sticky; bottom: 0; z-index: 10; display: flex; flex-wrap: wrap; align-items: center; gap: 10px;
  padding: 12px 0; background: var(--color-body); border-top: 1px solid var(--color-border-default);
}
</style>
