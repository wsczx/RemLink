<script setup lang="ts">
// 轻量代码编辑区：等宽字体 + 行号槽（与文本区同步滚动），Tab 键插入两个空格，可标出错误行
import { computed, ref } from 'vue'

const props = defineProps<{ errorLine?: number; placeholder?: string; minRows?: number }>()
const model = defineModel<string>({ required: true })
const emit = defineEmits<{ save: [] }>()

const ta = ref<HTMLTextAreaElement | null>(null)
const gutter = ref<HTMLDivElement | null>(null)
const lineCount = computed(() => Math.max(model.value.split('\n').length, props.minRows || 1))

function syncScroll() {
  if (gutter.value && ta.value) gutter.value.scrollTop = ta.value.scrollTop
}

function onKeydown(e: KeyboardEvent) {
  // Ctrl/⌘ + S 保存
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    emit('save')
    return
  }
  if (e.key === 'Tab' && !e.shiftKey && ta.value) {
    e.preventDefault()
    const el = ta.value
    const { selectionStart: s, selectionEnd: end } = el
    model.value = model.value.slice(0, s) + '  ' + model.value.slice(end)
    requestAnimationFrame(() => { el.selectionStart = el.selectionEnd = s + 2 })
  }
}

// 跳转到指定行
function gotoLine(n: number) {
  const el = ta.value
  if (!el) return
  const lines = model.value.split('\n')
  const pos = lines.slice(0, Math.max(0, n - 1)).reduce((a, l) => a + l.length + 1, 0)
  el.focus()
  el.selectionStart = el.selectionEnd = pos
  el.scrollTop = Math.max(0, (n - 5) * 20)
  syncScroll()
}
defineExpose({ gotoLine })
</script>

<template>
  <div class="prf-editor">
    <div ref="gutter" class="prf-editor__gutter" aria-hidden="true">
      <div v-for="n in lineCount" :key="n" :class="n === errorLine && 'prf-editor__err'">{{ n }}</div>
    </div>
    <textarea
      ref="ta" v-model="model" class="prf-editor__ta" spellcheck="false" autocomplete="off" autocapitalize="off"
      wrap="off" :placeholder="placeholder" @scroll="syncScroll" @keydown="onKeydown"
    />
  </div>
</template>

<style scoped>
.prf-editor {
  display: flex; height: min(68vh, 720px); min-height: 360px; overflow: hidden;
  font-family: v-mono, ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; line-height: 20px;
}
.prf-editor__gutter {
  flex-shrink: 0; min-width: 52px; padding: 12px 10px 12px 0; overflow: hidden; text-align: right; user-select: none;
  color: var(--color-text-quaternary); background: var(--color-secondary); border-right: 1px solid var(--color-border-default);
}
.prf-editor__err { color: var(--color-text-error); font-weight: 600; }
.prf-editor__ta {
  flex: 1; min-width: 0; margin: 0; padding: 12px 16px; border: 0; outline: none; resize: none;
  background: transparent; color: var(--color-text-primary); font: inherit; line-height: inherit; tab-size: 2;
  white-space: pre; overflow: auto;
}
.prf-editor__ta::placeholder { color: var(--color-text-quaternary); }
</style>
