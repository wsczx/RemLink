<script setup lang="ts">
// 通用输入弹窗：证书下载密码 / 最大设备数
import { ref, watch } from 'vue'
import { NButton, NInput, NInputNumber, NModal } from 'naive-ui'

const props = withDefaults(defineProps<{
  title: string
  label: string
  kind?: 'password' | 'number'
  placeholder?: string
  initial?: number
  max?: number
  confirmText?: string
  loading?: boolean
}>(), { kind: 'password', confirmText: '确定' })
const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ confirm: [value: string | number] }>()

const text = ref('')
const num = ref<number | null>(1)
watch(show, v => {
  if (!v) return
  text.value = ''
  num.value = props.initial ?? 1
})

function ok() {
  if (props.kind === 'number') {
    if (!num.value || num.value < 1) return
    emit('confirm', num.value)
  } else emit('confirm', text.value)
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="title" class="w-400px! max-w-[calc(100vw-32px)]" :bordered="false">
    <div class="mb-6px text-sm text-secondary">{{ label }}</div>
    <NInputNumber v-if="kind === 'number'" v-model:value="num" :min="1" :max="max" class="w-160px" @keyup.enter="ok" />
    <NInput v-else v-model:value="text" type="password" show-password-on="click" :placeholder="placeholder" :input-props="{ autocomplete: 'new-password' }" @keyup.enter="ok" />
    <template #footer>
      <div class="flex justify-end gap-sm">
        <NButton @click="show = false">取消</NButton>
        <NButton type="primary" :loading="loading" :disabled="kind === 'number' && (!num || num < 1)" @click="ok">{{ confirmText }}</NButton>
      </div>
    </template>
  </NModal>
</template>
