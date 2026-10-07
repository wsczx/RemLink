<script setup lang="ts">
import { redirectToVpnOriginIfAdmin } from '@/utils/portalOrigin'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import axios from 'axios'
import { NButton, NIcon, NInput, NSpin } from 'naive-ui'
import Person from '~icons/fluent/person-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import Phone from '~icons/fluent/phone-20-regular'
import Key from '~icons/fluent/key-20-regular'
import Moon from '~icons/fluent/weather-moon-20-regular'
import Sun from '~icons/fluent/weather-sunny-20-regular'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-regular'
import ErrorCircle from '~icons/fluent/error-circle-20-regular'
import { useThemeStore } from '@/stores/theme'
import { applyBrandToDocument, normalizeImageSrc } from '@/utils/brand'
import GroupPicker from './webauth/GroupPicker.vue'

// Web 认证（客户端内嵌浏览器）：state 串起 start → groups/identify → step… → done
// 接口统一用裸 axios + withCredentials:false，不经过后台拦截器
type Step = 'loading' | 'identify' | 'select_group' | 'sms_phone' | 'credentials' | 'otp' | 'sms' | 'radius' | 'change_pwd' | 'done'

const route = useRoute()
const theme = useThemeStore()

const brand = ref<Record<string, any>>({ title: '', logo: '' })
const logo = computed(() => normalizeImageSrc(brand.value.logo))

const state = typeof route.query.state === 'string' ? route.query.state : ''
const step = ref<Step>('loading')
const loading = ref(false)
const loadingText = ref('')
const error = ref('')
const hint = ref('')
const challengeMsg = ref('')
const usernameField = ref('')
const noPassword = ref(false)
const portalUrl = ref('')

const groups = ref<string[]>([])
const fallbackGroups = ref<string[]>([])
const selectedGroup = ref('')
const identifyForm = reactive({ username: '' })
const credForm = reactive({ username: '', password: '' })
const changePwdForm = reactive({ new_password: '', confirm_password: '' })
const phoneForm = reactive({ phone: '' })
const otpForm = reactive({ code: '' })
const smsForm = reactive({ code: '' })
const phoneMasked = ref('')
const smsCountdown = ref(0)
let smsTimer: ReturnType<typeof setInterval> | null = null

const isPhoneValid = computed(() => /^1[3-9]\d{9}$/.test(phoneForm.phone))

const apiPath = (name: string) => `/api/v1/web-auth/${name}?state=${encodeURIComponent(state)}`
const opts = { withCredentials: false }

function loadBrand() {
  axios.get('/api/v1/portal/login-config').then(r => {
    if (r.data?.data) {
      brand.value = Object.assign({ title: '', logo: '', favicon: '' }, r.data.data)
      applyBrandToDocument(brand.value)
    }
  }).catch(() => {})
}

// 每一步进入后聚焦首个输入框
const stepEl = ref<HTMLElement>()
watch(step, () => nextTick(() => stepEl.value?.querySelector<HTMLInputElement>('input:not([disabled])')?.focus()))

// ---------- 初始化 ----------
async function initAuth() {
  error.value = ''
  loadingText.value = '正在检测证书信息...'
  try {
    const { data } = await axios.get(apiPath('start'), opts)
    loadingText.value = ''
    switch (data.status) {
      case 'select_group':
        if (data.require_identify) {
          // 开启组过滤：先收集用户名，再按权限过滤组清单
          fallbackGroups.value = data.groups || []
          step.value = 'identify'
        } else {
          setGroups(data.groups || [])
        }
        return
      case 'identify':
        step.value = 'identify'
        return
      default:
        handleStepResponse(data)
    }
  } catch {
    loadingText.value = ''
    error.value = '认证服务异常，请重试'
  }
}

// 展示组清单；只有一个组时直接提交
function setGroups(list: string[]) {
  groups.value = list
  step.value = 'select_group'
  if (list.length === 1) {
    selectedGroup.value = list[0]
    submitGroup()
  }
}

// ---------- 进入各步骤 ----------
function enterCredentials(d: any) {
  step.value = 'credentials'
  hint.value = d.hint || ''
  usernameField.value = d.username || ''
  noPassword.value = d.no_password === true
  if (d.username) credForm.username = d.username
}
function enterSmsPhone(d: any) {
  step.value = 'sms_phone'
  hint.value = d.hint || ''
  phoneForm.phone = ''
}
function enterOtp(d: any) {
  step.value = 'otp'
  hint.value = d.hint || ''
  otpForm.code = ''
}
function enterSms(d: any) {
  step.value = 'sms'
  hint.value = d.hint || '请输入短信验证码'
  phoneMasked.value = d.phone_masked || ''
  smsForm.code = ''
  startSmsCountdown()
}
function enterRadius(d: any) {
  step.value = 'radius'
  challengeMsg.value = d.challenge_msg || ''
  credForm.password = ''
}
function enterChangePwd(d: any) {
  step.value = 'change_pwd'
  hint.value = d.hint || ''
  changePwdForm.new_password = ''
  changePwdForm.confirm_password = ''
}
function onDone(d: any) {
  // 跳到服务端完成端点：设置 acSamlv2Token Cookie 供客户端读取
  if (d.complete_url) {
    window.location.href = d.complete_url
    return
  }
  step.value = 'done'
  portalUrl.value = d.portal_url || '/ui/#/portal'
  setTimeout(goPortal, 2000)
}
function goPortal() {
  window.location.href = portalUrl.value
}

function handleStepResponse(d: any) {
  switch (d.status) {
    case 'credentials':
      credForm.password = ''
      enterCredentials(d)
      break
    case 'sms_phone': enterSmsPhone(d); break
    case 'otp': enterOtp(d); break
    case 'sms': enterSms(d); break
    case 'radius': enterRadius(d); break
    case 'change_pwd': enterChangePwd(d); break
    case 'sso': window.location.href = d.redirect_url; break
    case 'done': onDone(d); break
    case 'error': error.value = d.message || '认证失败'; break
    default: error.value = d.message || '未知响应'
  }
}

// 通用提交：loading 互斥 + 失败文案
async function send(name: string, body: any, failMsg: string, onData: (d: any) => void = handleStepResponse) {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    const { data } = await axios.post(apiPath(name), body, opts)
    onData(data)
  } catch {
    error.value = failMsg
  } finally {
    loading.value = false
  }
}

// ---------- 提交 ----------
function submitIdentify() {
  if (!identifyForm.username || loading.value) return
  loading.value = true
  error.value = ''
  axios.post(apiPath('identify'), { username: identifyForm.username }, opts).then(({ data }) => {
    loading.value = false
    if (data.status === 'select_group') setGroups(data.groups || [])
    else error.value = data.message || (data.status === 'error' ? '加载组失败' : '未知响应')
  }).catch(() => {
    // 旧版服务端无 /api/v1/web-auth/identify：回退展示 start 返回的全量组
    loading.value = false
    if (fallbackGroups.value.length) {
      groups.value = fallbackGroups.value
      step.value = 'select_group'
    } else {
      error.value = '请求失败，请重试'
    }
  })
}

function submitGroup() {
  if (!selectedGroup.value) return
  send('groups', { group: selectedGroup.value, username: credForm.username || '' }, '请求失败，请重试')
}

function submitCredentials() {
  if (!noPassword.value && !credForm.password) return
  if (noPassword.value && !credForm.username) return
  send('step', { username: credForm.username || '', password: credForm.password }, '认证失败，请重试')
}

function submitSmsPhone() {
  if (!isPhoneValid.value) return
  send('step', { phone: phoneForm.phone }, '请求失败，请重试')
}

function submitOtp() {
  if (otpForm.code.length < 6) return
  send('step', { otp_code: otpForm.code }, '验证失败，请重试')
}

function submitSmsCode() {
  if (smsForm.code.length < 6) return
  send('step', { sms_code: smsForm.code }, '验证失败，请重试')
}

function submitChangePwd() {
  if (!changePwdForm.new_password || !changePwdForm.confirm_password) return
  send('change_password', {
    new_password: changePwdForm.new_password,
    new_password_confirm: changePwdForm.confirm_password,
  }, '修改失败，请重试')
}

function resendSmsCode() {
  if (smsCountdown.value > 0) return
  send('sms/resend', {}, '重发失败，请重试', d => {
    if (d.status === 'ok') startSmsCountdown()
    else error.value = d.message || '重发失败'
  })
}

function startSmsCountdown() {
  smsCountdown.value = 60
  if (smsTimer) clearInterval(smsTimer)
  smsTimer = setInterval(() => {
    if (--smsCountdown.value <= 0 && smsTimer) {
      clearInterval(smsTimer)
      smsTimer = null
    }
  }, 1000)
}

// SSO 回调后继续（整页跳转方式的兼容入口）
async function continueAfterSSO() {
  loading.value = true
  loadingText.value = '正在恢复认证流程...'
  error.value = ''
  try {
    const { data } = await axios.post(apiPath('continue'), {}, opts)
    loadingText.value = ''
    loading.value = false
    handleStepResponse(data)
  } catch {
    loading.value = false
    loadingText.value = ''
    error.value = '恢复认证失败，请重新发起连接'
  }
}

onMounted(async () => {
  if (await redirectToVpnOriginIfAdmin()) return
  if (!state) {
    error.value = '缺少认证参数，请重新发起连接'
    return
  }
  loadBrand()
  if (route.path === '/web-auth/continue') continueAfterSSO()
  else initAuth()
})
onBeforeUnmount(() => { if (smsTimer) clearInterval(smsTimer) })

// 各步骤标题 / 说明
const title = computed(() => ({
  loading: 'Web 认证',
  identify: '验证身份',
  select_group: '选择用户组',
  sms_phone: '短信验证',
  credentials: '登录',
  otp: hint.value || '请输入 6 位动态验证码',
  sms: hint.value || '请输入短信验证码',
  radius: challengeMsg.value || '请输入二次验证码',
  change_pwd: hint.value || '首次登录，请修改密码',
  done: '认证成功',
}[step.value]))
const desc = computed(() => ({
  loading: '',
  identify: '请输入用户名以加载您可接入的用户组',
  select_group: '请选择您所属的用户组以继续身份认证',
  sms_phone: hint.value || '请输入手机号以接收短信验证码',
  credentials: hint.value || '请输入登录凭据',
  otp: '已启用 OTP 二次认证，请输入手机令牌动态码',
  sms: `验证码已发送至 ${phoneMasked.value}`,
  radius: '已启用 RADIUS 二次认证，请输入动态验证码',
  change_pwd: '新密码规则：至少 8 位且须包含字母和数字',
  done: '即将跳转到用户自助门户',
}[step.value]))
</script>

<template>
  <div class="relative flex min-h-screen flex-col bg-body text-primary">
    <div class="absolute right-16px top-16px">
      <NButton quaternary circle class="text-secondary hover:text-primary!" :title="theme.isDark ? '切换亮色' : '切换暗色'" @click="theme.toggle()">
        <template #icon><NIcon><Sun v-if="theme.isDark" /><Moon v-else /></NIcon></template>
      </NButton>
    </div>

    <div class="flex flex-1 items-center justify-center px-16px py-48px">
      <div class="w-full max-w-400px">
        <!-- 品牌 -->
        <div class="mb-md flex items-center gap-12px">
          <img v-if="logo" :src="logo" class="h-40px w-40px object-contain" alt="logo">
          <div class="min-w-0">
            <div class="truncate text-xl font-bold">{{ brand.title || 'RemLink' }}</div>
            <div class="truncate text-sm text-secondary">{{ brand.desc || '企业级安全远程接入网关认证' }}</div>
          </div>
        </div>

        <div ref="stepEl" class="rounded-12px border border-solid border-[var(--color-border-default)] bg-primary p-28px">
          <!-- 加载中 / 仅错误 -->
          <template v-if="step === 'loading'">
            <div v-if="loadingText" class="flex items-center gap-10px py-md text-md text-secondary">
              <NSpin size="small" />{{ loadingText }}
            </div>
            <h2 v-else-if="!error" class="m-0 text-xl font-semibold">{{ title }}</h2>
          </template>

          <!-- 完成 -->
          <div v-else-if="step === 'done'" class="flex flex-col items-center py-sm text-center">
            <NIcon size="40" class="text-success"><CheckmarkCircle /></NIcon>
            <h2 class="m-0 mt-sm text-xl font-semibold">{{ title }}</h2>
            <p class="m-0 mt-4px text-md text-secondary">{{ desc }}</p>
            <NButton type="primary" size="large" block class="mt-md" @click="goPortal">进入门户</NButton>
          </div>

          <template v-else>
            <h2 class="m-0 text-xl font-semibold">{{ title }}</h2>
            <p v-if="desc" class="mb-0 mt-4px text-md text-secondary">{{ desc }}</p>

            <div class="mt-md">
              <!-- 输入用户名以过滤组 -->
              <template v-if="step === 'identify'">
                <NInput v-model:value="identifyForm.username" size="large" placeholder="用户名" @keydown.enter.prevent="submitIdentify">
                  <template #prefix><NIcon class="text-secondary"><Person /></NIcon></template>
                </NInput>
                <NButton type="primary" size="large" block class="mt-md" :loading="loading" :disabled="!identifyForm.username" @click="submitIdentify">继续</NButton>
              </template>

              <!-- 组选择 -->
              <template v-else-if="step === 'select_group'">
                <GroupPicker v-model="selectedGroup" :groups="groups" />
                <p v-if="!groups.length" class="m-0 text-md text-tertiary">暂无可接入的用户组</p>
                <NButton type="primary" size="large" block class="mt-md" :loading="loading" :disabled="!selectedGroup" @click="submitGroup">下一步</NButton>
              </template>

              <!-- 手机号（SMS 首步） -->
              <template v-else-if="step === 'sms_phone'">
                <NInput
                  v-model:value="phoneForm.phone" size="large" placeholder="手机号" maxlength="11"
                  :input-props="{ inputmode: 'tel' }" @keydown.enter.prevent="submitSmsPhone"
                >
                  <template #prefix><NIcon class="text-secondary"><Phone /></NIcon></template>
                </NInput>
                <NButton type="primary" size="large" block class="mt-md" :loading="loading" :disabled="!isPhoneValid" @click="submitSmsPhone">获取验证码</NButton>
              </template>

              <!-- 用户名密码 -->
              <template v-else-if="step === 'credentials'">
                <div class="grid gap-12px">
                  <NInput
                    v-model:value="credForm.username" size="large" placeholder="用户名" :disabled="!!usernameField"
                    :input-props="{ autocomplete: 'username' }" @keydown.enter.prevent="submitCredentials"
                  >
                    <template #prefix><NIcon class="text-secondary"><Person /></NIcon></template>
                  </NInput>
                  <NInput
                    v-if="!noPassword" v-model:value="credForm.password" size="large" type="password" show-password-on="click"
                    placeholder="密码" :input-props="{ autocomplete: 'current-password' }" @keydown.enter.prevent="submitCredentials"
                  >
                    <template #prefix><NIcon class="text-secondary"><LockClosed /></NIcon></template>
                  </NInput>
                </div>
                <NButton
                  type="primary" size="large" block class="mt-md" :loading="loading"
                  :disabled="noPassword ? !credForm.username : !credForm.password" @click="submitCredentials"
                >
                  继续
                </NButton>
              </template>

              <!-- OTP -->
              <template v-else-if="step === 'otp'">
                <NInput
                  v-model:value="otpForm.code" size="large" placeholder="6 位动态验证码" maxlength="6" class="mono"
                  :input-props="{ inputmode: 'numeric', autocomplete: 'one-time-code' }" @keydown.enter.prevent="submitOtp"
                >
                  <template #prefix><NIcon class="text-secondary"><Key /></NIcon></template>
                </NInput>
                <NButton type="primary" size="large" block class="mt-md" :loading="loading" :disabled="otpForm.code.length < 6" @click="submitOtp">验证</NButton>
              </template>

              <!-- 短信验证码 -->
              <template v-else-if="step === 'sms'">
                <NInput
                  v-model:value="smsForm.code" size="large" placeholder="6 位验证码" maxlength="6" class="mono"
                  :input-props="{ inputmode: 'numeric', autocomplete: 'one-time-code' }" @keydown.enter.prevent="submitSmsCode"
                >
                  <template #prefix><NIcon class="text-secondary"><Key /></NIcon></template>
                </NInput>
                <NButton type="primary" size="large" block class="mt-md" :loading="loading" :disabled="smsForm.code.length < 6" @click="submitSmsCode">验证</NButton>
                <div class="mt-16px text-center">
                  <NButton text :disabled="smsCountdown > 0" class="text-secondary hover:text-primary!" @click="resendSmsCode">
                    {{ smsCountdown > 0 ? `${smsCountdown}s 后重新发送` : '重新发送验证码' }}
                  </NButton>
                </div>
              </template>

              <!-- RADIUS 二次验证 -->
              <template v-else-if="step === 'radius'">
                <NInput
                  v-model:value="credForm.password" size="large" type="password" show-password-on="click" placeholder="二次验证码"
                  @keydown.enter.prevent="submitCredentials"
                >
                  <template #prefix><NIcon class="text-secondary"><LockClosed /></NIcon></template>
                </NInput>
                <NButton type="primary" size="large" block class="mt-md" :loading="loading" :disabled="!credForm.password" @click="submitCredentials">提交</NButton>
              </template>

              <!-- 首次登录改密 -->
              <template v-else-if="step === 'change_pwd'">
                <div class="grid gap-12px">
                  <NInput
                    v-model:value="changePwdForm.new_password" size="large" type="password" show-password-on="click" placeholder="新密码"
                    :input-props="{ autocomplete: 'new-password' }" @keydown.enter.prevent="submitChangePwd"
                  >
                    <template #prefix><NIcon class="text-secondary"><LockClosed /></NIcon></template>
                  </NInput>
                  <NInput
                    v-model:value="changePwdForm.confirm_password" size="large" type="password" show-password-on="click" placeholder="确认新密码"
                    :input-props="{ autocomplete: 'new-password' }" @keydown.enter.prevent="submitChangePwd"
                  >
                    <template #prefix><NIcon class="text-secondary"><LockClosed /></NIcon></template>
                  </NInput>
                </div>
                <NButton
                  type="primary" size="large" block class="mt-md" :loading="loading"
                  :disabled="!changePwdForm.new_password || !changePwdForm.confirm_password" @click="submitChangePwd"
                >
                  修改并继续
                </NButton>
              </template>
            </div>

            <div v-if="loadingText" class="mt-md flex items-center gap-8px text-md text-secondary">
              <NSpin size="small" />{{ loadingText }}
            </div>
          </template>

          <!-- 错误提示 -->
          <div v-if="error" class="flex items-start gap-8px rounded-8px bg-fill px-12px py-10px text-md text-error" :class="step !== 'loading' && 'mt-md'">
            <NIcon class="mt-3px flex-shrink-0"><ErrorCircle /></NIcon>
            <span>{{ error }}</span>
          </div>
        </div>

        <!-- 品牌页脚（管理员配置的 HTML） -->
        <div v-if="brand.footer" class="wa-footer mt-md text-center text-sm text-secondary" v-html="brand.footer" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.wa-footer :deep(a) { color: var(--color-text-info); }
</style>
