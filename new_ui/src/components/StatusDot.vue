<script setup lang="ts">
// 状态：6px 圆点 + 文字，颜色只表达语义
withDefaults(defineProps<{ tone?: 'success' | 'warning' | 'error' | 'info' | 'muted'; pulse?: boolean }>(), { tone: 'muted' })
</script>

<template>
  <span class="rl-status" :class="`rl-status--${tone}`">
    <span class="rl-status__dot" :class="pulse && 'rl-status__dot--pulse'" />
    <span class="truncate"><slot /></span>
  </span>
</template>
