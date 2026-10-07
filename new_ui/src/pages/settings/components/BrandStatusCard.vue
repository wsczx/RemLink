<script setup lang="ts">
// 页面顶部状态卡：左侧「图标 + 标题 + 状态 + 摘要」，右侧可选的检查项/用途区（各设置页共用）
defineProps<{ title: string; tone?: 'success' | 'warning' | 'error' | 'info' | 'muted' }>()
</script>

<template>
  <section class="brand-status rl-panel" :class="`brand-status--${tone || 'muted'}`">
    <div class="brand-status__main">
      <div class="brand-status__icon"><slot name="icon" /></div>
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-10px">
          <span class="text-xl font-semibold">{{ title }}</span>
          <slot name="status" />
        </div>
        <div class="mt-4px text-md text-secondary"><slot /></div>
        <div v-if="$slots.meta" class="mt-12px flex flex-wrap items-center gap-8px"><slot name="meta" /></div>
      </div>
    </div>
    <div v-if="$slots.aside" class="brand-status__aside"><slot name="aside" /></div>
  </section>
</template>

<style scoped>
.brand-status { display: grid; grid-template-columns: minmax(0, 1fr); overflow: hidden; position: relative; }
.brand-status::before {
  content: ''; position: absolute; left: 0; top: 0; bottom: 0; width: 3px; background: var(--color-text-quaternary);
}
.brand-status--success::before { background: var(--color-text-success); }
.brand-status--warning::before { background: var(--color-text-warning); }
.brand-status--error::before { background: var(--color-text-error); }
.brand-status--info::before { background: var(--color-text-info); }
.brand-status__main { display: flex; gap: 16px; padding: 20px 24px; min-width: 0; }
.brand-status__icon {
  display: flex; align-items: center; justify-content: center; flex-shrink: 0; width: 44px; height: 44px;
  border-radius: 12px; background: var(--color-fill); font-size: 22px; color: var(--color-text-primary);
}
.brand-status__aside { padding: 16px 24px; border-top: 1px solid var(--color-border-default); background: var(--color-secondary); }
@media (min-width: 1100px) {
  .brand-status { grid-template-columns: minmax(0, 1fr) minmax(320px, 400px); }
  .brand-status__aside { border-top: 0; border-left: 1px solid var(--color-border-default); padding: 20px 24px; }
}
</style>
