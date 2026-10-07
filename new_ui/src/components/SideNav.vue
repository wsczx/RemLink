<script setup lang="ts">
// 两级侧边导航：一级模块可展开，二级为页面；折叠态只显示图标，悬停弹出二级菜单
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { NIcon, NPopover } from 'naive-ui'
import ChevronRight from '~icons/fluent/chevron-right-16-regular'
import Open from '~icons/fluent/open-16-regular'
import { findNav, isLeafActive, type NavLeaf, type NavNode } from '@/layout/nav'

const props = defineProps<{ nav: NavNode[]; collapsed?: boolean }>()
const emit = defineEmits<{ external: [leaf: NavLeaf] }>()
const route = useRoute()

// 展开状态：当前路由所在模块自动展开，其余保持用户操作
const open = ref<Set<string>>(new Set())
watch(() => route.path, p => {
  const hit = findNav(props.nav, p)
  if (hit?.leaf) open.value = new Set([...open.value, hit.node.key])
}, { immediate: true })

function toggle(key: string) {
  const s = new Set(open.value)
  s.has(key) ? s.delete(key) : s.add(key)
  open.value = s
}
const nodeActive = (n: NavNode) => (n.to ? isLeafActive(route.path, n) : !!n.children?.some(c => isLeafActive(route.path, c)))
</script>

<template>
  <nav class="flex flex-col gap-2px">
    <template v-for="n in nav" :key="n.key">
      <!-- 单页模块 -->
      <RouterLink v-if="n.to" :to="n.to" class="rl-nav-l1" :class="[nodeActive(n) && 'rl-nav-l1--active', collapsed && 'rl-nav-l1--mini']" :title="collapsed ? n.label : undefined">
        <NIcon :size="18" class="flex-shrink-0"><component :is="n.icon" /></NIcon>
        <span v-if="!collapsed" class="flex-1 truncate">{{ n.label }}</span>
      </RouterLink>

      <!-- 折叠态：悬停弹出二级 -->
      <NPopover v-else-if="collapsed" placement="right-start" trigger="hover" :show-arrow="false" raw>
        <template #trigger>
          <div class="rl-nav-l1 rl-nav-l1--mini" :class="nodeActive(n) && 'rl-nav-l1--active'">
            <NIcon :size="18"><component :is="n.icon" /></NIcon>
          </div>
        </template>
        <div class="rl-nav-pop">
          <div class="px-10px pb-4px pt-2px text-sm text-tertiary">{{ n.label }}</div>
          <template v-for="c in n.children" :key="c.label">
            <RouterLink v-if="c.to" :to="c.to" class="rl-nav-l2" :class="isLeafActive(route.path, c) && 'rl-nav-l2--active'">{{ c.label }}</RouterLink>
            <div v-else class="rl-nav-l2" @click="emit('external', c)">{{ c.label }}<NIcon :size="12" class="ml-auto text-tertiary"><Open /></NIcon></div>
          </template>
        </div>
      </NPopover>

      <!-- 展开态：可折叠分组 -->
      <div v-else>
        <div class="rl-nav-l1" :class="nodeActive(n) && !open.has(n.key) && 'rl-nav-l1--active'" @click="toggle(n.key)">
          <NIcon :size="18" class="flex-shrink-0"><component :is="n.icon" /></NIcon>
          <span class="flex-1 truncate">{{ n.label }}</span>
          <NIcon :size="14" class="text-tertiary transition-transform duration-150" :class="open.has(n.key) && 'rotate-90'"><ChevronRight /></NIcon>
        </div>
        <div v-show="open.has(n.key)" class="rl-nav-sub">
          <template v-for="c in n.children" :key="c.label">
            <RouterLink v-if="c.to" :to="c.to" class="rl-nav-l2" :class="isLeafActive(route.path, c) && 'rl-nav-l2--active'">{{ c.label }}</RouterLink>
            <div v-else class="rl-nav-l2" @click="emit('external', c)">{{ c.label }}<NIcon :size="12" class="ml-auto text-tertiary"><Open /></NIcon></div>
          </template>
        </div>
      </div>
    </template>
  </nav>
</template>

<style>
.rl-nav-l1 {
  display: flex; align-items: center; gap: 10px; height: 36px; padding: 0 10px;
  border-radius: 8px; cursor: pointer; user-select: none;
  color: var(--color-text-secondary); font-size: 14px;
  transition: background-color .15s, color .15s;
}
.rl-nav-l1:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.rl-nav-l1--mini { justify-content: center; padding: 0; }
.rl-nav-l1--active { color: var(--color-text-primary); font-weight: 600; background: var(--color-primary); box-shadow: inset 0 0 0 1px var(--color-border-default); }
.rl-nav-sub { position: relative; margin: 2px 0 6px 19px; padding-left: 10px; border-left: 1px solid var(--color-border-default); display: flex; flex-direction: column; gap: 1px; }
.rl-nav-l2 {
  position: relative; display: flex; align-items: center; height: 32px; padding: 0 10px; border-radius: 6px;
  color: var(--color-text-secondary); font-size: 13px; cursor: pointer; transition: background-color .15s, color .15s;
}
.rl-nav-l2:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.rl-nav-l2--active { color: var(--color-text-primary); font-weight: 600; background: var(--color-action-hover); }
.rl-nav-sub .rl-nav-l2--active::before { content: ''; position: absolute; left: -11px; top: 8px; bottom: 8px; width: 2px; border-radius: 2px; background: var(--color-action); }
.rl-nav-pop { min-width: 180px; padding: 8px 6px; border-radius: 10px; background: var(--color-primary); border: 1px solid var(--color-border-default); box-shadow: 0 8px 24px rgba(0,0,0,.08); }
</style>
