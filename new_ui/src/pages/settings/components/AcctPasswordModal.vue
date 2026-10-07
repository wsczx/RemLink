<script setup lang="ts">
// 修改管理员密码：当前密码 + 新密码（实时规则提示）+ 确认
import { computed, ref, watch } from 'vue'
import { NButton, NInput, NModal } from 'naive-ui'
import Checkmark from '~icons/fluent/checkmark-circle-16-filled'
import Circle from '~icons/fluent/circle-16-regular'
import { errMsg, post } from '@/api/http'
import { ui } from '@/utils/ui'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ changed: [] }>()

const form = ref({ old: '', next: '', confirm: '' })
const saving = ref(false)
watch(show, v => { if (v) form.value = { old: '', next: '', confirm: '' } })

const rules = computed(() => {
  const p = form.value.next
  return [
    { ok: p.length >= 8, text: '至少 8 位（必须）' },
    { ok: /[a-z]/.test(p) && /[A-Z]/.test(p), text: '同时包含大小写字母（建议）' },
    { ok: /\d/.test(p) && /[^A-Za-z0-9]/.test(p), text: '包含数字和符号（建议）' },
  ]
})
const mismatch = computed(() => !!form.value.confirm && form.value.confirm !== form.value.next)
const canSubmit = computed(() => !!form.value.old && form.value.next.length >= 8 && form.value.confirm === form.value.next)

async function submit() {
  const p = form.value
  if (!p.old) return ui.message.warning('请输入当前密码')
  if (p.next.length < 8) return ui.message.warning('新密码长度不能少于 8 位')
  if (p.confirm !== p.next) return ui.message.warning('两次输入的新密码不一致')
  saving.value = true
  try {
    await post('/api/v1/set/change_password', { old_password: p.old, new_password: p.next })
    ui.message.success('密码修改成功')
    show.value = false
    emit('changed')
  } catch (e) {
    ui.message.error(errMsg(e, '请求出错，请检查网络'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NModal v-model:show="show" preset="card" title="修改管理员密码" class="w-460px! max-w-[calc(100vw-32px)]" :bordered="false" :mask-closable="!saving">
    <form class="grid gap-14px" @submit.prevent="submit">
      <label class="block">
        <span class="mb-6px block text-sm font-semibold">当前密码</span>
        <NInput v-model:value="form.old" type="password" show-password-on="click" placeholder="输入当前使用的密码" :input-props="{ autocomplete: 'current-password' }" />
      </label>
      <label class="block">
        <span class="mb-6px block text-sm font-semibold">新密码</span>
        <NInput v-model:value="form.next" type="password" show-password-on="click" placeholder="设置新密码" :input-props="{ autocomplete: 'new-password' }" />
        <ul class="m-0 mt-8px grid list-none gap-4px p-0 text-sm">
          <li v-for="r in rules" :key="r.text" class="flex items-center gap-6px" :class="r.ok ? 'text-success' : 'text-tertiary'">
            <component :is="r.ok ? Checkmark : Circle" />{{ r.text }}
          </li>
        </ul>
      </label>
      <label class="block">
        <span class="mb-6px block text-sm font-semibold">确认新密码</span>
        <NInput
          v-model:value="form.confirm" type="password" show-password-on="click" placeholder="再输入一次新密码"
          :status="mismatch ? 'error' : undefined" :input-props="{ autocomplete: 'new-password' }" @keyup.enter="submit"
        />
        <span v-if="mismatch" class="mt-4px block text-sm text-error">两次输入的新密码不一致</span>
      </label>
    </form>
    <template #footer>
      <div class="flex justify-end gap-sm">
        <NButton :disabled="saving" @click="show = false">取消</NButton>
        <NButton type="primary" :loading="saving" :disabled="!canSubmit" @click="submit">更新密码</NButton>
      </div>
    </template>
  </NModal>
</template>
