<script setup lang="ts">
// 抽屉表单内的分段：标题 + 说明，右侧可放操作；分段之间自动留白
defineProps<{ title: string; description?: string }>()
</script>

<template>
  <section class="fs">
    <div class="mb-12px flex items-start justify-between gap-sm">
      <div class="min-w-0">
        <div class="text-md font-semibold">{{ title }}</div>
        <div v-if="description" class="mt-2px text-sm text-secondary">{{ description }}</div>
      </div>
      <div v-if="$slots.extra" class="flex flex-shrink-0 items-center gap-xs"><slot name="extra" /></div>
    </div>
    <slot />
  </section>
</template>

<style scoped>
.fs + .fs { margin-top: 24px; padding-top: 20px; border-top: 1px solid var(--color-border-default); }
</style>
