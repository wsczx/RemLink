<script setup lang="ts">
// 测试登录（LDAP / RADIUS）：输入一个真实账号验证连通性，结果就地显示
import { ref } from 'vue'
import { NButton, NForm, NFormItem, NInput, type FormInst, type FormRules } from 'naive-ui'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-regular'
import DismissCircle from '~icons/fluent/dismiss-circle-20-regular'
import { post, errMsg } from '@/api/http'
import { formatDate } from '@/utils/format'

const props = defineProps<{ providerId: number; disabledReason?: string }>()
const emit = defineEmits<{ result: [ok: boolean] }>()

const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const form = ref({ name: '', pwd: '' })
const result = ref<{ ok: boolean; msg: string; at: Date } | null>(null)
const rules: FormRules = {
  name: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  pwd: [{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, message: '至少6个字符', trigger: 'blur' }],
}

async function submit() {
  if (props.disabledReason) return
  try { await formRef.value?.validate() } catch { return }
  loading.value = true
  try {
    await post('/api/v1/provider/test_login', { id: props.providerId, name: form.value.name, pwd: form.value.pwd })
    result.value = { ok: true, msg: `账号 ${form.value.name} 登录成功，认证源工作正常`, at: new Date() }
    emit('result', true)
  } catch (e) {
    result.value = { ok: false, msg: errMsg(e), at: new Date() }
    emit('result', false)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <div v-if="disabledReason" class="rounded-8px bg-fill px-12px py-10px text-md text-secondary">{{ disabledReason }}</div>
    <template v-else>
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="top" class="grid grid-cols-1 gap-x-12px sm:grid-cols-2">
        <NFormItem label="测试账号" path="name">
          <NInput v-model:value="form.name" placeholder="目录中的用户名" :input-props="{ autocomplete: 'off' }" @keydown.enter="submit" />
        </NFormItem>
        <NFormItem label="密码" path="pwd">
          <NInput v-model:value="form.pwd" type="password" show-password-on="click" placeholder="该用户的密码" :input-props="{ autocomplete: 'new-password' }" @keydown.enter="submit" />
        </NFormItem>
      </NForm>
      <div class="flex flex-wrap items-center gap-10px">
        <NButton :loading="loading" @click="submit">测试登录</NButton>
        <span class="text-sm text-tertiary">多次失败会触发防爆破锁定</span>
      </div>
    </template>

    <div
      v-if="result" class="mt-12px flex items-start gap-8px rounded-8px px-12px py-10px"
      :class="result.ok ? 'pv-res--ok' : 'pv-res--err'"
    >
      <component :is="result.ok ? CheckmarkCircle : DismissCircle" class="mt-1px flex-shrink-0 text-lg" :class="result.ok ? 'text-success' : 'text-error'" />
      <div class="min-w-0 flex-1">
        <div class="text-md" :class="result.ok ? 'text-success' : 'text-error'">{{ result.ok ? '测试通过' : '测试失败' }}</div>
        <div class="break-all text-sm text-secondary">{{ result.msg }}</div>
      </div>
      <span class="flex-shrink-0 text-sm text-tertiary">{{ formatDate(result.at, 'HH:mm:ss') }}</span>
    </div>
  </div>
</template>

<style scoped>
.pv-res--ok { background: color-mix(in srgb, var(--color-text-success) 10%, transparent); }
.pv-res--err { background: color-mix(in srgb, var(--color-text-error) 10%, transparent); }
</style>
