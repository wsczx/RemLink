<script setup lang="ts">
// 品牌图片字段：拖入或点击上传（转 data URI，原图 ≤1MB）、也可粘贴 URL / SVG 源码；支持复制与移除
import { ref } from 'vue'
import { NButton, NInput } from 'naive-ui'
import ArrowUpload from '~icons/fluent/arrow-upload-20-regular'
import Copy from '~icons/fluent/copy-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import ImageIcon from '~icons/fluent/image-20-regular'
import Link from '~icons/fluent/link-20-regular'
import { normalizeImageSrc } from '@/utils/brand'
import { ui } from '@/utils/ui'

const props = defineProps<{ label: string; hint?: string; size?: number }>()
const model = defineModel<string>({ default: '' })
const fileEl = ref<HTMLInputElement | null>(null)
const over = ref(false)
const showUrl = ref(false)

function readFile(file?: File | null) {
  if (!file) return
  if (!file.type.startsWith('image/')) return ui.message.warning('请选择图片文件')
  // 原图 1MB 上限（base64 后约 1.33MB，低于后端 2MB 限制）
  if (file.size > 1 << 20) {
    return ui.message.warning(`图片过大（${(file.size / 1024 / 1024).toFixed(1)}MB），请压缩到 1MB 以内再上传`)
  }
  const reader = new FileReader()
  reader.onload = () => { model.value = String(reader.result || '') }
  reader.readAsDataURL(file)
}
function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  readFile(input.files?.[0])
  input.value = ''
}
function onDrop(e: DragEvent) {
  over.value = false
  readFile(e.dataTransfer?.files?.[0])
}

async function copy() {
  try {
    await navigator.clipboard.writeText(model.value)
  } catch {
    // 非 https 环境回退
    const ta = document.createElement('textarea')
    ta.value = model.value
    ta.style.cssText = 'position:fixed;opacity:0'
    document.body.appendChild(ta)
    ta.select()
    try { document.execCommand('copy') } catch { /* 忽略 */ }
    ta.remove()
  }
  ui.message.success(`${props.label}地址已复制`)
}
</script>

<template>
  <div>
    <div class="flex items-center gap-14px">
      <div
        class="brand-drop" :class="over && 'brand-drop--over'" :style="{ width: `${(size || 40) + 32}px`, height: `${(size || 40) + 32}px` }"
        title="点击或拖入图片" @click="fileEl?.click()" @dragover.prevent="over = true" @dragleave="over = false" @drop.prevent="onDrop"
      >
        <img v-if="model" :src="normalizeImageSrc(model)" :alt="label" :style="{ maxWidth: `${size || 40}px`, maxHeight: `${size || 40}px` }">
        <ImageIcon v-else class="text-2xl text-quaternary" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="text-sm text-secondary">{{ model ? (model.startsWith('data:') ? '已上传图片' : '外部图片地址') : '使用默认图标' }}</div>
        <div v-if="hint" class="text-sm text-tertiary">{{ hint }}</div>
        <div class="mt-8px flex flex-wrap gap-6px">
          <NButton size="small" @click="fileEl?.click()"><template #icon><ArrowUpload /></template>上传</NButton>
          <NButton size="small" quaternary @click="showUrl = !showUrl"><template #icon><Link /></template>{{ showUrl ? '收起' : '填写地址' }}</NButton>
          <NButton v-if="model" size="small" quaternary @click="copy"><template #icon><Copy /></template>复制</NButton>
          <NButton v-if="model" size="small" quaternary type="error" @click="model = ''"><template #icon><Delete /></template>移除</NButton>
        </div>
      </div>
    </div>
    <NInput
      v-if="showUrl" v-model:value="model" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" class="rl-mono mt-10px"
      placeholder="https://… 、data:image/…;base64,… 或 <svg> 源码"
    />
    <input ref="fileEl" type="file" accept="image/*" class="hidden" @change="onPick">
  </div>
</template>

<style scoped>
.brand-drop {
  display: flex; align-items: center; justify-content: center; flex-shrink: 0; overflow: hidden; cursor: pointer;
  border: 1px dashed var(--color-border-strong); border-radius: 12px; background: var(--color-secondary);
  transition: border-color .15s, background .15s;
}
.brand-drop:hover, .brand-drop--over { border-color: var(--color-action); background: var(--color-action-hover); }
</style>
