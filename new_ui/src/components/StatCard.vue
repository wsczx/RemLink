<script setup lang="ts">
// 指标卡：左上标签，数值 + 单位，右下迷你趋势 / 底部说明
import type { Component } from 'vue'
defineProps<{ label: string; value: string | number; unit?: string; hint?: string; icon?: Component; to?: string; tone?: 'success' | 'warning' | 'error' | 'info' }>()
</script>

<template>
  <component :is="to ? 'RouterLink' : 'div'" v-bind="to ? { to } : {}" class="rl-panel rl-stat" :class="to && 'rl-stat--link'">
    <div class="flex items-center gap-xs text-md text-secondary">
      <component :is="icon" v-if="icon" class="text-lg" />
      <span class="truncate">{{ label }}</span>
      <div class="flex-1" />
      <slot name="extra" />
    </div>
    <div class="mt-10px flex items-baseline gap-4px">
      <span class="text-[28px] font-semibold leading-34px tabular-nums" :class="tone && `text-${tone}`">{{ value }}</span>
      <span v-if="unit" class="text-md text-secondary">{{ unit }}</span>
      <slot name="delta" />
    </div>
    <div v-if="hint || $slots.hint" class="mt-4px truncate text-sm text-tertiary"><slot name="hint">{{ hint }}</slot></div>
    <div v-if="$slots.default" class="mt-12px"><slot /></div>
  </component>
</template>
