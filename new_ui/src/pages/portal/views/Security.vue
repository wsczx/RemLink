<script setup lang="ts">
// 账号安全：修改密码 + 两步验证（TOTP 查看 / 重新绑定）
import { computed, nextTick, reactive, ref } from 'vue'
import { NAlert, NButton, NForm, NFormItem, NInput, NModal } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import { ui } from '@/utils/ui'
import PwdStrength from '../PwdStrength.vue'
import { usePortal } from '../store'
import { copyText, portalApi, validatePassword } from '../shared'

const p = usePortal()
const { user, cardVisible } = p

// ---------- 修改密码 ----------
const form = reactive({ old_password: '', new_password: '', confirm_password: '' })
const pwdLoading = ref(false)

async function submitPassword() {
  if (!form.old_password || !form.new_password) return ui.message.warning('请输入新旧密码')
  const err = validatePassword(form.new_password)
  if (err) return ui.message.warning(err)
  if (form.new_password !== form.confirm_password) return ui.message.warning('两次输入的新密码不一致')
  pwdLoading.value = true
  try {
    const r = await portalApi('post', '/api/v1/portal/change_password', { old_password: form.old_password, new_password: form.new_password })
    if (r.code !== 0) return ui.message.error(r.msg)
    ui.message.success('密码修改成功，下次登录请使用新密码')
    Object.assign(form, { old_password: '', new_password: '', confirm_password: '' })
  } catch {
    ui.message.error('请求失败，请稍后重试')
  } finally {
    pwdLoading.value = false
  }
}

// ---------- 两步验证 ----------
const otpVisible = ref(false)
const otpStep = ref<'password' | 'qr'>('password')
const otpPassword = ref('')
const secret = ref('')
const qr = ref('')
const otpLoading = ref(false)
const otpPwdInput = ref<InstanceType<typeof NInput>>()
const otpTitle = computed(() => (otpStep.value === 'password' ? '重新绑定身份验证器' : '身份验证器密钥'))

async function showQr() {
  try {
    const r = await portalApi('get', '/api/v1/portal/otp/status')
    if (r.code !== 0) return ui.message.error(r.msg)
    secret.value = r.data?.secret || ''
    qr.value = r.data?.qr_base64 || ''
    otpStep.value = 'qr'
    otpVisible.value = true
  } catch {
    ui.message.error('请求失败')
  }
}

function openRebind() {
  otpStep.value = 'password'
  otpPassword.value = ''
  secret.value = ''
  qr.value = ''
  otpVisible.value = true
  nextTick(() => otpPwdInput.value?.focus())
}

async function submitRebind() {
  if (!otpPassword.value) return ui.message.warning('请输入密码')
  otpLoading.value = true
  try {
    const r = await portalApi('post', '/api/v1/portal/otp/regenerate', { password: otpPassword.value })
    if (r.code !== 0) return ui.message.error(r.msg)
    secret.value = r.data?.secret || ''
    qr.value = r.data?.qr_base64 || ''
    otpStep.value = 'qr'
    otpPassword.value = ''
    user.value.otp_enabled = true
    ui.message.success('请立即扫码绑定两步验证')
  } catch {
    ui.message.error('请求失败')
  } finally {
    otpLoading.value = false
  }
}

function onOtpClosed() {
  otpStep.value = 'password'
  otpPassword.value = ''
  secret.value = ''
  qr.value = ''
}
</script>

<template>
  <div>
    <PageHeader title="账号安全" description="管理登录密码与两步验证，保护你的账号不被他人使用" />

    <div class="grid gap-16px">
      <!-- 两步验证：状态优先 -->
      <RlPanel v-if="cardVisible('otp')" title="两步验证" subtitle="登录时除密码外，还需输入身份验证器 App 中的 6 位动态码">
        <template #extra>
          <StatusDot :tone="user.otp_enabled ? 'success' : 'muted'">{{ user.otp_enabled ? '已启用' : '未启用' }}</StatusDot>
        </template>
        <p v-if="!user.otp_enabled" class="m-0 text-md text-secondary">当前账号未启用两步验证。如需开启，请联系管理员在后台为你启用 OTP。</p>
        <div v-else class="flex flex-wrap items-center gap-10px">
          <NButton @click="showQr">查看密钥 / 二维码</NButton>
          <NButton type="warning" secondary @click="openRebind">重新绑定</NButton>
          <span class="text-sm text-tertiary">更换手机或丢失验证器时，可重新绑定</span>
        </div>
      </RlPanel>

      <!-- 修改密码 -->
      <RlPanel v-if="cardVisible('password')" title="登录密码">
        <div v-if="user.can_change_password" class="max-w-420px">
          <NForm label-placement="top" :show-feedback="false" class="grid gap-16px" @submit.prevent>
            <NFormItem label="当前密码">
              <NInput v-model:value="form.old_password" type="password" show-password-on="click" placeholder="输入当前密码" :input-props="{ autocomplete: 'current-password' }" />
            </NFormItem>
            <NFormItem label="新密码">
              <NInput v-model:value="form.new_password" type="password" show-password-on="click" placeholder="至少 8 位，含字母和数字" :input-props="{ autocomplete: 'new-password' }" />
            </NFormItem>
            <NFormItem label="确认新密码">
              <NInput v-model:value="form.confirm_password" type="password" show-password-on="click" placeholder="再次输入新密码" :input-props="{ autocomplete: 'new-password' }" @keydown.enter.prevent="submitPassword" />
            </NFormItem>
            <PwdStrength :password="form.new_password" />
          </NForm>
          <NButton type="primary" class="mt-20px" :loading="pwdLoading" @click="submitPassword">修改密码</NButton>
        </div>
        <p v-else class="m-0 text-md text-secondary">
          你的账号由外部身份源（{{ p.typeLabel.value }}）认证，请到对应平台修改密码。
        </p>
      </RlPanel>
    </div>

    <NModal
      v-model:show="otpVisible" preset="card" :title="otpTitle" :style="{ width: 'min(420px, calc(100vw - 32px))' }"
      :segmented="{ footer: true }" @after-leave="onOtpClosed"
    >
      <template v-if="otpStep === 'password'">
        <NAlert type="warning" :show-icon="true" class="mb-16px">重新绑定后旧密钥立即失效，所有已绑定的验证器都需要重新扫码</NAlert>
        <p class="m-0 mb-8px text-md text-secondary">请输入当前登录密码确认操作</p>
        <NInput ref="otpPwdInput" v-model:value="otpPassword" type="password" show-password-on="click" placeholder="当前密码" @keydown.enter.prevent="submitRebind" />
      </template>
      <template v-else>
        <div v-if="qr" class="flex justify-center">
          <img :src="'data:image/png;base64,' + qr" alt="OTP QR" class="h-200px w-200px rounded-8px bg-white p-8px">
        </div>
        <div v-if="secret" class="mt-16px flex items-center gap-8px rounded-8px bg-fill px-12px py-8px">
          <span class="text-sm text-secondary">密钥</span>
          <code class="rl-mono min-w-0 flex-1 break-all">{{ secret }}</code>
          <NButton text type="primary" size="small" @click="copyText(secret)">复制</NButton>
        </div>
        <p class="m-0 mt-12px text-center text-sm text-secondary">使用 Google Authenticator 或其他身份验证器 App 扫描二维码</p>
      </template>
      <template #footer>
        <div class="flex justify-end gap-10px">
          <NButton @click="otpVisible = false">{{ otpStep === 'qr' ? '完成' : '取消' }}</NButton>
          <NButton v-if="otpStep === 'password'" type="primary" :loading="otpLoading" @click="submitRebind">确认</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
