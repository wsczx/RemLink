<script setup lang="ts">
// 类终端日志视图：等宽字体、级别列、语法高亮；配色全部走主题 token
import { ref } from 'vue'
import TextBulletList from '~icons/fluent/text-bullet-list-ltr-20-regular'
import { highlightLog } from './SyslogHighlight'
import type { SyslogEntry } from './SyslogWs'

defineProps<{ entries: (SyslogEntry & { id: number })[]; keyword?: string; emptyText: string; emptyHint?: string }>()
const emit = defineEmits<{ scroll: [el: HTMLElement] }>()
const el = ref<HTMLElement>()
defineExpose({ el })

const LEVEL_CLS: Record<string, string> = {
  trace: 'text-tertiary', debug: 'text-secondary', info: 'text-info', warn: 'text-warning', error: 'text-error', fatal: 'text-error',
}
</script>

<template>
  <div ref="el" class="slg-view rl-mono" @scroll="emit('scroll', el!)">
    <div v-if="!entries.length" class="flex h-full flex-col items-center justify-center gap-6px text-tertiary">
      <TextBulletList class="text-4xl" />
      <span class="text-md">{{ emptyText }}</span>
      <span v-if="emptyHint" class="text-sm">{{ emptyHint }}</span>
    </div>
    <div v-for="e in entries" :key="e.id" class="slg-line" :class="`slg-line--${e.level.toLowerCase()}`">
      <span class="slg-time">{{ e.time }}</span>
      <span class="slg-level" :class="LEVEL_CLS[e.level.toLowerCase()]">{{ e.level.toUpperCase() }}</span>
      <!-- eslint-disable-next-line vue/no-v-html -- 已在 highlightLog 中转义 -->
      <span class="slg-msg" v-html="highlightLog(e.msg, keyword)" />
    </div>
    <slot />
  </div>
</template>

<style scoped>
.slg-view {
  height: calc(100vh - 330px);
  min-height: 360px;
  overflow: auto;
  padding: 8px 0;
  background: var(--color-secondary);
  font-size: 12.5px;
  line-height: 20px;
  border-radius: 0 0 12px 12px;
}
.slg-line {
  display: grid;
  grid-template-columns: 150px 52px minmax(0, 1fr);
  gap: 12px;
  padding: 0 20px;
  border-left: 2px solid transparent;
}
.slg-line:hover { background: var(--color-action-hover); }
.slg-line--warn { border-left-color: var(--color-text-warning); }
.slg-line--error, .slg-line--fatal { border-left-color: var(--color-text-error); }
.slg-time { color: var(--color-text-tertiary); white-space: nowrap; }
.slg-level { font-weight: 600; }
.slg-msg { color: var(--color-text-default); white-space: pre-wrap; word-break: break-all; }

.slg-msg :deep(.slg-err) { color: var(--color-text-error); font-weight: 600; }
.slg-msg :deep(.slg-warn) { color: var(--color-text-warning); }
.slg-msg :deep(.slg-ok) { color: var(--color-text-success); }
.slg-msg :deep(.slg-id) { color: var(--color-code-key); }
.slg-msg :deep(.slg-group) { color: var(--color-semantics-teal); }
.slg-msg :deep(.slg-addr) { color: var(--color-code-number); }
.slg-msg :deep(.slg-domain) { color: var(--color-code-string); }
.slg-msg :deep(.slg-net) { color: var(--color-text-keyword); }
.slg-msg :deep(.slg-url) { color: var(--color-text-info); text-decoration: underline; text-underline-offset: 2px; }
.slg-msg :deep(.slg-mark) {
  color: var(--color-text-primary);
  background: color-mix(in srgb, var(--color-text-warning) 30%, transparent);
  border-radius: 2px;
}

@media (max-width: 640px) {
  .slg-line { grid-template-columns: minmax(0, 1fr); gap: 0; padding: 4px 16px; }
}
</style>
