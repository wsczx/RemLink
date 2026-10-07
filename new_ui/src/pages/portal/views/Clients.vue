<script setup lang="ts">
// 客户端下载：按平台卡片选择（自动识别当前设备）+ 分步连接指引 + 管理员配置的下载内容
import { computed, ref, watch, type Component } from 'vue'
import Desktop from '~icons/fluent/desktop-20-regular'
import Laptop from '~icons/fluent/laptop-20-regular'
import WindowConsole from '~icons/fluent/window-console-20-regular'
import Phone from '~icons/fluent/phone-20-regular'
import Tablet from '~icons/fluent/tablet-20-regular'
import AppsIcon from '~icons/fluent/apps-20-regular'
import Copy from '~icons/fluent/copy-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import { usePortal } from '../store'
import { copyText } from '../shared'

const p = usePortal()
const { user, dashboard, clientGuide, showClientGuide } = p

type Os = 'windows' | 'macos' | 'linux' | 'android' | 'ios' | 'harmony' | ''

function osOf(name: string): Os {
  const n = name.toLowerCase()
  if (n.includes('win')) return 'windows'
  if (n.includes('mac') || n.includes('os x')) return 'macos'
  if (n.includes('ios') || n.includes('iphone') || n.includes('ipad')) return 'ios'
  if (n.includes('鸿蒙') || n.includes('harmony')) return 'harmony'
  if (n.includes('android') || n.includes('安卓')) return 'android'
  if (n.includes('linux') || n.includes('ubuntu') || n.includes('centos') || n.includes('debian')) return 'linux'
  return ''
}

// 识别当前访问设备的平台
function detectOs(): Os {
  const ua = navigator.userAgent.toLowerCase()
  if (/harmonyos|openharmony/.test(ua)) return 'harmony'
  if (/android/.test(ua)) return 'android'
  if (/iphone|ipad|ipod/.test(ua) || (/macintosh/.test(ua) && navigator.maxTouchPoints > 1)) return 'ios'
  if (/windows/.test(ua)) return 'windows'
  if (/mac os x|macintosh/.test(ua)) return 'macos'
  if (/linux|x11/.test(ua)) return 'linux'
  return ''
}
const currentOs = detectOs()

const icons: Record<string, Component> = { windows: Desktop, macos: Laptop, linux: WindowConsole, android: Phone, harmony: Phone, ios: Tablet }
const iconOf = (name: string) => icons[osOf(name)] || AppsIcon
const isCurrent = (name: string) => !!currentOs && osOf(name) === currentOs

const active = ref('')
// 默认选中当前平台；无匹配时选第一个，避免内容空白
watch(clientGuide, list => {
  if (!list.length || list.some(g => g.name === active.value)) return
  active.value = (list.find(g => isCurrent(g.name)) || list[0]).name
}, { immediate: true })
const current = computed(() => clientGuide.value.find(g => g.name === active.value))

const fill = (html: string) => String(html).replace(/\{\{server_addr\}\}/g, user.value.server_addr || 'vpn.example.com')
</script>

<template>
  <div>
    <PageHeader title="客户端下载" description="选择你的设备平台，安装客户端后按步骤连接" />

    <div class="grid gap-16px">
      <!-- 服务器地址：连接时唯一需要填写的信息 -->
      <RlPanel :padded="false">
        <div class="flex flex-wrap items-center gap-x-16px gap-y-8px px-20px py-16px">
          <div class="min-w-0 flex-1">
            <div class="text-sm text-secondary">服务器地址（在客户端中填写）</div>
            <div class="rl-mono mt-2px truncate text-lg! font-semibold">{{ user.server_addr || '-' }}</div>
          </div>
          <button v-if="user.server_addr" class="rl-icon-btn" title="复制服务器地址" @click="copyText(user.server_addr)"><Copy /></button>
        </div>
      </RlPanel>

      <template v-if="showClientGuide">
        <!-- 平台卡片 -->
        <div class="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-12px">
          <button
            v-for="g in clientGuide" :key="g.name" type="button" class="platform"
            :class="active === g.name && 'platform--active'" @click="active = g.name"
          >
            <span class="flex w-full items-start justify-between">
              <span class="text-[22px]" :class="active === g.name ? 'text-primary' : 'text-secondary'"><component :is="iconOf(g.name)" /></span>
              <span v-if="isCurrent(g.name)" class="ax-badge text-info">当前设备</span>
            </span>
            <span class="text-md font-semibold">{{ g.name }}</span>
            <span class="text-sm text-tertiary">{{ g.steps.length }} 个步骤</span>
          </button>
        </div>

        <!-- 分步指引 -->
        <RlPanel v-if="current" :title="`在 ${current.name} 上连接`" :subtitle="isCurrent(current.name) ? '已根据你当前的设备自动选择' : undefined">
          <ol class="steps m-0 grid list-none p-0">
            <li v-for="(s, i) in current.steps" :key="i" class="step">
              <span class="step__no">{{ i + 1 }}</span>
              <!-- 管理员配置的 HTML 片段 -->
              <div class="guide-html min-w-0 flex-1 pt-2px text-md leading-relaxed" v-html="fill(s)" />
            </li>
          </ol>
        </RlPanel>
      </template>

      <!-- 管理员配置的下载内容 -->
      <RlPanel v-if="dashboard.client_download_html" title="下载地址" subtitle="由管理员提供的客户端安装包">
        <div class="guide-html text-md" v-html="dashboard.client_download_html" />
      </RlPanel>
    </div>
  </div>
</template>

<style scoped>
.platform {
  display: flex; flex-direction: column; align-items: flex-start; gap: 6px; padding: 14px 16px;
  border: 1px solid var(--color-border-default); border-radius: 12px; background: var(--color-primary);
  color: var(--color-text-primary); font: inherit; text-align: left; cursor: pointer; transition: border-color .15s, box-shadow .15s;
}
.platform:hover { border-color: var(--color-border-strong); }
.platform--active { border-color: var(--color-action); box-shadow: inset 0 0 0 1px var(--color-action); }
.steps { gap: 0; }
.step { position: relative; display: flex; gap: 14px; padding-bottom: 20px; }
.step:last-child { padding-bottom: 0; }
.step:not(:last-child)::before {
  content: ''; position: absolute; left: 11px; top: 28px; bottom: 4px; width: 1px; background: var(--color-border-default);
}
.step__no {
  display: flex; flex-shrink: 0; align-items: center; justify-content: center; width: 24px; height: 24px;
  border-radius: 50%; background: var(--color-action); color: var(--color-body); font-size: 12px; font-weight: 600;
}
.guide-html :deep(code) {
  padding: 1px 6px; border-radius: 4px; background: var(--color-fill);
  font-family: v-mono, ui-monospace, monospace; font-size: 12px; word-break: break-all;
}
.guide-html :deep(a) { color: var(--color-text-info); }
.guide-html :deep(a:hover) { text-decoration: underline; }
.guide-html :deep(img) { max-width: 100%; border-radius: 6px; }
.guide-html :deep(ul), .guide-html :deep(ol) { margin: 0; padding-left: 20px; }
.guide-html :deep(li) { margin-bottom: 2px; }
.guide-html :deep(h3), .guide-html :deep(h4) { margin: 0 0 8px; font-size: 15px; }
.guide-html :deep(table) { border-collapse: collapse; width: 100%; }
.guide-html :deep(td), .guide-html :deep(th) { padding: 4px 8px; }
</style>
