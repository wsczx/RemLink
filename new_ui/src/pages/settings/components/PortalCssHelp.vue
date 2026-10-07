<script setup lang="ts">
// 门户自定义 CSS 参考：可用变量、核心选择器与示例
import { NCollapse, NCollapseItem } from 'naive-ui'

const groups = [
  { title: '主题色', items: ['--color-primary', '--color-primary-bg', '--color-primary-light', '--color-primary-dark', '--color-success', '--color-warning', '--color-danger', '--color-info'] },
  { title: '背景', items: ['--bg-page', '--bg-card', '--bg-hover', '--bg-header', '--bg-stripe', '--bg-overlay', '--header-bg', '--footer-bg'] },
  { title: '文字', items: ['--text-primary', '--text-regular', '--text-secondary', '--text-placeholder', '--text-inverse'] },
  { title: '边框 / 卡片', items: ['--border-base', '--border-color', '--border-color-light', '--card-radius', '--card-shadow'] },
  {
    title: '门户核心选择器',
    items: ['.portal-page', '.portal-dash-header', '.portal-dash-main', '.welcome-banner', '.stats-row / .stat-card / .stat-icon', '.quicklinks-card / .quicklink-item',
      '.devices-card / .device-item', '.table-card / .card-title / .card-header', '.client-step / .step-num', '.group-item / .cert-item',
      '.dash-grid / .dash-left / .dash-right', '.portal-announcement', '.portal-footer', '.portal-login-card'],
  },
]
const examples = [
  { label: '配色示例', code: '.portal-dash-main { --bg-card: #f7faff; }' },
  { label: '布局示例（主区两列，记得加 !important）', code: '.dash-grid { display: grid !important; grid-template-columns: 1fr 360px !important; }' },
  { label: '悬浮窗示例（把快捷链接钉在左侧）', code: '.quicklinks-card { position: fixed !important; left: 20px; top: 90px; width: 220px; z-index: 50; }' },
  { label: '隐藏页脚', code: '.portal-footer { display: none !important; }' },
]
</script>

<template>
  <NCollapse class="mt-8px">
    <NCollapseItem title="可用 CSS 变量与选择器参考" name="help">
      <div class="grid gap-12px text-md">
        <p class="m-0 text-secondary">
          自定义 CSS 注入到门户页面 &lt;head&gt;（全局作用域），可改配色，也能改写布局。<b class="text-primary">仅登录后的门户首页生效，登录页不注入。</b>
        </p>
        <p class="m-0 rounded-8px bg-fill px-12px py-8px text-warning">
          门户组件样式带 scoped 属性选择器，特异性高于自定义 CSS。要覆盖组件原有样式的规则<b>必须加 !important</b>，否则只有配色等少数规则生效。
        </p>
        <div class="grid grid-cols-1 gap-12px sm:grid-cols-2">
          <div v-for="g in groups" :key="g.title" :class="g.items.length > 8 && 'sm:col-span-2'">
            <div class="mb-6px text-sm font-semibold text-secondary">{{ g.title }}</div>
            <div class="flex flex-wrap gap-6px">
              <span v-for="c in g.items" :key="c" class="rl-kbd">{{ c }}</span>
            </div>
          </div>
        </div>
        <div v-for="e in examples" :key="e.code">
          <div class="mb-4px text-sm text-secondary">{{ e.label }}</div>
          <code class="rl-mono block break-all rounded-6px bg-fill px-10px py-6px text-sm!">{{ e.code }}</code>
        </div>
      </div>
    </NCollapseItem>
  </NCollapse>
</template>
