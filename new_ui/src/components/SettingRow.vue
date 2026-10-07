<script setup lang="ts">
// 设置行：左侧标题 + 说明（固定宽），右侧控件；行间用细线分隔（放在 RlPanel padded=false 内）
defineProps<{ label: string; description?: string; required?: boolean }>()
</script>

<template>
  <div class="rl-setting-row">
    <div class="rl-setting-row__label">
      <div class="text-md font-semibold">{{ label }}<span v-if="required" class="ml-2px text-error">*</span></div>
      <div v-if="description || $slots.description" class="mt-2px text-sm text-secondary"><slot name="description">{{ description }}</slot></div>
    </div>
    <div class="rl-setting-row__control"><slot /></div>
  </div>
</template>
