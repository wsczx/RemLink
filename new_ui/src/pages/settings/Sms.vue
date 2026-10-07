<script setup lang="ts">
// 短信：通道状态 → 服务商卡片（阿里云 / 腾讯云 / 关闭）→ 服务商配置 → 测试发送
import { computed, ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import Phone from '~icons/fluent/phone-20-regular'
import Cloud from '~icons/fluent/cloud-20-regular'
import Power from '~icons/fluent/power-20-regular'
import Send from '~icons/fluent/send-20-regular'
import Key from '~icons/fluent/key-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import StatusDot from '@/components/StatusDot.vue'
import BrandStatusCard from './components/BrandStatusCard.vue'
import BrandChoiceCard from './components/BrandChoiceCard.vue'
import BrandSaveBar from './components/BrandSaveBar.vue'
import { useSettingForm } from './components/BrandUseForm'
import { SMS_PROVIDERS, smsMissing, type SmsForm } from './components/SmsDefs'
import { get, post, errMsg } from '@/api/http'
import { confirm, ui } from '@/utils/ui'

const empty = (): SmsForm => ({
  provider: '',
  ali_access_key_id: '', ali_access_key_secret: '', ali_sign_name: '', ali_template_code: '',
  tencent_secret_id: '', tencent_secret_key: '', tencent_app_id: '', tencent_sign_name: '', tencent_template_id: '', tencent_region: '',
})

const { form, saved, dirty, loading, saving, save, reset, commit } = useSettingForm<SmsForm>(empty, {
  fetch: async () => ({ ...empty(), ...((await get('/api/v1/set/other/sms')) || {}) }),
  submit: v => post('/api/v1/set/other/sms/edit', v),
  validate: v => {
    const miss = smsMissing(v, v.provider)
    return miss.length ? `请完善必填项：${miss.join('、')}` : null
  },
  successText: '短信配置已保存',
})

const active = computed(() => SMS_PROVIDERS.find(p => p.value === saved.value.provider))
const signName = computed(() => (saved.value.provider === 'aliyun' ? saved.value.ali_sign_name : saved.value.tencent_sign_name))
const templateId = computed(() => (saved.value.provider === 'aliyun' ? saved.value.ali_template_code : saved.value.tencent_template_id))

// 各服务商卡片状态：当前使用 / 已配置（已保存的必填项齐全）/ 未配置
function providerState(v: string) {
  if (!v) return null
  if (saved.value.provider === v) return { tone: 'success' as const, text: '使用中' }
  return smsMissing(saved.value, v).length ? { tone: 'muted' as const, text: '未配置' } : { tone: 'info' as const, text: '已配置' }
}

// 选择「关闭」：若当前已启用则确认后立即保存关闭状态（后端保留已填写的密钥）
async function pick(v: string) {
  if (v !== '' || !saved.value.provider) {
    form.value.provider = v
    return
  }
  if (!(await confirm('关闭后短信验证码登录与短信二次认证将立即不可用，已填写的服务商配置会保留。', '关闭短信服务？'))) return
  try {
    await post('/api/v1/set/other/sms/edit', { provider: '' })
    const keep = form.value
    form.value = { ...saved.value, provider: '' }
    commit()
    form.value = { ...keep, provider: '' }
    ui.message.success('短信服务已关闭')
  } catch (e) {
    ui.message.error(errMsg(e))
  }
}

// ---------- 测试发送（使用已保存的配置） ----------
const phone = ref('')
const testing = ref(false)
const testDisabled = computed(() => !saved.value.provider || dirty.value)
async function sendTest() {
  if (!phone.value) return ui.message.warning('请输入接收测试短信的手机号')
  testing.value = true
  try {
    await post('/api/v1/set/other/sms/test', { phone: phone.value })
    ui.message.success('测试短信已发送，请留意手机')
  } catch (e) {
    ui.message.error(errMsg(e, '发送失败'))
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <PageHeader title="短信" description="接入短信服务商，为验证码登录和二次认证发送短信" />

  <div class="grid gap-16px" :class="loading && 'pointer-events-none opacity-60'">
    <!-- 通道状态 -->
    <BrandStatusCard title="短信通道" :tone="active ? 'success' : 'muted'">
      <template #icon><Phone /></template>
      <template #status>
        <StatusDot v-if="active" tone="success" pulse>已启用 · {{ active.label }}</StatusDot>
        <StatusDot v-else tone="muted">已关闭</StatusDot>
      </template>
      <template v-if="active">验证码短信将通过{{ active.label }}发送，签名为「{{ signName || '-' }}」。</template>
      <template v-else>当前未启用短信服务，依赖短信的验证方式不可用。在下方选择一个服务商并完成配置即可启用。</template>
      <template v-if="active" #meta>
        <span class="ax-badge text-secondary">签名 {{ signName || '-' }}</span>
        <span class="ax-badge rl-mono text-secondary">模板 {{ templateId || '-' }}</span>
        <span v-if="saved.provider === 'tencent'" class="ax-badge rl-mono text-secondary">{{ saved.tencent_region || 'ap-guangzhou' }}</span>
      </template>
      <template #aside>
        <div class="mb-10px text-sm font-semibold text-secondary">依赖短信的功能</div>
        <ul class="m-0 grid list-none gap-8px p-0 text-md">
          <li class="flex items-center gap-8px"><StatusDot :tone="active ? 'success' : 'muted'">短信验证码登录</StatusDot></li>
          <li class="flex items-center gap-8px"><StatusDot :tone="active ? 'success' : 'muted'">短信二次认证</StatusDot></li>
        </ul>
        <div class="mt-12px text-sm text-tertiary">验证码模板需包含「验证码」和「有效分钟数」两个变量。</div>
      </template>
    </BrandStatusCard>

    <!-- 服务商选择 -->
    <div>
      <div class="mb-10px text-lg font-semibold">选择服务商</div>
      <div class="grid grid-cols-1 gap-12px md:grid-cols-3">
        <BrandChoiceCard
          v-for="p in SMS_PROVIDERS" :key="p.value" :title="p.label" :description="p.desc"
          :active="form.provider === p.value" @select="pick(p.value)"
        >
          <template #icon><component :is="p.value ? Cloud : Power" /></template>
          <template #status>
            <StatusDot v-if="providerState(p.value)" :tone="providerState(p.value)!.tone" class="text-sm">{{ providerState(p.value)!.text }}</StatusDot>
          </template>
        </BrandChoiceCard>
      </div>
    </div>

    <!-- 阿里云 -->
    <RlPanel v-if="form.provider === 'aliyun'" title="阿里云短信服务" subtitle="在阿里云控制台「短信服务」中获取以下信息" :padded="false">
      <SettingRow label="AccessKey ID" description="建议使用仅授权短信发送的 RAM 子账号" required>
        <NInput v-model:value="form.ali_access_key_id" placeholder="LTAI5t…" class="rl-mono">
          <template #prefix><Key class="text-tertiary" /></template>
        </NInput>
      </SettingRow>
      <SettingRow label="AccessKey Secret" description="保持掩码不变表示不修改" required>
        <NInput v-model:value="form.ali_access_key_secret" type="password" show-password-on="click" placeholder="AccessKey Secret" />
      </SettingRow>
      <SettingRow label="短信签名" description="已审核通过的签名名称，不含【】" required>
        <NInput v-model:value="form.ali_sign_name" placeholder="如 某某科技" />
      </SettingRow>
      <SettingRow label="模板 CODE" required>
        <template #description>
          模板内容示例：您的验证码为 <span v-pre class="rl-kbd">${code}</span>，<span v-pre class="rl-kbd">${time}</span> 分钟内有效
        </template>
        <NInput v-model:value="form.ali_template_code" placeholder="SMS_123456789" class="rl-mono" />
      </SettingRow>
    </RlPanel>

    <!-- 腾讯云 -->
    <RlPanel v-if="form.provider === 'tencent'" title="腾讯云短信" subtitle="在腾讯云控制台「短信」与「访问管理」中获取以下信息" :padded="false">
      <SettingRow label="SecretId" required>
        <NInput v-model:value="form.tencent_secret_id" placeholder="AKID…" class="rl-mono">
          <template #prefix><Key class="text-tertiary" /></template>
        </NInput>
      </SettingRow>
      <SettingRow label="SecretKey" description="保持掩码不变表示不修改" required>
        <NInput v-model:value="form.tencent_secret_key" type="password" show-password-on="click" placeholder="SecretKey" />
      </SettingRow>
      <SettingRow label="地域" description="短信服务所在地域，如 ap-guangzhou、ap-beijing、ap-shanghai" required>
        <NInput v-model:value="form.tencent_region" placeholder="ap-guangzhou" class="rl-mono" />
      </SettingRow>
      <SettingRow label="SDKAppID" description="短信应用的 SDKAppID" required>
        <NInput v-model:value="form.tencent_app_id" placeholder="1400006666" class="rl-mono" />
      </SettingRow>
      <SettingRow label="短信签名" description="已审核通过的签名内容" required>
        <NInput v-model:value="form.tencent_sign_name" placeholder="如 某某科技" />
      </SettingRow>
      <SettingRow label="模板 ID" required>
        <template #description>
          模板内容示例：您的验证码为 <span v-pre class="rl-kbd">{1}</span>，<span v-pre class="rl-kbd">{2}</span> 分钟内有效
        </template>
        <NInput v-model:value="form.tencent_template_id" placeholder="1234567" class="rl-mono" />
      </SettingRow>
    </RlPanel>

    <!-- 测试发送 -->
    <RlPanel title="发送测试短信" subtitle="使用已保存的配置向指定手机号发送一条验证码短信">
      <div class="flex flex-wrap items-center gap-10px">
        <NInput v-model:value="phone" placeholder="接收手机号，如 13800138000" :disabled="testDisabled" class="w-280px!">
          <template #prefix><Phone class="text-tertiary" /></template>
        </NInput>
        <NButton :disabled="testDisabled" :loading="testing" @click="sendTest">
          <template #icon><Send /></template>发送测试
        </NButton>
        <span v-if="!saved.provider" class="text-sm text-tertiary">短信服务启用并保存后才能测试</span>
        <span v-else-if="dirty" class="text-sm text-warning">请先保存修改，测试会使用已保存的配置</span>
      </div>
    </RlPanel>
  </div>

  <BrandSaveBar :dirty="dirty" :saving="saving" @reset="reset" @save="save" />
</template>
