<script setup lang="ts">
// ACME 签发向导：DNS 服务商 → 凭据 → 域名与邮箱 → 签发
import { computed, ref } from 'vue'
import { NAlert, NButton, NCollapseTransition, NInput, NStep, NSteps, NSwitch } from 'naive-ui'
import Checkmark from '~icons/fluent/checkmark-circle-20-filled'
import ChevronDown from '~icons/fluent/chevron-down-20-regular'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import { type LetsForm, PROVIDERS, providerOf } from './acme'
import { errMsg, post } from '@/api/http'
import { ui } from '@/utils/ui'

const props = defineProps<{ webvpnDomain: string; editing: boolean }>()
const form = defineModel<LetsForm>('form', { required: true })
const emit = defineEmits<{ cancel: []; done: [] }>()

const step = ref(form.value.name ? 2 : 1)
const applying = ref(false)
const advanced = ref(!!form.value.dnsResolver)

const provider = computed(() => providerOf(form.value.name))
const creds = computed(() => (form.value.name ? (form.value[form.value.name] as Record<string, string>) : {}))
const wildName = computed(() => (props.webvpnDomain ? `*.${props.webvpnDomain}` : ''))
const target = computed(() => (form.value.certType === 'wild' ? wildName.value : form.value.domain.trim()))

function pick(v: LetsForm['name']) {
  form.value.name = v
  step.value = 2
}

// 每步校验，返回错误文本
function check(s: number) {
  const f = form.value
  if (s === 1 && !f.name) return '请选择 DNS 服务商'
  if (s === 2) for (const fd of provider.value?.fields || []) if (!creds.value[fd.key]?.trim()) return `请填写 ${fd.label}`
  if (s === 3) {
    if (f.certType === 'wild' && !props.webvpnDomain) return '未配置 WebVPN 域名，无法申请泛域名证书'
    if (f.certType === 'main' && !f.domain.trim()) return '请填写证书域名'
    if (!f.legomail.trim()) return '请填写邮箱'
  }
  return ''
}
function next() {
  const err = check(step.value)
  if (err) return ui.message.warning(err)
  step.value++
}

async function issue() {
  for (const s of [1, 2, 3]) {
    const err = check(s)
    if (err) { step.value = s; return ui.message.warning(err) }
  }
  applying.value = true
  try {
    await post('/api/v1/set/other/createcert', form.value)
    ui.message.success('证书签发成功，已自动部署')
    emit('done')
  } catch (e) {
    ui.message.error(errMsg(e, '签发失败'))
  } finally {
    applying.value = false
  }
}

const certTypes = computed(() => [
  { value: 'main' as const, title: '主证书', desc: '门户、管理后台与 VPN 接入使用的单域名证书' },
  { value: 'wild' as const, title: 'WebVPN 泛域名证书', desc: wildName.value ? `签发 ${wildName.value}` : '需先配置 WebVPN 域名', disabled: !props.webvpnDomain },
])
</script>

<template>
  <RlPanel :title="editing ? '编辑签发配置' : '配置自动签发'" subtitle="Let's Encrypt 通过 DNS-01 验证域名所有权，无需开放 80 端口">
    <template v-if="editing" #extra>
      <NButton text :disabled="applying" @click="emit('cancel')">取消编辑</NButton>
    </template>

    <NSteps :current="step" size="small" class="mb-28px">
      <NStep title="DNS 服务商" />
      <NStep title="API 凭据" />
      <NStep title="域名与邮箱" />
      <NStep title="签发" />
    </NSteps>

    <div class="max-w-720px">
      <!-- 1. 服务商 -->
      <div v-if="step === 1">
        <div class="mb-12px text-md text-secondary">你的域名解析托管在哪里？RemLink 会在签发时自动添加并清理 TXT 验证记录。</div>
        <div class="grid gap-10px sm:grid-cols-3">
          <button
            v-for="p in PROVIDERS" :key="p.value" type="button"
            class="acme-opt flex flex-col gap-10px rounded-10px bg-primary p-14px text-left" :class="form.name === p.value && 'acme-opt--on'"
            @click="pick(p.value)"
          >
            <div class="flex items-center justify-between">
              <span class="rl-mono h-36px w-36px flex items-center justify-center rounded-8px bg-fill font-semibold">{{ p.mark }}</span>
              <Checkmark v-if="form.name === p.value" class="text-lg" />
            </div>
            <div>
              <div class="text-md font-semibold">{{ p.label }}</div>
              <div class="mt-2px text-sm text-secondary">{{ p.desc }}</div>
            </div>
          </button>
        </div>
      </div>

      <!-- 2. 凭据 -->
      <div v-else-if="step === 2" class="grid gap-16px">
        <div class="text-md text-secondary">填写 {{ provider?.label }} 的 API 凭据，仅用于签发与续期时修改 DNS 记录。<template v-if="editing">已保存的密钥以掩码显示，不修改则保留原值。</template></div>
        <div v-for="fd in provider?.fields || []" :key="form.name + fd.key">
          <div class="mb-6px text-md font-medium">{{ fd.label }}</div>
          <NInput
            v-model:value="creds[fd.key]" :type="fd.secret ? 'password' : 'text'" show-password-on="click"
            :placeholder="fd.label" class="rl-mono" :input-props="{ autocomplete: 'off' }"
          />
          <div class="mt-4px text-sm text-tertiary">{{ fd.hint }}</div>
        </div>
        <div>
          <button type="button" class="acme-link flex items-center gap-4px text-sm text-secondary" @click="advanced = !advanced">
            <ChevronDown class="transition-transform" :class="!advanced && '-rotate-90'" />高级：自定义验证用 DNS 服务器
          </button>
          <NCollapseTransition :show="advanced">
            <div class="pt-10px">
              <NInput v-model:value="form.dnsResolver" placeholder="223.6.6.6,223.5.5.5" class="rl-mono" />
              <div class="mt-4px text-sm text-tertiary">签发前查询 TXT 记录是否生效，多个以逗号分隔；留空使用阿里 DNS（223.6.6.6、223.5.5.5）。</div>
            </div>
          </NCollapseTransition>
        </div>
      </div>

      <!-- 3. 域名与邮箱 -->
      <div v-else-if="step === 3" class="grid gap-20px">
        <div>
          <div class="mb-8px text-md font-medium">签发哪张证书</div>
          <div class="grid gap-10px sm:grid-cols-2">
            <button
              v-for="t in certTypes" :key="t.value" type="button" :disabled="t.disabled"
              class="acme-opt rounded-10px bg-primary p-12px text-left" :class="form.certType === t.value && 'acme-opt--on'"
              @click="form.certType = t.value"
            >
              <div class="flex items-center justify-between gap-6px">
                <span class="text-md font-semibold">{{ t.title }}</span>
                <Checkmark v-if="form.certType === t.value" class="text-lg" />
              </div>
              <div class="mt-2px text-sm text-secondary">{{ t.desc }}</div>
            </button>
          </div>
        </div>
        <div>
          <div class="mb-6px text-md font-medium">证书域名</div>
          <NInput v-if="form.certType === 'wild'" :value="wildName" disabled class="rl-mono" />
          <NInput v-else v-model:value="form.domain" placeholder="vpn.example.com" class="rl-mono" />
          <div class="mt-4px text-sm text-tertiary">
            {{ form.certType === 'wild' ? '由「WebVPN 域名」决定，如需修改请前往 WebVPN 设置' : '用户访问门户和连接 VPN 时使用的完整域名，需已解析到本服务器' }}
          </div>
        </div>
        <div>
          <div class="mb-6px text-md font-medium">联系邮箱</div>
          <NInput v-model:value="form.legomail" placeholder="admin@example.com" />
          <div class="mt-4px text-sm text-tertiary">用于注册 Let's Encrypt 账号并接收到期提醒</div>
        </div>
        <div class="flex items-center justify-between gap-16px rounded-10px bg-fill px-14px py-12px">
          <div>
            <div class="text-md font-medium">自动续期</div>
            <div class="text-sm text-secondary">证书有效期 90 天，开启后到期前自动重新签发并部署</div>
          </div>
          <NSwitch v-model:value="form.renew" />
        </div>
      </div>

      <!-- 4. 确认签发 -->
      <div v-else class="grid gap-16px">
        <div class="rounded-10px b-1px b-[var(--color-border-default)] b-solid">
          <div class="px-16px py-14px">
            <div class="text-sm text-secondary">即将签发</div>
            <div class="rl-mono mt-2px break-all text-xl font-semibold">{{ target || '—' }}</div>
          </div>
          <dl class="m-0 grid grid-cols-[88px_minmax(0,1fr)] gap-x-12px gap-y-10px b-t-1px b-t-solid b-[var(--color-border-default)] px-16px py-14px text-md">
            <dt class="text-secondary">证书类型</dt>
            <dd class="m-0">{{ form.certType === 'wild' ? 'WebVPN 泛域名证书' : '主证书' }}</dd>
            <dt class="text-secondary">验证方式</dt>
            <dd class="m-0">DNS-01 · {{ provider?.label }}</dd>
            <dt class="text-secondary">联系邮箱</dt>
            <dd class="m-0 break-all">{{ form.legomail }}</dd>
            <dt class="text-secondary">DNS 服务器</dt>
            <dd class="rl-mono m-0">{{ form.dnsResolver || '默认（223.6.6.6, 223.5.5.5）' }}</dd>
            <dt class="text-secondary">自动续期</dt>
            <dd class="m-0"><StatusDot :tone="form.renew ? 'success' : 'muted'">{{ form.renew ? '开启' : '关闭' }}</StatusDot></dd>
          </dl>
        </div>
        <NAlert type="info" :bordered="false">
          {{ applying ? '正在添加 DNS 记录并等待生效，通常需要 1–2 分钟，请勿关闭页面或重复提交…' : '签发成功后证书会立即替换当前对应的服务器证书，配置同时保存。' }}
        </NAlert>
      </div>
    </div>

    <template #footer>
      <span class="text-sm text-secondary">第 {{ step }} / 4 步</span>
      <div class="flex gap-sm">
        <NButton v-if="step > 1" :disabled="applying" @click="step--">上一步</NButton>
        <NButton v-if="step < 4" type="primary" :disabled="!form.name" @click="next">下一步</NButton>
        <NButton v-if="step === 4" type="primary" :loading="applying" @click="issue">开始签发</NButton>
      </div>
    </template>
  </RlPanel>
</template>

<style scoped>
.acme-opt { border: 1px solid var(--color-border-default); cursor: pointer; font: inherit; color: inherit; transition: border-color .15s; }
.acme-opt:hover:not(:disabled) { border-color: var(--color-border-strong); }
.acme-opt--on { border-color: var(--color-text-primary); box-shadow: inset 0 0 0 1px var(--color-text-primary); }
.acme-opt:disabled { cursor: not-allowed; opacity: .5; }
.acme-link { border: 0; background: none; padding: 0; cursor: pointer; font: inherit; }
.acme-link:hover { color: var(--color-text-primary); }
</style>
