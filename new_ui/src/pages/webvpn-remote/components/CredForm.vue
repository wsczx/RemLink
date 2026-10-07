<script setup lang="ts">
// 凭据步骤：按 app-info 的 need_username / need_password 显示；ssh/sftp 支持私钥（文件 + 口令）；VNC 账号与密码可留空
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { NButton, NForm, NFormItem, NIcon, NInput } from 'naive-ui'
import Person from '~icons/fluent/person-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import Key from '~icons/fluent/key-20-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import ShieldError from '~icons/fluent/shield-error-20-regular'
import Record from '~icons/fluent/record-20-regular'
import type { AppInfo, CredInput } from '../api'

const props = defineProps<{ info: AppInfo; busy: boolean; error: { code: string; msg: string; fp?: string } | null }>()
const emit = defineEmits<{ submit: [c: CredInput] }>()

const isPrompt = computed(() => props.info.cred_mode === 'prompt')
// 私钥仅 ssh/sftp 可用（旧版 app-info 无该字段时按协议判断）
const allowKey = computed(() => isPrompt.value && (props.info.private_key ?? ['ssh', 'sftp'].includes(props.info.protocol)))
// VNC：prompt 模式且未固定账号时可选填账号
const optUser = computed(() => !props.info.need_username && !!props.info.username_optional && isPrompt.value && !props.info.fixed_username)
const pwOptional = computed(() => !!props.info.password_optional)
const recordText = computed(() => (props.info.desktop || props.info.record_format === 'guac' ? '本会话将被录像，远程桌面画面会被记录用于审计' : '本会话将被录像，终端输出会被记录用于审计'))
const method = ref<'password' | 'key'>('password')
const form = reactive({ username: '', password: '', private_key: '', passphrase: '' })
const keyName = ref('')
const keyPaste = ref(false)
const fileEl = ref<HTMLInputElement>()
const firstInput = ref<InstanceType<typeof NInput>>()
const pwInput = ref<InstanceType<typeof NInput>>()

const proto = computed(() => (props.info.protocol || '').toUpperCase())
const who = computed(() => props.info.fixed_username || (form.username || (props.info.username_optional ? '（无）' : '…')))

onMounted(() => nextTick(() => (firstInput.value || pwInput.value)?.focus()))

async function onKeyFile(ev: Event) {
  const input = ev.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) return
  if (f.size > 32 << 10) { form.private_key = ''; keyName.value = '文件过大，不像私钥'; return }
  form.private_key = await f.text()
  keyName.value = f.name
}
function clearKey() { form.private_key = ''; keyName.value = ''; form.passphrase = '' }

const canSubmit = computed(() => {
  if (props.info.need_username && !form.username.trim()) return false
  if (method.value === 'key' && allowKey.value) return !!form.private_key.trim()
  return pwOptional.value || !!form.password
})

function submit() {
  if (!canSubmit.value || props.busy) return
  const c: CredInput = { app: props.info.name }
  if (props.info.need_username || (optUser.value && form.username.trim())) c.username = form.username.trim()
  if (method.value === 'key' && allowKey.value) {
    c.private_key = form.private_key
    if (form.passphrase) c.passphrase = form.passphrase
  } else {
    c.password = form.password
  }
  emit('submit', c)
}
</script>

<template>
  <div class="w-full max-w-380px">
    <h1 class="m-0 text-4xl font-semibold leading-32px">连接到 {{ info.note || info.name }}</h1>
    <p class="mb-20px mt-6px text-md text-secondary">
      <template v-if="info.cred_mode === 'passthrough'">将使用你的门户账号 <b class="text-primary">{{ info.fixed_username }}</b> 登录目标主机，请输入门户登录密码</template>
      <template v-else>通过浏览器安全访问 {{ proto }}，凭据仅用于本次连接，不会被保存</template>
    </p>

    <div v-if="info.record" class="notice notice--warn mb-16px">
      <Record class="flex-shrink-0" />
      <span>{{ recordText }}</span>
    </div>

    <div v-if="error" class="notice notice--err mb-16px">
      <ShieldError class="mt-1px flex-shrink-0" />
      <div class="min-w-0">
        <div>{{ error.msg }}</div>
        <div v-if="error.fp" class="mt-4px text-sm">
          目标主机当前指纹：<span class="rl-mono break-all text-sm!">{{ error.fp }}</span>
        </div>
      </div>
    </div>

    <NForm label-placement="top" :show-require-mark="false" @keyup.enter="submit" @submit.prevent>
      <NFormItem v-if="info.need_username" label="目标主机账号">
        <NInput ref="firstInput" v-model:value="form.username" placeholder="如 root" :input-props="{ autocomplete: 'username', spellcheck: false }">
          <template #prefix><NIcon class="text-secondary"><Person /></NIcon></template>
        </NInput>
      </NFormItem>
      <NFormItem v-else-if="optUser" label="目标主机账号（可选）">
        <NInput ref="firstInput" v-model:value="form.username" placeholder="仅密码认证时留空" :input-props="{ autocomplete: 'username', spellcheck: false }">
          <template #prefix><NIcon class="text-secondary"><Person /></NIcon></template>
        </NInput>
      </NFormItem>
      <div v-else class="mb-16px flex items-center gap-8px text-md">
        <span class="text-secondary">登录账号</span><span class="rl-kbd text-primary!">{{ who }}</span>
      </div>

      <div v-if="allowKey" class="seg mb-16px">
        <button :class="method === 'password' && 'seg--on'" type="button" @click="method = 'password'">密码</button>
        <button :class="method === 'key' && 'seg--on'" type="button" @click="method = 'key'">私钥</button>
      </div>

      <NFormItem v-if="method === 'password' || !allowKey" :label="info.cred_mode === 'passthrough' ? '门户登录密码' : pwOptional ? '密码（可选）' : '密码'">
        <NInput
          ref="pwInput" v-model:value="form.password" type="password" show-password-on="click"
          :placeholder="pwOptional ? '未设置密码时留空' : '请输入密码'" :input-props="{ autocomplete: 'current-password' }"
        >
          <template #prefix><NIcon class="text-secondary"><LockClosed /></NIcon></template>
        </NInput>
      </NFormItem>

      <template v-else>
        <NFormItem label="私钥">
          <div class="w-full">
            <div v-if="keyName && !keyPaste" class="keyfile">
              <Key class="flex-shrink-0 text-secondary" />
              <span class="min-w-0 flex-1 truncate">{{ keyName }}</span>
              <button class="rl-icon-btn h-24px! w-24px!" title="移除" type="button" @click="clearKey"><Dismiss /></button>
            </div>
            <NInput
              v-else-if="keyPaste" v-model:value="form.private_key" type="textarea" :rows="5" class="rl-mono"
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" :input-props="{ spellcheck: false }"
            />
            <NButton v-else block dashed @click="fileEl?.click()">
              <template #icon><Key /></template>选择私钥文件
            </NButton>
            <div class="mt-6px text-sm text-secondary">
              支持 OpenSSH / PEM 格式。
              <a class="cursor-pointer text-info" @click="keyPaste = !keyPaste; keyName = ''">{{ keyPaste ? '改为选择文件' : '改为粘贴文本' }}</a>
            </div>
            <input ref="fileEl" type="file" hidden @change="onKeyFile">
          </div>
        </NFormItem>
        <NFormItem label="私钥口令（可选）">
          <NInput v-model:value="form.passphrase" type="password" show-password-on="click" placeholder="私钥未加密可留空" :input-props="{ autocomplete: 'off' }" />
        </NFormItem>
      </template>
    </NForm>

    <NButton type="primary" block class="mt-4px" :loading="busy" :disabled="!canSubmit" @click="submit">连接</NButton>
  </div>
</template>

<style scoped>
.notice { display: flex; gap: 8px; align-items: flex-start; padding: 10px 12px; border-radius: 10px; font-size: 13px; line-height: 1.5; position: relative; isolation: isolate; }
.notice::before { content: ''; position: absolute; inset: 0; border-radius: inherit; background: currentColor; opacity: .1; z-index: -1; }
.notice--warn { color: var(--color-text-warning); }
.notice--err { color: var(--color-text-error); }
.seg { display: inline-flex; padding: 2px; border-radius: 8px; background: var(--color-fill); }
.seg button { height: 28px; padding: 0 14px; border: 0; border-radius: 6px; background: transparent; color: var(--color-text-secondary); font: inherit; font-size: 13px; cursor: pointer; }
.seg button.seg--on { background: var(--color-primary); color: var(--color-text-primary); font-weight: 600; box-shadow: 0 1px 2px var(--color-overlay); }
.keyfile { display: flex; align-items: center; gap: 8px; height: 36px; padding: 0 6px 0 12px; border: 1px solid var(--color-border-default); border-radius: 6px; }
</style>
