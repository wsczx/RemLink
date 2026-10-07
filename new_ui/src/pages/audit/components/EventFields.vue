<script setup lang="ts">
// 展开行：完整字段网格；每个字段可「按此筛选」或复制，底部可复制原始 JSON
import { NTooltip } from 'naive-ui'
import Filter from '~icons/fluent/filter-16-regular'
import Copy from '~icons/fluent/copy-16-regular'
import CountryFlag from '@/components/CountryFlag.vue'
import { ui } from '@/utils/ui'
import type { EventField } from './shared'

const props = defineProps<{ fields: EventField[]; raw?: unknown }>()

async function copy(text: string, tip = '已复制') {
  try {
    await navigator.clipboard.writeText(text)
    ui.message.success(tip)
  } catch {
    ui.message.error('复制失败，请手动选择文本')
  }
}
const empty = (v: unknown) => v === null || v === undefined || v === ''
</script>

<template>
  <div class="rl-evf">
    <div class="rl-evf__grid">
      <div v-for="f in props.fields" :key="f.label" class="rl-evf__item" :class="f.wide && 'rl-evf__item--wide'">
        <div class="text-sm text-tertiary">{{ f.label }}</div>
        <div class="flex min-w-0 items-start gap-6px">
          <CountryFlag v-if="f.flag !== undefined && !empty(f.value)" :code="f.flag" with-name class="mt-2px flex-shrink-0 text-sm text-secondary" />
          <span class="min-w-0 break-all" :class="[f.mono && 'rl-mono', empty(f.value) && 'text-quaternary']">{{ empty(f.value) ? '—' : f.value }}</span>
          <span v-if="!empty(f.value)" class="rl-evf__acts">
            <NTooltip v-if="f.onFilter" :show-arrow="false">
              <template #trigger><button class="rl-evf__btn" @click="f.onFilter"><Filter /></button></template>
              按此筛选
            </NTooltip>
            <button class="rl-evf__btn" title="复制" @click="copy(String(f.value))"><Copy /></button>
          </span>
        </div>
      </div>
    </div>
    <div v-if="raw" class="mt-12px flex justify-end">
      <button class="rl-evf__raw" @click="copy(JSON.stringify(raw, null, 2), '已复制原始 JSON')"><Copy />复制原始 JSON</button>
    </div>
  </div>
</template>

<style scoped>
.rl-evf { padding: 4px 4px 4px 28px; }
.rl-evf__grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 14px 24px; }
.rl-evf__item { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.rl-evf__item--wide { grid-column: 1 / -1; }
.rl-evf__acts { display: inline-flex; flex-shrink: 0; gap: 2px; opacity: 0; transition: opacity .12s; }
.rl-evf__item:hover .rl-evf__acts { opacity: 1; }
.rl-evf__btn {
  display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border: 0; border-radius: 5px;
  background: none; color: var(--color-text-secondary); cursor: pointer;
}
.rl-evf__btn:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.rl-evf__raw {
  display: inline-flex; align-items: center; gap: 4px; height: 26px; padding: 0 8px; border: 0; border-radius: 6px;
  background: none; color: var(--color-text-secondary); font: inherit; font-size: 12px; cursor: pointer;
}
.rl-evf__raw:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
@media (hover: none) { .rl-evf__acts { opacity: 1; } }
</style>
