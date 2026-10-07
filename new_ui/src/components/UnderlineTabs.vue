<script setup lang="ts" generic="T extends string | number">
// 下划线 tab：用于页头下方切换视图，带可选计数
defineProps<{ options: { label: string; value: T; count?: number }[] }>()
const model = defineModel<T>({ required: true })
</script>

<template>
  <div class="flex gap-20px b-b-1px b-b-solid b-[var(--color-border-default)]">
    <button
      v-for="o in options" :key="o.value" type="button"
      class="rl-utab" :class="model === o.value && 'rl-utab--active'"
      @click="model = o.value"
    >
      {{ o.label }}
      <span v-if="o.count != null" class="ml-4px rounded-full bg-fill px-6px text-sm text-secondary">{{ o.count }}</span>
    </button>
  </div>
</template>
