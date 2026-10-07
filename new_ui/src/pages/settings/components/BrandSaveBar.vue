<script setup lang="ts">
// 吸底保存条：表单变脏时高亮，提供「放弃修改 / 保存」
import { NButton } from 'naive-ui'

defineProps<{ dirty: boolean; saving?: boolean; hint?: string }>()
defineEmits<{ reset: []; save: [] }>()
</script>

<template>
  <div class="brand-savebar" :class="dirty && 'brand-savebar--dirty'">
    <span class="brand-savebar__dot" />
    <span class="min-w-0 truncate text-md" :class="dirty ? 'font-semibold text-primary' : 'text-tertiary'">
      {{ dirty ? '有未保存的修改' : (hint || '所有修改已保存') }}
    </span>
    <div class="flex-1" />
    <NButton :disabled="!dirty || saving" @click="$emit('reset')">放弃修改</NButton>
    <NButton type="primary" :disabled="!dirty" :loading="saving" @click="$emit('save')">保存</NButton>
  </div>
</template>

<style scoped>
.brand-savebar {
  position: sticky; bottom: 0; z-index: 10; display: flex; align-items: center; gap: 10px;
  margin-top: 16px; padding: 12px 0; background: var(--color-body);
  border-top: 1px solid var(--color-border-default);
}
.brand-savebar__dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; background: var(--color-text-quaternary); }
.brand-savebar--dirty { border-top-color: var(--color-border-strong); }
.brand-savebar--dirty .brand-savebar__dot { background: var(--color-text-warning); }
</style>
