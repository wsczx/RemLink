<script setup lang="ts">
// 页头：标题 + 一行说明，右侧操作区；下方可放 tabs 插槽（下划线式）
defineProps<{ title: string; description?: string }>()
</script>

<template>
  <div class="pb-20px pt-28px">
    <div class="flex flex-wrap items-start justify-between gap-md">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-sm">
          <h1 class="m-0 text-4xl font-semibold leading-32px text-primary">{{ title }}</h1>
          <slot name="meta" />
        </div>
        <p v-if="description || $slots.description" class="m-0 mt-4px text-md text-secondary">
          <slot name="description">{{ description }}</slot>
        </p>
      </div>
      <div class="flex flex-shrink-0 items-center gap-sm">
        <slot />
      </div>
    </div>
    <div v-if="$slots.tabs" class="mt-16px">
      <slot name="tabs" />
    </div>
  </div>
</template>
