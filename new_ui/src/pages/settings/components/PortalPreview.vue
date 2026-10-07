<script setup lang="ts">
// 门户首页迷你预览：顶栏、公告、主题色、快捷链接、卡片显隐、客户端指引
import { computed } from 'vue'
import Shield from '~icons/fluent/shield-checkmark-20-regular'
import LinkIcon from '~icons/fluent/link-20-regular'
import { normalizeImageSrc } from '@/utils/brand'
import { CARD_OPTIONS, isHexColor, linkIconKind, normHex, type GuideGroup, type QuickLink } from './PortalDefs'

const props = defineProps<{
  brand: { title?: string; logo?: string; desc?: string }
  themeColor?: string
  announcementOn: boolean
  announcement?: string
  level?: string
  linksOn: boolean
  links: QuickLink[]
  guideOn: boolean
  guide: GuideGroup[]
  cards: Record<string, boolean>
}>()

const accent = computed(() => (isHexColor(props.themeColor) ? normHex(props.themeColor!) : 'var(--color-action)'))
// 主题色上的文字：自定义色按亮度取黑/白，未设置时沿用控制台 action 配色
const onAccent = computed(() => {
  if (!isHexColor(props.themeColor)) return 'var(--color-text-on-action)'
  const h = normHex(props.themeColor!).slice(1)
  const [r, g, b] = [0, 2, 4].map(i => parseInt(h.slice(i, i + 2), 16))
  return 0.299 * r + 0.587 * g + 0.114 * b > 160 ? 'black' : 'white'
})
const levelTone = computed(() => (['success', 'warning', 'error'].includes(props.level || '') ? props.level : 'info'))
const logoSrc = computed(() => normalizeImageSrc(props.brand.logo))
const visible = computed(() => CARD_OPTIONS.filter(c => props.cards[c.key] !== false))
const hidden = computed(() => CARD_OPTIONS.length - visible.value.length)
const validLinks = computed(() => props.links.filter(l => l.label || l.url))
</script>

<template>
  <div class="pp" :style="{ '--pv-accent': accent, '--pv-on-accent': onAccent }">
    <!-- 顶栏 -->
    <div class="pp-top">
      <img v-if="logoSrc" :src="logoSrc" alt="" class="h-18px w-18px object-contain">
      <span v-else class="pp-logo"><Shield /></span>
      <span class="truncate text-sm font-semibold">{{ brand.title || 'RemLink' }}</span>
      <div class="flex-1" />
      <span class="pp-avatar">Z</span>
    </div>

    <div class="grid gap-8px p-10px">
      <!-- 公告 -->
      <div v-if="announcementOn && announcement" class="pp-alert" :class="`text-${levelTone}`">
        <div class="pp-html text-primary" v-html="announcement" />
      </div>

      <!-- 欢迎区（主题色） -->
      <div class="pp-welcome">
        <div class="text-sm font-semibold">下午好，zhangsan</div>
        <div class="pp-xs opacity-80">已连接 1 台设备 · 账号有效</div>
        <span class="pp-btn">连接 VPN</span>
      </div>

      <!-- 快捷链接 -->
      <div v-if="linksOn && validLinks.length" class="flex flex-wrap gap-6px">
        <span v-for="(l, i) in validLinks.slice(0, 6)" :key="i" class="pp-link">
          <img v-if="linkIconKind(l.icon) === 'image'" :src="l.icon" alt="" class="h-12px w-12px object-contain">
          <span v-else-if="linkIconKind(l.icon) === 'emoji'" class="leading-none">{{ l.icon }}</span>
          <LinkIcon v-else class="pp-accent" />
          <span class="truncate">{{ l.label || l.url }}</span>
        </span>
      </div>

      <!-- 卡片 -->
      <div class="grid grid-cols-2 gap-6px">
        <div v-if="guideOn && guide.length" class="pp-card col-span-2">
          <div class="pp-xs font-semibold text-primary">客户端连接指引</div>
          <div class="mt-4px flex flex-wrap gap-4px">
            <span v-for="(g, i) in guide.slice(0, 5)" :key="i" class="pp-chip" :class="i === 0 && 'pp-chip--on'">{{ g.name || '未命名' }}</span>
          </div>
          <div class="pp-bar mt-6px w-4/5" /><div class="pp-bar mt-4px w-3/5" />
        </div>
        <div v-for="c in visible" :key="c.key" class="pp-card">
          <div class="pp-xs truncate font-semibold text-primary">{{ c.label }}</div>
          <div class="pp-bar mt-6px w-4/5" /><div class="pp-bar mt-4px w-1/2" />
        </div>
      </div>
      <div v-if="hidden" class="pp-xs text-center text-tertiary">已对用户隐藏 {{ hidden }} 个卡片</div>
    </div>
  </div>
</template>

<style scoped>
.pp { overflow: hidden; border: 1px solid var(--color-border-default); border-radius: 10px; background: var(--color-secondary); }
.pp-top { display: flex; align-items: center; gap: 6px; height: 36px; padding: 0 10px; background: var(--color-primary); border-bottom: 1px solid var(--color-border-default); }
.pp-logo {
  display: inline-flex; align-items: center; justify-content: center; width: 18px; height: 18px; border-radius: 5px;
  background: var(--pv-accent); color: var(--pv-on-accent); font-size: 11px;
}
.pp-avatar {
  display: inline-flex; align-items: center; justify-content: center; width: 20px; height: 20px; border-radius: 50%;
  background: var(--color-fill); font-size: 10px; font-weight: 600; color: var(--color-text-secondary);
}
.pp-alert { position: relative; padding: 6px 8px 6px 10px; border-radius: 6px; font-size: 11px; isolation: isolate; border-left: 2px solid currentColor; }
.pp-alert::before { content: ''; position: absolute; inset: 0; border-radius: inherit; background: currentColor; opacity: .12; z-index: -1; }
.pp-html { max-height: 54px; overflow: hidden; }
.pp-html :deep(p) { margin: 0; }
.pp-welcome {
  position: relative; display: grid; gap: 2px; padding: 10px 12px; border-radius: 8px; color: var(--pv-on-accent);
  background: var(--pv-accent);
}
.pp-btn {
  position: absolute; right: 10px; top: 50%; transform: translateY(-50%); padding: 2px 8px; border-radius: 5px; font-size: 10px;
  background: var(--color-primary); color: var(--color-text-primary);
}
.pp-link {
  display: inline-flex; align-items: center; gap: 4px; max-width: 120px; height: 22px; padding: 0 8px; border-radius: 6px;
  background: var(--color-primary); border: 1px solid var(--color-border-default); font-size: 11px;
}
.pp-accent { color: var(--pv-accent); }
.pp-card { padding: 8px; border-radius: 7px; background: var(--color-primary); border: 1px solid var(--color-border-default); min-width: 0; }
.pp-bar { height: 5px; border-radius: 3px; background: var(--color-fill); }
.pp-chip { padding: 0 6px; height: 16px; line-height: 16px; border-radius: 4px; font-size: 10px; background: var(--color-fill); color: var(--color-text-secondary); }
.pp-chip--on { background: var(--pv-accent); color: var(--pv-on-accent); }
.pp-xs { font-size: 11px; line-height: 1.5; }
</style>
