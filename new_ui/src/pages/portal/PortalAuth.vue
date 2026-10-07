<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { NButton, NForm, NFormItem, NIcon, NInput, type FormInst, type FormRules } from 'naive-ui'
import Person from '~icons/fluent/person-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import Phone from '~icons/fluent/phone-20-regular'
import Mail from '~icons/fluent/mail-20-regular'
import ArrowLeft from '~icons/fluent/arrow-left-20-regular'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-regular'
import ShieldCheckmark from '~icons/fluent/shield-checkmark-20-regular'
import PeopleTeam from '~icons/fluent/people-team-20-regular'
import Building from '~icons/fluent/building-20-regular'
import ChatMultiple from '~icons/fluent/chat-multiple-20-regular'
import PeopleCommunity from '~icons/fluent/people-community-20-regular'
import { ui } from '@/utils/ui'
import PwdStrength from './PwdStrength.vue'
import { isWebPath, parseJson, portalApi, safeRedirect, validatePassword, type PortalBrand, type PortalResp } from './shared'

import type { AuthPreset } from './store'

// 门户未登录时的认证区：密码 / 短信 / 二次验证 / 首登改密 / 找回 / 重置

const props = defineProps<{ brand: PortalBrand; webvpnMode: boolean; preset: AuthPreset }>()
const emit = defineEmits<{ pass: [data: any]; smsPass: [] }>()
const route = useRoute()

type Mode = 'login' | 'sms' | 'challenge' | 'change_pwd' | 'forgot' | 'reset'
const mode = ref<Mode>('login')

// 父组件在挂载时解析出的 SSO 回调挑战 / 重置链接
watch(() => ({ ...props.preset }), p => {
  if (p.mode === 'challenge') {
    challenge.type = p.challengeType || 'otp'
    challenge.session = p.session || ''
    challenge.code = ''
    mode.value = 'challenge'
  } else if (p.mode === 'reset') {
    reset.token = p.resetToken || ''
    reset.username = p.resetUsername || '未知用户'
    mode.value = 'reset'
  }
}, { immediate: true })

// ---------- 介绍要点 ----------
const featureIcons = [CheckmarkCircle, PeopleTeam, ShieldCheckmark]
const showFeatures = computed(() => props.brand.features_enabled !== 2)
const features = computed(() => {
  const list = parseJson<any[]>(props.brand.features, [])
  if (Array.isArray(list) && list.length) return list as { label: string; desc: string }[]
  return [
    { label: '账号状态', desc: '查看在线设备与连接状态' },
    { label: '用户分组', desc: '查看所属用户分组' },
    { label: '安全设置', desc: '自主修改密码与验证方式' },
  ]
})

// ---------- 密码登录 ----------
const loginRef = ref<FormInst>()
const loginForm = reactive({ username: '', password: '' })
const loginRules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}
const loginLoading = ref(false)

async function submitLogin() {
  if (loginLoading.value) return
  try { await loginRef.value?.validate() } catch { return }
  loginLoading.value = true
  try {
    handleAuth(await portalApi('post', '/api/v1/portal/login', loginForm))
  } catch {
    ui.message.error('网络请求失败，请稍后重试')
  } finally {
    loginLoading.value = false
  }
}

// 统一处理认证响应：通过 / 改密 / 二次验证
function handleAuth(resp: PortalResp) {
  if (resp.code !== 0) {
    ui.message.error(resp.msg)
    return
  }
  const d = resp.data || {}
  if (d.status === 'pass') {
    challenge.code = ''
    challenge.session = ''
    loginForm.password = ''
    emit('pass', d)
    return
  }
  if (d.status === 'change_pwd') {
    force.token = d.token || ''
    force.new_password = ''
    force.confirm_password = ''
    loginForm.password = ''
    mode.value = 'change_pwd'
    nextTick(() => forceInput.value?.focus())
    return
  }
  if (['otp', 'radius', 'sms', 'verify'].includes(d.status)) {
    challenge.type = d.status
    challenge.session = d.session_id
    challenge.code = ''
    mode.value = 'challenge'
    nextTick(() => challengeInput.value?.focus())
    return
  }
  ui.message.warning(d.message || '需要继续认证')
}

// ---------- 二次验证 ----------
const challenge = reactive({ type: '', session: '', code: '' })
const challengeLoading = ref(false)
const challengeInput = ref<InstanceType<typeof NInput>>()
const challengeTitle = computed(() => (challenge.type === 'sms' ? '短信验证' : '两步验证'))
const challengeDesc = computed(() => {
  if (challenge.type === 'sms') return '验证码已发送至您的手机'
  if (challenge.type === 'radius') return '请输入动态验证码'
  return '已启用二次认证，请输入 6 位 TOTP 动态码'
})
const challengePlaceholder = computed(() =>
  challenge.type === 'sms' ? '请输入短信验证码' : challenge.type === 'radius' ? '请输入二次验证码' : '6 位动态验证码')

async function submitChallenge() {
  if (!challenge.code) {
    ui.message.warning('请输入验证码')
    return
  }
  challengeLoading.value = true
  try {
    handleAuth(await portalApi('post', '/api/v1/portal/verify', { session_id: challenge.session, code: challenge.code }))
  } catch {
    ui.message.error('验证失败，请重试')
  } finally {
    challengeLoading.value = false
  }
}

function cancelChallenge() {
  mode.value = 'login'
  challenge.code = ''
  challenge.session = ''
}

// ---------- 首次登录强制改密 ----------
const force = reactive({ token: '', new_password: '', confirm_password: '' })
const forceLoading = ref(false)
const forceInput = ref<InstanceType<typeof NInput>>()

async function submitForceChange() {
  if (!force.new_password) return ui.message.warning('请输入新密码')
  const err = validatePassword(force.new_password)
  if (err) return ui.message.warning(err)
  if (force.new_password !== force.confirm_password) return ui.message.warning('两次输入的新密码不一致')
  forceLoading.value = true
  try {
    const r = await portalApi('post', '/api/v1/portal/force_change_password', {
      token: force.token,
      new_password: force.new_password,
      new_password_confirm: force.confirm_password,
    })
    // 改密后可能直接登录，或继续二次认证
    handleAuth(r)
  } catch {
    ui.message.error('请求失败，请稍后重试')
  } finally {
    forceLoading.value = false
  }
}

// ---------- 短信登录 ----------
const sms = reactive({ phone: '', code: '', sent: false, sending: false, verifying: false, countdown: 0, tail: '' })
let smsTimer: ReturnType<typeof setInterval> | null = null
const smsPhoneInput = ref<InstanceType<typeof NInput>>()
const smsCodeInput = ref<InstanceType<typeof NInput>>()

function stopSmsTimer() {
  if (smsTimer) clearInterval(smsTimer)
  smsTimer = null
}
onBeforeUnmount(stopSmsTimer)

function switchToSms() {
  mode.value = 'sms'
  Object.assign(sms, { phone: '', code: '', sent: false })
  nextTick(() => smsPhoneInput.value?.focus())
}

function startSmsCountdown() {
  sms.countdown = 60
  stopSmsTimer()
  smsTimer = setInterval(() => {
    if (--sms.countdown <= 0) stopSmsTimer()
  }, 1000)
}

async function sendSmsCode() {
  if (sms.countdown > 0) return
  if (!sms.phone || sms.phone.length < 11) return ui.message.warning('请输入正确的手机号')
  sms.sending = true
  try {
    const r = await portalApi('post', '/api/v1/portal/sms/send', { phone: sms.phone })
    if (r.code !== 0) return ui.message.error(r.msg)
    sms.sent = true
    sms.tail = r.data?.phone_tail || sms.phone.slice(-4)
    sms.code = ''
    startSmsCountdown()
    nextTick(() => smsCodeInput.value?.focus())
  } catch {
    ui.message.error('发送失败，请稍后重试')
  } finally {
    sms.sending = false
  }
}

async function submitSmsLogin() {
  if (sms.code.length !== 6) return
  sms.verifying = true
  try {
    const r = await portalApi('post', '/api/v1/portal/sms/verify', { phone: sms.phone, code: sms.code })
    if (r.code !== 0) return ui.message.error(r.msg)
    challenge.code = ''
    challenge.session = ''
    sms.sent = false
    emit('smsPass')
  } catch {
    ui.message.error('验证失败，请重试')
  } finally {
    sms.verifying = false
  }
}

// ---------- 找回 / 重置密码 ----------
const forgot = reactive({ username: '', email: '', loading: false, sent: false })
const reset = reactive({ token: '', username: '', new_password: '', confirm_password: '', loading: false })

function switchToLogin() {
  mode.value = 'login'
  Object.assign(forgot, { username: '', email: '', sent: false })
  Object.assign(reset, { new_password: '', confirm_password: '' })
  Object.assign(sms, { phone: '', code: '', sent: false, countdown: 0 })
  stopSmsTimer()
}

async function submitForgot() {
  if (!forgot.username || !forgot.email) return ui.message.warning('请输入用户名和邮箱')
  forgot.loading = true
  try {
    const r = await portalApi('post', '/api/v1/portal/forgot_password', { username: forgot.username, email: forgot.email })
    // 不暴露账号是否存在：失败也提示“如果匹配则已发送”
    if (r.code === 0) ui.message.success(r.data?.message || '重置邮件已发送')
    else ui.message.success(r.data?.message || r.msg || '如果账号匹配，重置邮件已发送')
    forgot.sent = true
  } catch {
    ui.message.error('请求失败，请稍后重试')
  } finally {
    forgot.loading = false
  }
}

async function submitReset() {
  if (!reset.new_password) return ui.message.warning('请输入新密码')
  const err = validatePassword(reset.new_password)
  if (err) return ui.message.warning(err)
  if (reset.new_password !== reset.confirm_password) return ui.message.warning('两次输入的密码不一致')
  reset.loading = true
  try {
    const r = await portalApi('post', '/api/v1/portal/reset_password', { token: reset.token, new_password: reset.new_password })
    if (r.code !== 0) return ui.message.error(r.msg)
    ui.message.success('密码重置成功，请使用新密码登录')
    switchToLogin()
  } catch {
    ui.message.error('请求失败，请稍后重试')
  } finally {
    reset.loading = false
  }
}

// ---------- 第三方登录 ----------
const ssoDefs = [
  { type: 'wxwork', label: '企业微信', icon: Building },
  { type: 'feishu', label: '飞书', icon: ChatMultiple },
  { type: 'dingtalk', label: '钉钉', icon: PeopleCommunity },
]
const ssoList = computed(() => ssoDefs.filter(s => (props.brand.sso_types || []).includes(s.type)))
// WebVPN 子域名场景需跳到专用门户域名完成三方认证，未配置该域名时不提供入口；
// 路径模式（redirect 为 /web/…）本就在主域名上，三方登录可直接使用
const hasSso = computed(() => ssoList.value.length > 0
  && !(props.webvpnMode && !props.brand.webvpn_sso_domain && !isWebPath(safeRedirect(route.query.redirect))))

function startSSO(type: string) {
  // 统一请求当前域名的相对路径，由后端决定是否转到门户域名完成认证
  let href = '/api/v1/portal/sso?type=' + encodeURIComponent(type)
  const rp = safeRedirect(route.query.redirect)
  if (rp) href += '&redirect=' + encodeURIComponent(window.location.origin + rp)
  window.location.href = href
}

const heading = computed(() => ({
  login: '登录用户中心',
  sms: '短信验证码登录',
  challenge: challengeTitle.value,
  change_pwd: '设置新密码',
  forgot: '找回密码',
  reset: '重置密码',
}[mode.value]))

const subheading = computed(() => {
  switch (mode.value) {
    case 'login': return props.brand.desc || `使用你的账号登录 ${props.brand.title || 'RemLink'}`
    case 'challenge': return challengeDesc.value
    case 'change_pwd': return '首次登录需设置新密码：至少 8 位，且须包含字母和数字'
    case 'sms': return sms.sent ? `验证码已发送至 ****${sms.tail}` : '使用绑定的手机号接收验证码'
    case 'forgot': return '输入用户名和注册邮箱，我们会发送重置链接'
    case 'reset': return `正在为 ${reset.username} 设置新密码`
  }
  return ''
})
const backLabel = computed(() => (mode.value === 'challenge' ? '返回' : mode.value === 'sms' ? '返回密码登录' : '返回登录'))
function goBack() {
  if (mode.value === 'challenge') cancelChallenge()
  else switchToLogin()
}
</script>

<template>
  <div class="w-full">
    <!-- 返回（非首屏步骤） -->
    <NButton v-if="mode !== 'login' && mode !== 'change_pwd'" text class="mb-16px text-secondary hover:text-primary!" @click="goBack">
      <template #icon><NIcon><ArrowLeft /></NIcon></template>
      {{ backLabel }}
    </NButton>

    <h1 class="m-0 text-4xl font-semibold leading-32px">{{ heading }}</h1>
    <p class="mb-24px mt-6px text-md text-secondary">{{ subheading }}</p>

    <!-- 密码登录 -->
    <template v-if="mode === 'login'">
      <NForm ref="loginRef" :model="loginForm" :rules="loginRules" label-placement="top" @submit.prevent>
        <NFormItem path="username" label="用户名">
          <NInput v-model:value="loginForm.username" placeholder="请输入用户名" :input-props="{ autocomplete: 'username' }" @keydown.enter.prevent="submitLogin">
            <template #prefix><NIcon class="text-secondary"><Person /></NIcon></template>
          </NInput>
        </NFormItem>
        <NFormItem path="password" label="密码">
          <NInput
            v-model:value="loginForm.password" type="password" show-password-on="click" placeholder="请输入密码"
            :input-props="{ autocomplete: 'current-password' }" @keydown.enter.prevent="submitLogin"
          >
            <template #prefix><NIcon class="text-secondary"><LockClosed /></NIcon></template>
          </NInput>
        </NFormItem>
      </NForm>
      <NButton type="primary" block class="mt-4px" :loading="loginLoading" @click="submitLogin">登录</NButton>
      <div class="mt-16px flex items-center justify-between text-md">
        <NButton text class="text-secondary hover:text-primary!" @click="mode = 'forgot'">忘记密码？</NButton>
        <NButton v-if="brand.sms_enabled" text class="text-secondary hover:text-primary!" @click="switchToSms">短信验证码登录</NButton>
      </div>

      <template v-if="hasSso">
        <div class="my-20px flex items-center gap-10px text-sm text-tertiary">
          <span class="h-1px flex-1 bg-[var(--color-border-default)]" />或使用以下方式登录<span class="h-1px flex-1 bg-[var(--color-border-default)]" />
        </div>
        <div class="grid gap-10px">
          <NButton v-for="s in ssoList" :key="s.type" block @click="startSSO(s.type)">
            <template #icon><NIcon><component :is="s.icon" /></NIcon></template>
            使用{{ s.label }}登录
          </NButton>
        </div>
      </template>
    </template>

    <!-- 短信登录 -->
    <template v-else-if="mode === 'sms'">
      <template v-if="!sms.sent">
        <NFormItem label="手机号" :show-feedback="false">
          <NInput
            ref="smsPhoneInput" v-model:value="sms.phone" placeholder="请输入手机号" maxlength="11"
            :input-props="{ inputmode: 'tel' }" @keydown.enter.prevent="sendSmsCode"
          >
            <template #prefix><NIcon class="text-secondary"><Phone /></NIcon></template>
          </NInput>
        </NFormItem>
        <NButton type="primary" block class="mt-20px" :loading="sms.sending" :disabled="!sms.phone" @click="sendSmsCode">发送验证码</NButton>
      </template>
      <template v-else>
        <NInput
          ref="smsCodeInput" v-model:value="sms.code" placeholder="000000" maxlength="6"
          class="rl-mono text-center tracking-[.5em]" :input-props="{ inputmode: 'numeric', autocomplete: 'one-time-code' }"
          @keydown.enter.prevent="submitSmsLogin"
        />
        <NButton type="primary" block class="mt-16px" :loading="sms.verifying" :disabled="sms.code.length !== 6" @click="submitSmsLogin">登录</NButton>
        <div class="mt-16px text-center">
          <NButton text :disabled="sms.countdown > 0" class="text-secondary hover:text-primary!" @click="sendSmsCode">
            {{ sms.countdown > 0 ? `${sms.countdown}s 后重新发送` : '重新发送验证码' }}
          </NButton>
        </div>
      </template>
    </template>

    <!-- 二次验证 -->
    <template v-else-if="mode === 'challenge'">
      <NInput
        ref="challengeInput" v-model:value="challenge.code" :placeholder="challengePlaceholder"
        :maxlength="challenge.type === 'radius' ? undefined : 6"
        :class="challenge.type === 'radius' ? 'rl-mono' : 'rl-mono text-center tracking-[.5em]'"
        :input-props="{ inputmode: challenge.type === 'radius' ? 'text' : 'numeric', autocomplete: 'one-time-code' }"
        @keydown.enter.prevent="submitChallenge"
      />
      <NButton type="primary" block class="mt-16px" :loading="challengeLoading" @click="submitChallenge">验证</NButton>
    </template>

    <!-- 首次登录改密 -->
    <template v-else-if="mode === 'change_pwd'">
      <NForm label-placement="top" :show-feedback="false" class="grid gap-16px" @submit.prevent>
        <NFormItem label="新密码">
          <NInput
            ref="forceInput" v-model:value="force.new_password" type="password" show-password-on="click" placeholder="至少 8 位，含字母和数字"
            :input-props="{ autocomplete: 'new-password' }" @keydown.enter.prevent="submitForceChange"
          />
        </NFormItem>
        <NFormItem label="确认新密码">
          <NInput
            v-model:value="force.confirm_password" type="password" show-password-on="click" placeholder="再次输入新密码"
            :input-props="{ autocomplete: 'new-password' }" @keydown.enter.prevent="submitForceChange"
          />
        </NFormItem>
        <PwdStrength :password="force.new_password" />
      </NForm>
      <NButton type="primary" block class="mt-20px" :loading="forceLoading" @click="submitForceChange">修改并继续</NButton>
    </template>

    <!-- 找回密码 -->
    <template v-else-if="mode === 'forgot'">
      <NForm label-placement="top" :show-feedback="false" class="grid gap-16px" @submit.prevent>
        <NFormItem label="用户名">
          <NInput v-model:value="forgot.username" placeholder="请输入用户名">
            <template #prefix><NIcon class="text-secondary"><Person /></NIcon></template>
          </NInput>
        </NFormItem>
        <NFormItem label="注册邮箱">
          <NInput v-model:value="forgot.email" placeholder="name@company.com" @keydown.enter.prevent="submitForgot">
            <template #prefix><NIcon class="text-secondary"><Mail /></NIcon></template>
          </NInput>
        </NFormItem>
      </NForm>
      <NButton type="primary" block class="mt-20px" :loading="forgot.loading" @click="submitForgot">发送重置邮件</NButton>
      <div v-if="forgot.sent" class="mt-16px flex items-center gap-8px text-md text-secondary">
        <NIcon class="text-success"><CheckmarkCircle /></NIcon>如果账号与邮箱匹配，重置邮件已发送，请查收
      </div>
    </template>

    <!-- 重置密码 -->
    <template v-else-if="mode === 'reset'">
      <NForm label-placement="top" :show-feedback="false" class="grid gap-16px" @submit.prevent>
        <NFormItem label="新密码">
          <NInput
            v-model:value="reset.new_password" type="password" show-password-on="click"
            placeholder="至少 8 位，含字母和数字" :input-props="{ autocomplete: 'new-password' }"
          />
        </NFormItem>
        <NFormItem label="确认新密码">
          <NInput
            v-model:value="reset.confirm_password" type="password" show-password-on="click"
            placeholder="再次输入新密码" :input-props="{ autocomplete: 'new-password' }" @keydown.enter.prevent="submitReset"
          />
        </NFormItem>
        <PwdStrength :password="reset.new_password" />
      </NForm>
      <NButton type="primary" block class="mt-20px" :loading="reset.loading" @click="submitReset">重置密码</NButton>
    </template>

    <!-- 门户功能要点（仅登录步骤展示） -->
    <ul v-if="mode === 'login' && showFeatures" class="m-0 mt-32px grid list-none gap-12px p-0">
      <li v-for="(f, i) in features" :key="i" class="flex items-start gap-10px">
        <NIcon class="mt-2px text-lg text-tertiary"><component :is="featureIcons[i % featureIcons.length]" /></NIcon>
        <div class="min-w-0">
          <div class="text-md font-medium">{{ f.label }}</div>
          <div class="text-sm text-secondary">{{ f.desc }}</div>
        </div>
      </li>
    </ul>
  </div>
</template>
