<script setup lang="ts">
import { computed } from 'vue'
import { pwdStrength } from './shared'

// 密码强度条：4 段，颜色只表达语义
const props = defineProps<{ password: string }>()
const level = computed(() => pwdStrength(props.password))
const text = computed(() => ['', '弱', '中', '强', '很强'][level.value])
const tone = computed(() => ['', 'error', 'warning', 'info', 'success'][level.value])
</script>

<template>
  <div v-if="level > 0" class="flex items-center gap-10px">
    <div class="grid flex-1 grid-cols-4 gap-4px">
      <span
        v-for="i in 4" :key="i" class="h-4px rounded-2px"
        :style="{ background: i <= level ? `var(--color-text-${tone})` : 'var(--color-fill)' }"
      />
    </div>
    <span class="w-28px text-right text-sm" :class="`text-${tone}`">{{ text }}</span>
  </div>
</template>
