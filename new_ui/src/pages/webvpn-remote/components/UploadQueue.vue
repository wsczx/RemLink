<script setup lang="ts">
// 上传队列浮层：总体进度 + 逐个任务（进度、速度、失败重试 / 覆盖、取消）
import { computed, ref } from 'vue'
import { NButton, NProgress } from 'naive-ui'
import ChevronDown from '~icons/fluent/chevron-down-20-regular'
import ChevronUp from '~icons/fluent/chevron-up-20-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import ArrowRepeat from '~icons/fluent/arrow-repeat-all-20-regular'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-regular'
import ErrorCircle from '~icons/fluent/error-circle-20-regular'
import { formatBytes } from '@/utils/format'
import type { UploadTask, useUploader } from '../upload'

const props = defineProps<{ up: ReturnType<typeof useUploader> }>()
const collapsed = ref(false)

const s = computed(() => props.up.summary.value)
const pct = computed(() => (s.value.total ? Math.floor((s.value.loaded / s.value.total) * 100) : 100))
const title = computed(() => {
  if (s.value.running) return `正在上传 ${s.value.running} 个文件`
  if (s.value.failed) return `${s.value.failed} 个文件上传失败`
  return `上传完成 ${s.value.done} 个文件`
})

function taskPct(t: UploadTask) {
  if (t.status === 'done') return 100
  return t.file.size ? Math.floor((t.loaded / t.file.size) * 100) : 0
}
const speed = (n: number) => (n > 0 ? formatBytes(n) + '/s' : '')
// 最近任务在前
const list = computed(() => [...props.up.tasks].reverse())
</script>

<template>
  <div v-if="up.tasks.length" class="uq">
    <div class="uq__head">
      <div class="min-w-0 flex-1">
        <div class="truncate text-md font-semibold">{{ title }}</div>
        <div class="text-sm text-secondary">
          {{ formatBytes(s.loaded) }} / {{ formatBytes(s.total) }}<template v-if="s.speed"> · {{ speed(s.speed) }}</template>
        </div>
      </div>
      <NButton v-if="s.failed" text size="small" @click="up.retryFailed()">全部重试</NButton>
      <NButton v-if="s.running" text size="small" @click="up.cancelAll()">全部取消</NButton>
      <button class="rl-icon-btn" :title="collapsed ? '展开' : '收起'" @click="collapsed = !collapsed">
        <ChevronUp v-if="collapsed" /><ChevronDown v-else />
      </button>
      <button v-if="!s.running" class="rl-icon-btn" title="清除已完成" @click="up.clearFinished()"><Dismiss /></button>
    </div>
    <NProgress type="line" :percentage="pct" :show-indicator="false" :height="2" :border-radius="0" :status="s.failed ? 'error' : 'default'" />
    <div v-if="!collapsed" class="uq__list">
      <div v-for="t in list" :key="t.id" class="uq__item">
        <div class="flex items-center gap-8px">
          <div class="min-w-0 flex-1">
            <div class="truncate text-md" :title="t.relpath">{{ t.relpath }}</div>
            <div class="truncate text-sm" :class="t.status === 'error' ? 'text-error' : 'text-secondary'">
              <template v-if="t.status === 'error'">{{ t.error }}</template>
              <template v-else-if="t.status === 'done'">{{ formatBytes(t.file.size) }} · 已完成</template>
              <template v-else-if="t.status === 'canceled'">已取消</template>
              <template v-else-if="t.status === 'pending'">等待中 · {{ formatBytes(t.file.size) }}</template>
              <template v-else>
                {{ formatBytes(t.loaded) }} / {{ formatBytes(t.file.size) }}<template v-if="t.speed"> · {{ speed(t.speed) }}</template><template v-if="t.resumed"> · 续传</template>
              </template>
            </div>
          </div>
          <CheckmarkCircle v-if="t.status === 'done'" class="text-success" />
          <template v-else-if="t.status === 'error' || t.status === 'canceled'">
            <NButton v-if="t.errorCode === 'exists'" text size="small" @click="up.retry(t, true)">覆盖</NButton>
            <button class="rl-icon-btn" title="重试" @click="up.retry(t, t.overwrite)"><ArrowRepeat /></button>
            <ErrorCircle v-if="t.status === 'error'" class="text-error" />
          </template>
          <button v-else class="rl-icon-btn" title="取消" @click="up.cancel(t)"><Dismiss /></button>
        </div>
        <NProgress
          v-if="t.status === 'uploading'" class="mt-6px" type="line" :percentage="taskPct(t)" :show-indicator="false" :height="3"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.uq {
  position: absolute; right: 16px; bottom: 16px; z-index: 5; width: 360px; max-width: calc(100% - 32px);
  border: 1px solid var(--color-border-default); border-radius: 12px; background: var(--color-primary);
  box-shadow: 0 8px 28px var(--color-overlay); overflow: hidden;
}
.uq__head { display: flex; align-items: center; gap: 6px; padding: 10px 8px 10px 16px; }
.uq__list { max-height: 280px; overflow-y: auto; }
.uq__item { padding: 10px 8px 10px 16px; border-top: 1px solid var(--color-border-default); }
</style>
