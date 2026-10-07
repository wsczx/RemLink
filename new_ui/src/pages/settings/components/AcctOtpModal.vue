<script setup lang="ts">
// 管理员 OTP：启用（生成密钥 → 扫码 → 输入验证码确认）/ 查看（重新验证密码 + 验证码后展示密钥）
import { ref, watch } from 'vue'
import { NButton, NInput, NModal } from 'naive-ui'
import Copy from '~icons/fluent/copy-20-regular'
import { errMsg, post } from '@/api/http'
import { ui } from '@/utils/ui'

const props = defineProps<{ mode: 'enable' | 'view' }>()
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ enabled: [] }>()

const qr = ref('')
const secret = ref('')
const code = ref('')
const busy = ref(false)
// 查看模式：先验证
const verified = ref(false)
const password = ref('')
const viewCode = ref('')

function reset() {
  qr.value = ''
  secret.value = ''
  code.value = ''
  verified.value = false
  password.value = ''
  viewCode.value = ''
}

watch(show, async v => {
  if (!v) return reset()
  reset()
  if (props.mode !== 'enable') return
  try {
    const d = await post('/api/v1/set/otp_generate')
    qr.value = d.qr_base64
    secret.value = d.secret
  } catch (e) {
    ui.message.error(errMsg(e, '生成密钥失败'))
    show.value = false
  }
})

async function confirmBind() {
  if (code.value.length < 6) return ui.message.warning('请输入 6 位验证码')
  busy.value = true
  try {
    await post('/api/v1/set/otp_confirm', { otp_code: code.value })
    ui.message.success('两步验证已启用')
    emit('enabled')
    show.value = false
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    busy.value = false
  }
}

async function verifyView() {
  if (!password.value) return ui.message.warning('请输入管理员密码')
  if (viewCode.value.length < 6) return ui.message.warning('请输入 6 位验证码')
  busy.value = true
  try {
    const d = await post('/api/v1/set/otp_qr', { password: password.value, otp_code: viewCode.value })
    qr.value = d.qr_base64
    secret.value = d.secret
    verified.value = true
  } catch (e) {
    ui.message.error(errMsg(e, '验证失败'))
  } finally {
    busy.value = false
  }
}

async function copySecret() {
  if (!secret.value) return
  try {
    await navigator.clipboard.writeText(secret.value)
  } catch {
    // 非安全上下文（HTTP）下 clipboard 不可用，回退到 execCommand
    const ta = document.createElement('textarea')
    ta.value = secret.value
    ta.style.cssText = 'position:fixed;opacity:0'
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
  }
  ui.message.success('密钥已复制到剪贴板')
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="mode === 'enable' ? '启用两步验证' : '两步验证密钥'" class="w-440px! max-w-[calc(100vw-32px)]" :bordered="false">
    <!-- 查看前验证 -->
    <div v-if="mode === 'view' && !verified" class="flex flex-col gap-sm">
      <p class="m-0 text-md text-secondary">查看密钥需要重新验证管理员密码和当前动态验证码。</p>
      <NInput v-model:value="password" type="password" show-password-on="click" placeholder="管理员密码" />
      <NInput v-model:value="viewCode" :maxlength="6" placeholder="当前 6 位验证码" class="rl-mono" @keyup.enter="verifyView" />
      <NButton type="primary" block :loading="busy" @click="verifyView">验证并查看</NButton>
    </div>

    <div v-else class="flex flex-col gap-16px">
      <p class="m-0 text-md text-secondary">
        <template v-if="mode === 'enable'">使用 Google Authenticator、Microsoft Authenticator 等应用扫描二维码，然后输入生成的 6 位验证码完成绑定。</template>
        <template v-else>使用以下密钥或二维码将两步验证添加到认证器应用。</template>
      </p>
      <div class="flex justify-center">
        <div class="h-200px w-200px flex items-center justify-center rounded-10px bg-primary p-8px b-1px b-solid b-[var(--color-border-default)]">
          <img v-if="qr" :src="'data:image/png;base64,' + qr" alt="OTP 二维码" class="h-full w-full">
          <span v-else class="text-sm text-tertiary">生成中…</span>
        </div>
      </div>
      <div>
        <div class="mb-6px text-sm text-secondary">密钥（手动输入）</div>
        <div class="flex items-center gap-xs rounded-8px bg-fill px-12px py-8px">
          <code class="rl-mono min-w-0 flex-1 select-all break-all">{{ secret || '—' }}</code>
          <button class="rl-icon-btn flex-shrink-0" title="复制" @click="copySecret"><Copy /></button>
        </div>
      </div>
      <div v-if="mode === 'enable'" class="flex gap-sm">
        <NInput v-model:value="code" :maxlength="6" placeholder="6 位验证码" class="rl-mono flex-1" @keyup.enter="confirmBind" />
        <NButton type="primary" :loading="busy" @click="confirmBind">确认绑定</NButton>
      </div>
    </div>

    <template v-if="mode === 'view' && verified" #footer>
      <div class="flex justify-end"><NButton @click="show = false">关闭</NButton></div>
    </template>
  </NModal>
</template>
