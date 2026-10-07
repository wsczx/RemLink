<script setup lang="ts" generic="T extends string | number">
// 分段 tab：灰色轨道 + 白色激活块
withDefaults(defineProps<{ options: { label: string; value: T }[]; size?: 'small' | 'large' }>(), { size: 'large' })
const model = defineModel<T>({ required: true })
</script>

<template>
  <div class="inline-flex gap-2px rounded-6px bg-fill p-3px">
    <div
      v-for="o in options" :key="o.value"
      class="flex cursor-pointer select-none items-center justify-center whitespace-nowrap rounded-6px transition-colors"
      :class="[
        size === 'large' ? 'min-h-38px min-w-100px px-18px text-lg' : 'min-h-26px min-w-48px px-sm text-sm',
        model === o.value ? 'ax-block-tab--active' : 'text-secondary hover:bg-action-hover active:bg-action-pressed',
      ]"
      @click="model = o.value"
    >
      {{ o.label }}
    </div>
  </div>
</template>
