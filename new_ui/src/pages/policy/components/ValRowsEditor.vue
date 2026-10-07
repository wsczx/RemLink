<script setup lang="ts">
// 多行条目编辑器：{ val, note } 列表，可增删；条目过多时折叠为摘要并限制高度
import { computed, ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import type { ValData } from './policy'

const props = withDefaults(defineProps<{
  valLabel?: string
  valPlaceholder?: string
  addText?: string
  minRows?: number
  collapseAt?: number
  emptyText?: string
}>(), { valLabel: '地址', valPlaceholder: '', addText: '添加', minRows: 0, collapseAt: 0, emptyText: '暂无条目' })

const rows = defineModel<ValData[]>({ required: true })
const expanded = ref(false)
const collapsed = computed(() => props.collapseAt > 0 && rows.value.length > props.collapseAt && !expanded.value)
const scroll = computed(() => props.collapseAt > 0 && rows.value.length > props.collapseAt)

function add() { rows.value.push({ val: '', note: '' }) }
function remove(i: number) { rows.value.splice(i, 1) }
</script>

<template>
  <div class="vr">
    <div v-if="collapsed" class="flex items-center gap-sm px-12px py-10px text-md text-secondary">
      <span>共 <b class="text-primary">{{ rows.length }}</b> 条，建议使用「批量编辑」管理</span>
      <NButton text type="info" @click="expanded = true">展开逐条编辑</NButton>
    </div>
    <template v-else>
      <div v-if="rows.length" class="vr__head">
        <span class="w-24px" />
        <span class="flex-[3]">{{ valLabel }}</span>
        <span class="flex-[2]">备注</span>
        <span class="w-32px" />
      </div>
      <div :class="scroll && 'max-h-360px overflow-y-auto'">
        <div v-for="(item, i) in rows" :key="i" class="vr__row">
          <span class="w-24px text-right text-sm text-tertiary tabular-nums">{{ i + 1 }}</span>
          <NInput v-model:value="item.val" :placeholder="valPlaceholder" class="flex-[3] rl-mono" />
          <NInput v-model:value="item.note" placeholder="备注" class="flex-[2]" />
          <button type="button" class="rl-icon-btn vr__del" :disabled="rows.length <= minRows" title="删除" @click="remove(i)">
            <Delete />
          </button>
        </div>
      </div>
      <div v-if="!rows.length" class="px-12px py-14px text-center text-sm text-tertiary">{{ emptyText }}</div>
    </template>
    <div class="vr__foot">
      <NButton text size="small" @click="add"><template #icon><Add /></template>{{ addText }}</NButton>
      <slot name="extra" />
    </div>
  </div>
</template>

<style scoped>
.vr { border: 1px solid var(--color-border-default); border-radius: 8px; }
.vr__head { display: flex; align-items: center; gap: 8px; padding: 8px 12px 4px; font-size: 12px; color: var(--color-text-tertiary); }
.vr__row { display: flex; align-items: center; gap: 8px; padding: 4px 12px; }
.vr__foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 12px; border-top: 1px solid var(--color-border-default); }
.vr__del { width: 32px; flex-shrink: 0; }
.vr__del:hover:not(:disabled) { color: var(--color-text-error); }
.vr__del:disabled { opacity: .35; cursor: not-allowed; }
</style>
