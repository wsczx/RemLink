<script setup lang="ts">
// 时间范围选择：左侧预设列表，右侧内嵌日历面板（自定义区间）
import { computed, ref } from 'vue'
import { NDatePicker, NPopover } from 'naive-ui'
import Clock from '~icons/fluent/clock-20-regular'
import ChevronDown from '~icons/fluent/chevron-down-16-regular'
import Checkmark from '~icons/fluent/checkmark-16-regular'
import { PRESETS, type RangeMode, type TimeSel, rangeLabel } from './shared'

const props = defineProps<{ mode: RangeMode }>()
const model = defineModel<TimeSel>({ required: true })
const show = ref(false)
const presets = computed(() => PRESETS.filter(p => p.modes.includes(props.mode)))
const label = computed(() => rangeLabel(model.value, props.mode))

function pick(key: string) {
  model.value = { preset: key, custom: null }
  show.value = false
}
function onCustom(v: [string, string] | null) {
  if (!v) return pick('all')
  model.value = { preset: 'custom', custom: v }
  show.value = false
}
</script>

<template>
  <NPopover v-model:show="show" trigger="click" placement="bottom-start" :show-arrow="false" raw class="rl-trp-pop">
    <template #trigger>
      <button class="rl-trp" :class="model.preset !== 'all' && 'rl-trp--on'">
        <Clock class="text-lg" />
        <span class="truncate">{{ label }}</span>
        <ChevronDown class="text-secondary" />
      </button>
    </template>
    <div class="flex">
      <div class="w-150px flex-shrink-0 border-r border-r-solid py-6px b-[var(--color-border-default)]">
        <div class="px-12px pb-4px pt-2px text-sm text-tertiary">快速选择</div>
        <button v-for="p in presets" :key="p.key" class="rl-trp-item" :class="model.preset === p.key && 'rl-trp-item--on'" @click="pick(p.key)">
          <span class="flex-1">{{ p.label }}</span>
          <Checkmark v-if="model.preset === p.key" />
        </button>
      </div>
      <div class="p-8px">
        <div class="px-4px pb-4px text-sm text-tertiary">自定义区间</div>
        <NDatePicker
          panel
          :type="mode === 'date' ? 'daterange' : 'datetimerange'"
          :value-format="mode === 'date' ? 'yyyy-MM-dd' : 'yyyy-MM-dd HH:mm:ss'"
          :formatted-value="model.preset === 'custom' ? model.custom : null"
          :default-time="mode === 'datetime' ? ['00:00:00', '23:59:59'] : undefined"
          :actions="['clear', 'confirm']"
          @update:formatted-value="onCustom"
        />
      </div>
    </div>
  </NPopover>
</template>

<style scoped>
.rl-trp {
  display: inline-flex; align-items: center; gap: 8px; height: 34px; max-width: 320px; padding: 0 10px 0 12px;
  border: 1px solid var(--color-border-default); border-radius: 8px; background: var(--color-primary);
  color: var(--color-text-primary); font: inherit; font-size: 14px; cursor: pointer; transition: border-color .15s;
}
.rl-trp:hover { border-color: var(--color-border-strong); }
.rl-trp--on { font-weight: 600; }
.rl-trp-item {
  display: flex; align-items: center; width: 100%; height: 32px; padding: 0 12px; border: 0; background: none;
  color: var(--color-text-primary); font: inherit; font-size: 14px; text-align: left; cursor: pointer;
}
.rl-trp-item:hover { background: var(--color-action-hover); }
.rl-trp-item--on { font-weight: 600; }
</style>

<style>
.rl-trp-pop { border-radius: 10px; overflow: hidden; background: var(--color-primary); border: 1px solid var(--color-border-default); box-shadow: 0 8px 24px color-mix(in srgb, var(--color-text-primary) 12%, transparent); }
</style>
