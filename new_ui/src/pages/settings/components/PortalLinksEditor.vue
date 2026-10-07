<script setup lang="ts">
// 快捷链接编辑：每行「图标预览 / 标题 / 地址 / 图标」，支持上移下移与删除
import { NButton, NInput } from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import ArrowUp from '~icons/fluent/arrow-up-20-regular'
import ArrowDown from '~icons/fluent/arrow-down-20-regular'
import LinkIcon from '~icons/fluent/link-20-regular'
import { linkIconKind, type QuickLink } from './PortalDefs'

const links = defineModel<QuickLink[]>({ required: true })

function move(i: number, d: number) {
  const j = i + d
  if (j < 0 || j >= links.value.length) return
  const arr = links.value
  ;[arr[i], arr[j]] = [arr[j], arr[i]]
}
</script>

<template>
  <div>
    <div v-if="!links.length" class="rounded-10px bg-secondary px-16px py-20px text-center text-md text-tertiary">
      还没有快捷链接，添加内部 Wiki、工单系统、OA 等常用入口。
    </div>
    <div v-for="(link, idx) in links" :key="idx" class="pl-row">
      <div class="pl-icon">
        <img v-if="linkIconKind(link.icon) === 'image'" :src="link.icon" alt="" class="h-18px w-18px object-contain">
        <span v-else-if="linkIconKind(link.icon) === 'emoji'" class="text-lg leading-none">{{ link.icon }}</span>
        <LinkIcon v-else />
      </div>
      <div class="pl-fields">
        <NInput v-model:value="link.label" placeholder="名称，如 内部 Wiki" />
        <NInput v-model:value="link.url" placeholder="链接地址，如 wiki.example.com" class="rl-mono" />
        <NInput v-model:value="link.icon" placeholder="图标：emoji 或图片地址" />
      </div>
      <div class="flex flex-shrink-0 items-center">
        <button class="rl-icon-btn" title="上移" :disabled="idx === 0" @click="move(idx, -1)"><ArrowUp /></button>
        <button class="rl-icon-btn" title="下移" :disabled="idx === links.length - 1" @click="move(idx, 1)"><ArrowDown /></button>
        <button class="rl-icon-btn hover:text-error!" title="删除" @click="links.splice(idx, 1)"><Delete /></button>
      </div>
    </div>
    <div class="mt-12px flex flex-wrap items-center gap-10px">
      <NButton dashed @click="links.push({ label: '', url: '', icon: '' })"><template #icon><Add /></template>添加链接</NButton>
      <span class="text-sm text-tertiary">图标可填 emoji 或图片地址（http(s):// / data:image/）；旧版 el-icon 类名在新门户中显示为默认链接图标。</span>
    </div>
  </div>
</template>

<style scoped>
.pl-row { display: flex; align-items: center; gap: 10px; padding: 10px 0; }
.pl-row + .pl-row { border-top: 1px dashed var(--color-border-default); }
.pl-icon {
  display: flex; align-items: center; justify-content: center; flex-shrink: 0; width: 34px; height: 34px;
  border-radius: 9px; background: var(--color-fill); color: var(--color-text-secondary); font-size: 16px;
}
.pl-fields { display: grid; flex: 1; min-width: 0; gap: 8px; grid-template-columns: minmax(0, 1fr); }
@media (min-width: 900px) { .pl-fields { grid-template-columns: minmax(0, 1fr) minmax(0, 1.6fr) minmax(0, .9fr); } }
.rl-icon-btn:disabled { opacity: .35; cursor: not-allowed; }
</style>
