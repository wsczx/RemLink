<script setup lang="ts">
// 单选卡片：图标 + 标题 + 说明 + 右上角状态；选中时黑色描边（各设置页共用）
defineProps<{ active?: boolean; title: string; description?: string; disabled?: boolean }>()
defineEmits<{ select: [] }>()
</script>

<template>
  <button
    type="button" class="brand-choice" :class="[active && 'brand-choice--active', disabled && 'brand-choice--disabled']"
    :disabled="disabled" @click="$emit('select')"
  >
    <div class="flex items-start gap-12px">
      <div v-if="$slots.icon" class="brand-choice__icon"><slot name="icon" /></div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-8px">
          <span class="truncate text-md font-semibold text-primary">{{ title }}</span>
          <div class="flex-1" />
          <slot name="status" />
          <span class="brand-choice__radio" />
        </div>
        <div v-if="description || $slots.default" class="mt-2px text-sm text-secondary">
          <slot>{{ description }}</slot>
        </div>
      </div>
    </div>
  </button>
</template>

<style scoped>
.brand-choice {
  display: block; width: 100%; padding: 14px 16px; text-align: left; font: inherit; color: inherit; cursor: pointer;
  background: var(--color-primary); border: 1px solid var(--color-border-default); border-radius: 12px;
  transition: border-color .15s, box-shadow .15s;
}
.brand-choice:hover { border-color: var(--color-border-strong); }
.brand-choice--active { border-color: var(--color-action); box-shadow: inset 0 0 0 1px var(--color-action); }
.brand-choice--disabled { cursor: not-allowed; opacity: .5; }
.brand-choice__icon {
  display: flex; align-items: center; justify-content: center; flex-shrink: 0; width: 36px; height: 36px;
  border-radius: 10px; background: var(--color-fill); font-size: 18px; color: var(--color-text-secondary);
}
.brand-choice--active .brand-choice__icon { background: var(--color-action); color: var(--color-text-on-action); }
.brand-choice__radio {
  width: 16px; height: 16px; flex-shrink: 0; border-radius: 50%; border: 1px solid var(--color-border-strong);
  transition: border-width .15s;
}
.brand-choice--active .brand-choice__radio { border: 5px solid var(--color-action); }
</style>
