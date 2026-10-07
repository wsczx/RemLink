<script setup lang="ts">
// 面板：1px 边框 + 12px 圆角；可选标题栏（标题 / 副标题 / 右侧 extra），标题栏下一条细线
withDefaults(defineProps<{ title?: string; subtitle?: string; padded?: boolean }>(), { padded: true })
</script>

<template>
  <section class="rl-panel">
    <header v-if="title || $slots.header || $slots.extra" class="rl-panel__head">
      <slot name="header">
        <div class="min-w-0">
          <div class="truncate text-lg font-semibold">{{ title }}</div>
          <div v-if="subtitle" class="truncate text-sm text-secondary">{{ subtitle }}</div>
        </div>
      </slot>
      <div class="flex flex-shrink-0 items-center gap-xs"><slot name="extra" /></div>
    </header>
    <div v-if="$slots.toolbar" class="rl-panel__toolbar"><slot name="toolbar" /></div>
    <div :class="padded ? 'p-20px' : ''"><slot /></div>
    <footer v-if="$slots.footer" class="rl-panel__foot"><slot name="footer" /></footer>
  </section>
</template>
