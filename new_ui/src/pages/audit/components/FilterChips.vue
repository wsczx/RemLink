<script setup lang="ts">
// 筛选 chip 条：已生效的条件显示为「字段 值 ×」，点击 chip 修改；「添加筛选」从未使用的字段里选
import { computed, nextTick, ref } from 'vue'
import { NButton, NDropdown, NInput, NPopover } from 'naive-ui'
import Add from '~icons/fluent/add-16-regular'
import Dismiss from '~icons/fluent/dismiss-12-regular'
import Checkmark from '~icons/fluent/checkmark-16-regular'
import { type FilterField, type FilterValues, isEmpty } from './shared'

const props = defineProps<{ fields: FilterField[] }>()
const model = defineModel<FilterValues>({ required: true })
const emit = defineEmits<{ change: [] }>()

// 正在编辑的字段与草稿值
const editing = ref<string | null>(null)
const draft = ref<string>('')
const error = ref('')

const active = computed(() => props.fields.filter(f => !isEmpty(model.value[f.key]) || editing.value === f.key))
const addOptions = computed(() => props.fields.filter(f => !active.value.includes(f)).map(f => ({ label: f.label, key: f.key })))

const display = (f: FilterField) => {
  const v = model.value[f.key]
  if (isEmpty(v)) return '…'
  return f.type === 'select' ? f.options?.find(o => o.value === v)?.label ?? String(v) : String(v)
}

function open(key: string) {
  const v = model.value[key]
  draft.value = isEmpty(v) ? '' : String(v)
  error.value = ''
  editing.value = key
}
function close() { editing.value = null }
function set(key: string, v: string | number | null) {
  model.value = { ...model.value, [key]: v }
  editing.value = null
  emit('change')
}
function applyText(f: FilterField) {
  const v = draft.value.trim()
  const err = v && f.validate ? f.validate(v) : ''
  if (err) { error.value = err; return }
  set(f.key, v || null)
}
function remove(key: string) { set(key, null) }
function onAdd(key: string) { nextTick(() => open(key)) }
</script>

<template>
  <div class="flex flex-wrap items-center gap-6px">
    <NPopover
      v-for="f in active" :key="f.key" :show="editing === f.key" trigger="manual" placement="bottom-start"
      :show-arrow="false" raw class="rl-fc-pop" @clickoutside="close"
    >
      <template #trigger>
        <span class="rl-fc" :class="editing === f.key && 'rl-fc--edit'">
          <button class="rl-fc__main" @click="editing === f.key ? close() : open(f.key)">
            <span class="text-secondary">{{ f.label }}</span>
            <span class="truncate font-semibold" :class="f.mono && 'rl-mono'">{{ display(f) }}</span>
          </button>
          <button class="rl-fc__x" title="移除" @click.stop="remove(f.key)"><Dismiss /></button>
        </span>
      </template>
      <div class="w-260px p-8px">
        <div class="px-4px pb-6px text-sm text-tertiary">{{ f.label }}</div>
        <template v-if="f.type === 'select'">
          <div class="max-h-280px overflow-auto">
            <button
              v-for="o in f.options" :key="o.value" class="rl-fc-opt"
              :class="model[f.key] === o.value && 'rl-fc-opt--on'" @click="set(f.key, o.value)"
            >
              <span class="flex-1 truncate">{{ o.label }}</span>
              <Checkmark v-if="model[f.key] === o.value" />
            </button>
          </div>
        </template>
        <template v-else>
          <NInput
            v-model:value="draft" autofocus :placeholder="f.placeholder" :maxlength="f.maxlength" :class="f.mono && 'rl-mono'"
            :status="error ? 'error' : undefined" @keyup.enter="applyText(f)" @update:value="error = ''"
          />
          <div v-if="error" class="mt-4px px-2px text-sm text-error">{{ error }}</div>
          <div class="mt-8px flex justify-end gap-6px">
            <NButton size="small" quaternary @click="close">取消</NButton>
            <NButton size="small" type="primary" @click="applyText(f)">应用</NButton>
          </div>
        </template>
      </div>
    </NPopover>
    <NDropdown v-if="addOptions.length" trigger="click" :options="addOptions" placement="bottom-start" @select="onAdd">
      <button class="rl-fc-add"><Add />添加筛选</button>
    </NDropdown>
  </div>
</template>

<style scoped>
.rl-fc {
  display: inline-flex; align-items: center; height: 30px; max-width: 280px; border-radius: 15px;
  background: var(--color-fill); color: var(--color-text-primary); font-size: 13px;
  box-shadow: inset 0 0 0 1px transparent; transition: box-shadow .15s;
}
.rl-fc:hover, .rl-fc--edit { box-shadow: inset 0 0 0 1px var(--color-border-strong); }
.rl-fc__main {
  display: inline-flex; align-items: center; gap: 6px; min-width: 0; height: 100%; padding: 0 4px 0 12px;
  border: 0; background: none; color: inherit; font: inherit; cursor: pointer;
}
.rl-fc__x {
  display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; margin-right: 4px;
  border: 0; border-radius: 50%; background: none; color: var(--color-text-secondary); cursor: pointer;
}
.rl-fc__x:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.rl-fc-add {
  display: inline-flex; align-items: center; gap: 4px; height: 30px; padding: 0 12px; border-radius: 15px;
  border: 1px dashed var(--color-border-default); background: none; color: var(--color-text-secondary);
  font: inherit; font-size: 13px; cursor: pointer;
}
.rl-fc-add:hover { border-color: var(--color-border-strong); color: var(--color-text-primary); }
.rl-fc-opt {
  display: flex; align-items: center; width: 100%; height: 32px; padding: 0 8px; border: 0; border-radius: 6px;
  background: none; color: var(--color-text-primary); font: inherit; font-size: 14px; text-align: left; cursor: pointer;
}
.rl-fc-opt:hover { background: var(--color-action-hover); }
.rl-fc-opt--on { font-weight: 600; }
</style>

<style>
.rl-fc-pop { border-radius: 10px; background: var(--color-primary); border: 1px solid var(--color-border-default); box-shadow: 0 8px 24px color-mix(in srgb, var(--color-text-primary) 12%, transparent); }
</style>
