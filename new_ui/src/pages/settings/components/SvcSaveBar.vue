<script setup lang="ts">
// 吸底保存条：表单变脏时高亮，提供「放弃修改 / 保存」
import { NButton } from 'naive-ui'

const props = defineProps<{ dirty: number | boolean; saving?: boolean; text?: string; hint?: string }>()
defineEmits<{ reset: []; save: [] }>()
</script>

<template>
  <div class="svc-savebar" :class="{ 'svc-savebar--dirty': !!props.dirty }">
    <span class="svc-savebar__dot" />
    <span class="text-md" :class="dirty ? 'font-semibold text-primary' : 'text-tertiary'">
      {{ dirty ? (text || '有未保存的修改') : '所有修改已保存' }}
    </span>
    <span v-if="hint" class="hidden text-sm text-tertiary md:inline">{{ hint }}</span>
    <div class="flex-1" />
    <NButton :disabled="!dirty || saving" @click="$emit('reset')">放弃修改</NButton>
    <NButton type="primary" :disabled="!dirty" :loading="saving" @click="$emit('save')">保存</NButton>
  </div>
</template>

<style scoped>
.svc-savebar {
  position: sticky; bottom: 0; z-index: 10; display: flex; align-items: center; gap: 10px;
  margin-top: 16px; padding: 12px 0 16px; background: var(--color-body);
  border-top: 1px solid var(--color-border-default);
}
.svc-savebar--dirty { border-top-color: var(--color-border-strong); }
.svc-savebar__dot { width: 6px; height: 6px; border-radius: 50%; background: var(--color-text-quaternary); }
.svc-savebar--dirty .svc-savebar__dot { background: var(--color-text-warning); }
</style>
