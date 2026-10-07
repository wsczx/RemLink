<script setup lang="ts">
// 软件配置单项控件：按 type / options / multiline / sensitive 选择输入形态（直接编辑 item.edit_data）
import { computed } from 'vue'
import { NInput, NInputNumber, NRadioButton, NRadioGroup, NSelect, NSwitch } from 'naive-ui'
import LockClosed from '~icons/fluent/lock-closed-20-regular'

const props = defineProps<{ item: any; disabled?: boolean }>()

const opts = computed(() => Object.entries(props.item.options || {}).map(([label, value]) => ({ label, value: value as any })))
const mode = computed(() => {
  const it = props.item
  if (it.type === 'bool') return 'bool'
  if (it.type === 'int') return 'int'
  if (it.multiline) return 'multiline'
  if (opts.value.length > 2) return 'select'
  if (opts.value.length) return 'radio'
  return 'text'
})
// 下拉选项（主网卡）当前值不在可选列表中时提示
const invalidOption = computed(() => mode.value === 'select' && props.item.edit_data !== '' && props.item.edit_data != null
  && !opts.value.some(o => o.value === props.item.edit_data))
</script>

<template>
  <NSwitch v-if="mode === 'bool'" v-model:value="item.edit_data" :disabled="disabled" />
  <NInputNumber v-else-if="mode === 'int'" v-model:value="item.edit_data" :disabled="disabled" class="w-200px!" />
  <NInput
    v-else-if="mode === 'multiline'" v-model:value="item.edit_data" type="textarea" :rows="5" :disabled="disabled"
    placeholder="多个用逗号隔开或者每行一个，支持单 IP 和 CIDR 范围" class="rl-mono"
  />
  <div v-else-if="mode === 'select'">
    <NSelect v-model:value="item.edit_data" :options="opts" :disabled="disabled" />
    <div v-if="invalidOption" class="mt-4px text-sm text-error">主网卡设置错误，请选择正确的物理网卡</div>
  </div>
  <NRadioGroup v-else-if="mode === 'radio'" v-model:value="item.edit_data" :disabled="disabled">
    <NRadioButton v-for="o in opts" :key="String(o.value)" :value="o.value" :label="o.label" />
  </NRadioGroup>
  <NInput
    v-else v-model:value="item.edit_data" :type="item.sensitive ? 'password' : 'text'"
    :show-password-on="item.sensitive ? 'click' : undefined"
    :placeholder="item.sensitive ? '留空表示不修改' : ''" :disabled="disabled"
  >
    <template v-if="item.readonly" #prefix><LockClosed class="text-tertiary" /></template>
  </NInput>
</template>
