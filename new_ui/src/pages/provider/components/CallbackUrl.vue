<script setup lang="ts">
// 回调地址提示：说明文字 + 可复制的 URL
import Copy from '~icons/fluent/copy-20-regular'
import { ui } from '@/utils/ui'

const props = defineProps<{ label: string; url: string; warn?: boolean }>()

async function copy() {
  try {
    await navigator.clipboard.writeText(props.url)
    ui.message.success('已复制')
  } catch {
    ui.message.error('复制失败，请手动选择复制')
  }
}
</script>

<template>
  <div class="rounded-8px bg-fill px-12px py-10px" :class="warn && 'pv-cb--warn'">
    <div class="text-sm text-secondary"><slot name="label">{{ label }}</slot></div>
    <div class="mt-4px flex items-center gap-6px">
      <code class="rl-mono min-w-0 flex-1 break-all text-primary">{{ url }}</code>
      <button type="button" class="rl-icon-btn h-28px! w-28px! text-md!" title="复制" @click="copy"><Copy /></button>
    </div>
    <div v-if="$slots.default" class="mt-4px text-sm text-secondary"><slot /></div>
  </div>
</template>

<style scoped>
.pv-cb--warn { box-shadow: inset 3px 0 0 var(--color-text-warning); }
</style>
