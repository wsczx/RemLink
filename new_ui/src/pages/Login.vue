<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NForm, NFormItem, NIcon, NInput, NModal, type FormInst, type FormRules } from 'naive-ui'
import Person from '~icons/fluent/person-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import Moon from '~icons/fluent/weather-moon-20-regular'
import Sun from '~icons/fluent/weather-sunny-20-regular'
import ArrowLeft from '~icons/fluent/arrow-left-20-regular'
import KeyReset from '~icons/fluent/key-reset-20-regular'
import { http, post, postForm, errMsg } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { applyBrandToDocument, inlineBrand, normalizeImageSrc, type Brand } from '@/utils/brand'
import { ui } from '@/utils/ui'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const theme = useThemeStore()

const brand = ref<Partial<Brand>>({ title: inlineBrand().title })
// 仅显示管理员自定义 logo
const logo = computed(() => normalizeImageSrc(brand.value.logo))

const title = computed(() => brand.value.title || 'RemLink')

const formRef = ref<FormInst>()
const form = reactive({ admin_user: '', admin_pass: '' })
const rules: FormRules = {
  admin_user: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { max: 50, message: '长度小于 50 个字符', trigger: 'blur' },
  ],
  admin_pass: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '长度大于 6 个字符', trigger: 'blur' },
  ],
}
const loading = ref(false)
// 临时密码提示：必须先去修改
const tempPwd = ref(false)
function goChangePassword() {
  tempPwd.value = false
  router.push('/admin/settings/account')
}

const otpStep = ref(false)
const otpToken = ref('')
const otpCode = ref('')
const otpLoading = ref(false)
const otpInput = ref<InstanceType<typeof NInput>>()

onMounted(() => {
  http.get('/api/v1/portal/login-config').then(r => {
    if (r.data?.data) {
      brand.value = r.data.data
      applyBrandToDocument(brand.value)
    }
  }).catch(() => {})
})

async function onSuccess(d: any) {
  auth.setUser(d.admin_user)
  // 确认 JWT cookie 已生效，避免进入后台后首个请求 401 又被踢回
  if (!(await auth.check())) {
    ui.message.error('登录验证失败，请重试')
    return
  }
  ui.message.success('登录成功')
  if (d.admin_temp) {
    tempPwd.value = true
    return
  }
  const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/admin') ? route.query.redirect : '/admin/home'
  router.push(redirect)
}

async function submit() {
  if (loading.value) return
  try { await formRef.value?.validate() } catch { return }
  loading.value = true
  try {
    const d = await postForm('/api/v1/base/login', form)
    if (d.otp_required) {
      otpToken.value = d.otp_token
      otpCode.value = ''
      otpStep.value = true
      nextTick(() => otpInput.value?.focus())
      return
    }
    await onSuccess(d)
  } catch (e) {
    ui.message.error(errMsg(e, '请求出错，请检查网络连接'))
  } finally {
    loading.value = false
  }
}

async function submitOtp() {
  if (otpLoading.value) return
  if (!/^\d{6}$/.test(otpCode.value)) {
    ui.message.warning('请输入 6 位动态验证码')
    return
  }
  otpLoading.value = true
  try {
    const d = await post('/api/v1/base/login/otp', { otp_token: otpToken.value, otp_code: otpCode.value })
    await onSuccess(d)
  } catch (e) {
    ui.message.error(errMsg(e, '请求出错，请检查网络连接'))
  } finally {
    otpLoading.value = false
  }
}
</script>

<template>
  <div class="relative flex min-h-screen flex-col bg-body">
    <div class="flex h-64px items-center justify-between px-24px">
      <div class="flex items-center gap-8px">
        <img v-if="logo" :src="logo" class="h-24px w-24px object-contain" alt="">
        <span class="text-lg font-bold tracking-tight">{{ title }}</span>
      </div>
      <button class="rl-icon-btn" title="切换主题" @click="theme.toggle()">
        <Sun v-if="theme.isDark" /><Moon v-else />
      </button>
    </div>

      <div class="flex flex-1 items-center justify-center px-16px pb-64px">
        <div class="w-full max-w-360px">
          <template v-if="!otpStep">
            <h1 class="m-0 text-4xl font-semibold leading-32px">登录控制台</h1>
            <p class="mb-24px mt-6px text-md text-secondary">使用管理员账号登录 {{ title }}</p>
            <NForm ref="formRef" :model="form" :rules="rules" label-placement="top" @keyup.enter="submit">
              <NFormItem path="admin_user" label="用户名">
                <NInput v-model:value="form.admin_user" placeholder="请输入用户名" :input-props="{ autocomplete: 'username' }">
                  <template #prefix><NIcon class="text-secondary"><Person /></NIcon></template>
                </NInput>
              </NFormItem>
              <NFormItem path="admin_pass" label="密码">
                <NInput v-model:value="form.admin_pass" type="password" show-password-on="click" placeholder="请输入密码" :input-props="{ autocomplete: 'current-password' }">
                  <template #prefix><NIcon class="text-secondary"><LockClosed /></NIcon></template>
                </NInput>
              </NFormItem>
            </NForm>
            <NButton type="primary" block class="mt-4px" :loading="loading" @click="submit">登录</NButton>
          </template>

          <template v-else>
            <NButton text class="mb-16px text-secondary hover:text-primary!" @click="otpStep = false">
              <template #icon><NIcon><ArrowLeft /></NIcon></template>
              返回重新登录
            </NButton>
            <h1 class="m-0 text-4xl font-semibold leading-32px">两步验证</h1>
            <p class="mb-24px mt-6px text-md text-secondary">请输入身份验证器 App 中的 6 位动态验证码</p>
            <NInput
              ref="otpInput" v-model:value="otpCode" maxlength="6" placeholder="000000"
              class="rl-mono text-center tracking-[.5em]" :input-props="{ inputmode: 'numeric', autocomplete: 'one-time-code' }"
              @keyup.enter="submitOtp"
            />
            <NButton type="primary" block class="mt-16px" :loading="otpLoading" @click="submitOtp">验证</NButton>
          </template>
        </div>
      </div>
    <div class="pb-24px text-center text-sm text-tertiary">© {{ title }}</div>

    <!-- 临时密码提示 -->
    <NModal v-model:show="tempPwd" :mask-closable="false" :close-on-esc="false" transform-origin="center">
      <div class="rl-panel w-[calc(100vw-32px)] max-w-420px p-24px">
        <div class="mb-16px h-40px w-40px flex items-center justify-center rounded-10px bg-fill text-xl text-warning">
          <KeyReset />
        </div>
        <div class="text-xl font-semibold text-primary">请先修改管理员密码</div>
        <p class="m-0 mt-6px text-md text-secondary">
          你正在使用首次生成或命令重置的临时密码。为保证安全，请立即设置新密码；退出后台后将无法再次查看该临时密码。
        </p>
        <NButton type="primary" block class="mt-20px" @click="goChangePassword">去修改密码</NButton>
      </div>
    </NModal>
  </div>
</template>
