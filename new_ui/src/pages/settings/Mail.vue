<script setup lang="ts">
// 邮件（SMTP）：顶部通道状态 + 就绪检查，下方服务器 / 加密方式 / 发件身份配置
import { computed } from 'vue'
import { NInput, NInputNumber } from 'naive-ui'
import MailIcon from '~icons/fluent/mail-20-regular'
import Server from '~icons/fluent/server-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import Shield from '~icons/fluent/shield-checkmark-20-regular'
import Globe from '~icons/fluent/globe-20-regular'
import Checkmark from '~icons/fluent/checkmark-circle-20-regular'
import Warning from '~icons/fluent/warning-20-regular'
import ArrowRight from '~icons/fluent/arrow-right-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import StatusDot from '@/components/StatusDot.vue'
import BrandStatusCard from './components/BrandStatusCard.vue'
import BrandChoiceCard from './components/BrandChoiceCard.vue'
import BrandSaveBar from './components/BrandSaveBar.vue'
import { useSettingForm } from './components/BrandUseForm'
import { MAIL_ENCRYPTIONS, MAIL_PRESETS, mailChecks, type SmtpForm } from './components/MailDefs'
import { get, post } from '@/api/http'

const empty = (): SmtpForm => ({ host: '', port: null, username: '', password: '', from: '', encryption: 'None' })
const { form, saved, dirty, loading, saving, save, reset } = useSettingForm<SmtpForm>(
  empty,
  {
    fetch: async () => {
      const d = { ...empty(), ...((await get('/api/v1/set/other/smtp')) || {}) }
      if (!d.encryption) d.encryption = 'None'
      return d
    },
    submit: v => post('/api/v1/set/other/smtp/edit', v),
    validate: v => {
      const miss = [['host', '服务器地址'], ['port', '端口'], ['username', '登录账号'], ['from', '发件人地址']]
        .filter(([k]) => (v as any)[k] === '' || (v as any)[k] == null).map(([, l]) => l)
      return miss.length ? `请完善必填项：${miss.join('、')}` : null
    },
    successText: 'SMTP 配置已保存',
  },
)

// 状态基于已保存配置
const checks = computed(() => mailChecks(saved.value))
const ready = computed(() => checks.value.filter(c => c.required).every(c => c.ok))
const plain = computed(() => saved.value.encryption === 'None' || !saved.value.encryption)
const tone = computed(() => (!ready.value ? 'warning' : plain.value ? 'info' : 'success'))
const encLabel = (v?: string) => MAIL_ENCRYPTIONS.find(e => e.value === v)?.label || '不加密'

// 切换加密方式时，若端口仍是默认端口（或为空），顺带切换到对应默认端口
function pickEncryption(v: string) {
  const defaults = MAIL_ENCRYPTIONS.map(e => e.port)
  if (form.value.port == null || defaults.includes(form.value.port)) form.value.port = MAIL_ENCRYPTIONS.find(e => e.value === v)!.port
  form.value.encryption = v
}
function applyPreset(p: (typeof MAIL_PRESETS)[number]) {
  form.value.host = p.host
  form.value.port = p.port
  form.value.encryption = p.encryption
}
</script>

<template>
  <PageHeader title="邮件" description="配置发信用的 SMTP 服务器，用于账号开通通知、证书下发和找回密码" />

  <div class="grid gap-16px" :class="loading && 'pointer-events-none opacity-60'">
    <!-- 通道状态 -->
    <BrandStatusCard title="邮件通道" :tone="tone">
      <template #icon><MailIcon /></template>
      <template #status>
        <StatusDot v-if="!ready" tone="warning">未就绪</StatusDot>
        <StatusDot v-else-if="plain" tone="info">可用 · 明文传输</StatusDot>
        <StatusDot v-else tone="success" pulse>可用</StatusDot>
      </template>
      <template v-if="ready">
        系统邮件将通过 <b class="rl-mono text-primary">{{ saved.host }}:{{ saved.port }}</b> 以
        <b class="text-primary">{{ saved.from }}</b> 的身份发出。
      </template>
      <template v-else>尚未完成 SMTP 配置，新用户开通通知、证书邮件和找回密码邮件都无法送达。</template>
      <template #meta>
        <span class="ax-badge text-secondary"><LockClosed class="mr-4px" />{{ encLabel(saved.encryption) }}</span>
        <span class="ax-badge text-secondary">账号 {{ saved.username || '未设置' }}</span>
        <RouterLink to="/admin/settings/templates" class="ml-4px inline-flex items-center gap-4px text-md text-info hover:underline">
          编辑邮件模板<ArrowRight />
        </RouterLink>
      </template>
      <template #aside>
        <div class="mb-10px text-sm font-semibold text-secondary">就绪检查</div>
        <ul class="m-0 grid list-none gap-8px p-0">
          <li v-for="c in checks" :key="c.label" class="flex items-center gap-8px text-md">
            <Checkmark v-if="c.ok" class="flex-shrink-0 text-success" />
            <Warning v-else class="flex-shrink-0" :class="c.required ? 'text-warning' : 'text-tertiary'" />
            <span :class="c.ok ? 'text-primary' : 'text-secondary'">{{ c.label }}</span>
            <span class="ml-auto truncate text-sm text-tertiary">{{ c.detail }}</span>
          </li>
        </ul>
        <div class="mt-12px text-sm text-tertiary">后端未提供 SMTP 连通性测试，可在「用户」中给自己发送一封开通邮件验证。</div>
      </template>
    </BrandStatusCard>

    <!-- 服务器 -->
    <RlPanel title="SMTP 服务器" subtitle="填写邮件服务商提供的发信服务器" :padded="false">
      <SettingRow label="常用服务商" description="点击即可填入服务器、端口与加密方式">
        <div class="flex flex-wrap gap-8px">
          <button v-for="p in MAIL_PRESETS" :key="p.name" type="button" class="mail-chip" @click="applyPreset(p)">{{ p.name }}</button>
        </div>
      </SettingRow>
      <SettingRow label="服务器地址" description="发信服务器的域名或 IP" required>
        <NInput v-model:value="form.host" placeholder="smtp.example.com" class="rl-mono">
          <template #prefix><Server class="text-tertiary" /></template>
        </NInput>
      </SettingRow>
      <SettingRow label="端口" description="切换加密方式时会自动带出常用端口" required>
        <NInputNumber v-model:value="form.port" :show-button="false" :min="1" :max="65535" placeholder="465" class="w-160px!" />
      </SettingRow>
      <div class="b-t-1px b-t-solid b-[var(--color-border-default)] px-20px py-18px">
        <div class="mb-10px text-md font-semibold">加密方式</div>
        <div class="grid grid-cols-1 gap-12px md:grid-cols-3">
          <BrandChoiceCard
            v-for="e in MAIL_ENCRYPTIONS" :key="e.value" :title="e.label" :active="(form.encryption || 'None') === e.value"
            @select="pickEncryption(e.value)"
          >
            <template #icon><component :is="e.value === 'None' ? Globe : e.value === 'SSLTLS' ? LockClosed : Shield" /></template>
            {{ e.desc }}<span class="rl-kbd ml-6px">{{ e.port }}</span>
          </BrandChoiceCard>
        </div>
      </div>
    </RlPanel>

    <!-- 发件身份 -->
    <RlPanel title="发件身份" subtitle="登录 SMTP 的凭据，以及收件人看到的发件地址" :padded="false">
      <SettingRow label="登录账号" description="通常就是完整的邮箱地址" required>
        <NInput v-model:value="form.username" placeholder="noreply@example.com" />
      </SettingRow>
      <SettingRow label="密码 / 授权码" description="多数邮箱需使用「客户端授权码」而非登录密码；保持掩码不变表示不修改">
        <NInput v-model:value="form.password" type="password" show-password-on="click" placeholder="未设置" />
      </SettingRow>
      <SettingRow label="发件人地址" description="邮件 From 头，需与登录账号匹配，否则可能被服务商拒收" required>
        <NInput v-model:value="form.from" placeholder="noreply@example.com" />
      </SettingRow>
    </RlPanel>
  </div>

  <BrandSaveBar :dirty="dirty" :saving="saving" @reset="reset" @save="save" />
</template>

<style scoped>
.mail-chip {
  height: 28px; padding: 0 12px; border: 1px solid var(--color-border-default); border-radius: 14px;
  background: transparent; color: var(--color-text-secondary); font: inherit; font-size: 12px; cursor: pointer;
}
.mail-chip:hover { border-color: var(--color-border-strong); color: var(--color-text-primary); }
</style>
