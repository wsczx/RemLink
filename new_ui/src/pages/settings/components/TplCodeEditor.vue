<script setup lang="ts">
// HTML 模板编辑器：源码 / 分屏 / 预览三种视图；预览用 iframe srcdoc 隔离样式且禁用脚本
import { ref } from 'vue'

const props = withDefaults(defineProps<{ placeholder?: string; preview?: string; height?: number }>(), { height: 420 })
const model = defineModel<string>({ default: '' })
const view = ref<'split' | 'code' | 'preview'>('split')
const views = [
  { value: 'code', label: '源码' },
  { value: 'split', label: '分屏' },
  { value: 'preview', label: '预览' },
] as const
</script>

<template>
  <div class="tce">
    <div class="tce-bar">
      <slot name="toolbar" />
      <div class="flex-1" />
      <div class="tce-seg">
        <button v-for="v in views" :key="v.value" type="button" :class="view === v.value && 'tce-seg--on'" @click="view = v.value">{{ v.label }}</button>
      </div>
    </div>
    <div class="tce-body" :class="`tce-body--${view}`" :style="{ height: `${props.height}px` }">
      <textarea v-show="view !== 'preview'" v-model="model" :placeholder="placeholder" spellcheck="false" class="rl-mono tce-code" />
      <div v-show="view !== 'code'" class="tce-preview">
        <iframe v-if="(preview ?? model)" :srcdoc="preview ?? model" sandbox="" title="预览" class="block h-full w-full b-0" />
        <div v-else class="flex h-full items-center justify-center text-md text-tertiary">暂无内容</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tce { overflow: hidden; border: 1px solid var(--color-border-default); border-radius: 10px; }
.tce-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 8px 10px; border-bottom: 1px solid var(--color-border-default); background: var(--color-secondary); }
.tce-seg { display: inline-flex; padding: 2px; border-radius: 8px; background: var(--color-fill); }
.tce-seg button {
  height: 26px; padding: 0 10px; border: 0; border-radius: 6px; background: transparent; cursor: pointer;
  font: inherit; font-size: 12px; color: var(--color-text-secondary);
}
.tce-seg .tce-seg--on { background: var(--color-primary); color: var(--color-text-primary); font-weight: 600; box-shadow: 0 0 0 1px var(--color-border-default); }
.tce-body { display: grid; grid-template-columns: minmax(0, 1fr); }
.tce-body--split { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
.tce-code {
  width: 100%; height: 100%; min-height: 0; padding: 12px 14px; border: 0; outline: none; resize: none; box-sizing: border-box;
  background: var(--color-primary); color: var(--color-text-primary); line-height: 1.6; tab-size: 2;
}
.tce-code::placeholder { color: var(--color-text-quaternary); }
/* 邮件/首页 HTML 预览固定白底，模拟收件端 */
.tce-preview { min-width: 0; height: 100%; overflow: hidden; background: white; }
.tce-body--split .tce-preview { border-left: 1px solid var(--color-border-default); }
@media (max-width: 899px) {
  .tce-body--split { grid-template-columns: minmax(0, 1fr); grid-template-rows: 1fr 1fr; }
  .tce-body--split .tce-preview { border-left: 0; border-top: 1px solid var(--color-border-default); }
}
</style>
