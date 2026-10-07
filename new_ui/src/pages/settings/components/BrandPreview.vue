<script setup lang="ts">
// 品牌实时预览：浏览器标签 + 门户登录页、门户顶栏两处迷你示意
import { computed } from 'vue'
import Person from '~icons/fluent/person-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import { normalizeImageSrc } from '@/utils/brand'

const props = defineProps<{
  title?: string
  desc?: string
  logo?: string
  favicon?: string
  footer?: string
  featuresOn: boolean
  features: { label: string; desc: string }[]
}>()

const name = computed(() => props.title || 'RemLink')
const logoSrc = computed(() => normalizeImageSrc(props.logo))
const faviconSrc = computed(() => normalizeImageSrc(props.favicon) || `${import.meta.env.BASE_URL}favicon.svg`)
</script>

<template>
  <div class="grid gap-14px">
    <!-- 浏览器 + 登录页 -->
    <div>
      <div class="mb-6px text-sm text-tertiary">门户登录页</div>
      <div class="bp-browser">
        <div class="bp-tabbar">
          <span class="bp-dot" /><span class="bp-dot" /><span class="bp-dot" />
          <div class="bp-tab">
            <img :src="faviconSrc" alt="" class="h-14px w-14px flex-shrink-0 object-contain">
            <span class="truncate">{{ name }}</span>
            <Dismiss class="ml-auto flex-shrink-0 text-xs text-tertiary" />
          </div>
        </div>
        <div class="bp-page">
          <div class="bp-card">
            <div class="flex items-center gap-8px">
              <img v-if="logoSrc" :src="logoSrc" alt="" class="h-26px w-26px object-contain">
              <div class="min-w-0">
                <div class="truncate text-md font-semibold text-primary">{{ name }}</div>
                <div class="truncate text-xs text-secondary">{{ desc || '企业级安全远程接入网关用户门户' }}</div>
              </div>
            </div>
            <div class="bp-input mt-12px"><Person class="text-tertiary" /><span>用户名</span></div>
            <div class="bp-input mt-6px"><LockClosed class="text-tertiary" /><span>密码</span></div>
            <div class="bp-btn mt-10px">登录</div>
            <ul v-if="featuresOn" class="m-0 mt-12px grid list-none gap-6px p-0">
              <li v-for="(f, i) in features" :key="i" class="min-w-0">
                <div class="truncate text-xs font-semibold text-primary">{{ f.label || '未填写标题' }}</div>
                <div class="truncate text-xs text-secondary">{{ f.desc }}</div>
              </li>
            </ul>
          </div>
          <!-- 管理员配置的页脚 HTML -->
          <div v-if="footer" class="bp-footer" v-html="footer" />
          <div v-else class="bp-footer">Powered by RemLink</div>
        </div>
      </div>
    </div>

    <!-- 门户顶栏 -->
    <div>
      <div class="mb-6px text-sm text-tertiary">门户顶栏</div>
      <div class="bp-topbar">
        <img v-if="logoSrc" :src="logoSrc" alt="" class="h-22px w-22px object-contain">
        <div class="min-w-0">
          <div class="truncate text-sm font-semibold text-primary">{{ name }}</div>
          <div class="truncate text-xs text-secondary">{{ desc || '企业级安全远程接入网关用户门户' }}</div>
        </div>
        <div class="flex-1" />
        <span class="bp-avatar">Z</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bp-browser { overflow: hidden; border: 1px solid var(--color-border-default); border-radius: 10px; background: var(--color-secondary); }
.bp-tabbar { display: flex; align-items: center; gap: 5px; height: 34px; padding: 0 10px; background: var(--color-fill); }
.bp-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--color-border-strong); }
.bp-tab {
  display: flex; align-items: center; gap: 6px; height: 26px; margin: 8px 0 0 10px; padding: 0 10px; width: 180px;
  border-radius: 8px 8px 0 0; background: var(--color-secondary); font-size: 12px; color: var(--color-text-primary);
  align-self: flex-end;
}
.bp-page { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 24px 16px 12px; }
.bp-card {
  width: 100%; max-width: 260px; padding: 16px; border: 1px solid var(--color-border-default); border-radius: 12px;
  background: var(--color-primary);
}
.bp-logo {
  display: inline-flex; align-items: center; justify-content: center; width: 26px; height: 26px; flex-shrink: 0;
  border-radius: 8px; background: var(--color-action); color: var(--color-text-on-action); font-size: 15px;
}
.bp-logo--sm { width: 22px; height: 22px; border-radius: 6px; font-size: 13px; }
.bp-input {
  display: flex; align-items: center; gap: 6px; height: 26px; padding: 0 8px; border: 1px solid var(--color-border-default);
  border-radius: 6px; font-size: 11px; color: var(--color-text-quaternary);
}
.bp-btn {
  display: flex; align-items: center; justify-content: center; height: 26px; border-radius: 6px;
  background: var(--color-action); color: var(--color-text-on-action); font-size: 11px; font-weight: 600;
}
.bp-footer { max-width: 100%; overflow: hidden; font-size: 11px; color: var(--color-text-tertiary); text-align: center; }
.bp-footer :deep(a) { color: inherit; }
.bp-footer :deep(img) { max-height: 18px; }
.bp-topbar {
  display: flex; align-items: center; gap: 8px; height: 48px; padding: 0 12px;
  border: 1px solid var(--color-border-default); border-radius: 10px; background: var(--color-primary);
}
.bp-avatar {
  display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; border-radius: 50%;
  background: var(--color-fill); font-size: 11px; font-weight: 600; color: var(--color-text-secondary);
}
.text-xs { font-size: 11px; line-height: 1.5; }
</style>
