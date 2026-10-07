<script setup lang="ts">
// 服务器证书卡：用途 + 证书摘要（SAN chips / 颁发者 / 有效期进度 / 链长度）；无记录时显示未知态
import { computed } from 'vue'
import { NButton } from 'naive-ui'
import ArrowSync from '~icons/fluent/arrow-sync-20-regular'
import StatusDot from '@/components/StatusDot.vue'
import { formatDate } from '@/utils/format'
import { type CertInfo, daysLeft, expiryText, expiryTone } from './pem'

const props = defineProps<{
  label: string
  purpose: string
  target?: string
  info?: CertInfo | null
  source?: string // 信息来源说明
  disabledReason?: string
}>()
defineEmits<{ replace: [] }>()

const days = computed(() => daysLeft(props.info?.notAfter))
const tone = computed(() => expiryTone(days.value))
const issuer = computed(() => [props.info?.issuerCn, props.info?.issuerOrg].filter(Boolean).join(' · ') || '—')
// 进度条颜色（静态类名，便于 Uno 提取）
const barClass = computed(() => ({ success: 'text-success', warning: 'text-warning', error: 'text-error', info: 'text-info', muted: 'text-tertiary' })[tone.value])
// 已用有效期占比
const elapsed = computed(() => {
  const nb = props.info?.notBefore?.getTime()
  const na = props.info?.notAfter?.getTime()
  if (!nb || !na || na <= nb) return 0
  return Math.min(100, Math.max(0, ((Date.now() - nb) / (na - nb)) * 100))
})
const totalDays = computed(() => {
  const nb = props.info?.notBefore?.getTime()
  const na = props.info?.notAfter?.getTime()
  return nb && na ? Math.round((na - nb) / 86400000) : null
})
</script>

<template>
  <section class="rl-panel flex flex-col">
    <!-- 头部：用途 -->
    <div class="flex items-start gap-12px px-20px pb-16px pt-18px">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-8px">
          <span class="text-lg font-semibold">{{ label }}</span>
          <StatusDot v-if="info" :tone="tone" class="text-sm">{{ expiryText(days) }}</StatusDot>
          <StatusDot v-else tone="muted" class="text-sm">状态未知</StatusDot>
        </div>
        <div class="mt-2px text-sm text-secondary">{{ purpose }}</div>
      </div>
      <NButton size="small" :disabled="!!disabledReason" @click="$emit('replace')">
        <template #icon><ArrowSync /></template>替换
      </NButton>
    </div>

    <template v-if="info">
      <!-- 主域名 + 有效期进度 -->
      <div class="px-20px">
        <div class="rl-mono truncate text-xl font-semibold leading-28px" :title="info.cn">{{ info.cn || info.sans[0] || '（无通用名）' }}</div>
        <div class="mt-12px h-6px overflow-hidden rounded-3px bg-fill">
          <div class="h-full rounded-3px bg-current" :class="barClass" :style="{ width: `${elapsed}%` }" />
        </div>
        <div class="mt-6px flex justify-between text-sm text-secondary">
          <span>{{ formatDate(info.notBefore, 'YYYY-MM-DD') || '—' }}</span>
          <span v-if="totalDays">有效期 {{ totalDays }} 天</span>
          <span>{{ formatDate(info.notAfter, 'YYYY-MM-DD') || '—' }}</span>
        </div>
      </div>

      <dl class="m-0 mt-16px grid grid-cols-[64px_minmax(0,1fr)] gap-x-12px gap-y-10px b-t-1px b-t-solid b-[var(--color-border-default)] px-20px py-14px text-md">
        <dt class="text-secondary">覆盖域名</dt>
        <dd class="m-0 flex flex-wrap gap-4px">
          <span v-if="!info.sans.length" class="text-tertiary">未包含 SAN</span>
          <span v-for="s in info.sans" :key="s" class="rl-kbd" :class="s === target && 'text-success!'">{{ s }}</span>
        </dd>
        <dt class="text-secondary">颁发者</dt>
        <dd class="m-0 min-w-0 break-words">
          {{ issuer }}
          <span v-if="info.selfSigned" class="ax-badge ml-4px text-warning">自签名</span>
        </dd>
        <dt class="text-secondary">证书链</dt>
        <dd class="m-0">
          {{ info.chainLen }} 张
          <span v-if="info.chainLen < 2 && !info.selfSigned" class="text-sm text-warning">· 未包含中间证书，部分客户端可能不信任</span>
        </dd>
      </dl>
    </template>

    <!-- 未知态：后端未提供读取接口 -->
    <div v-else class="mx-20px mb-4px flex flex-1 flex-col justify-center rounded-10px b-1px b-[var(--color-border-default)] b-dashed px-16px py-24px text-center">
      <div class="text-md font-medium">{{ disabledReason || '尚无此证书的本地解析记录' }}</div>
      <div v-if="!disabledReason" class="mt-4px text-sm text-secondary">替换证书时会在浏览器中解析并记录域名、颁发者与到期时间</div>
    </div>

    <div class="mt-auto flex items-center justify-between gap-12px px-20px py-10px text-sm text-tertiary">
      <span class="truncate">{{ source || '' }}</span>
      <span v-if="target" class="rl-mono flex-shrink-0">{{ target }}</span>
    </div>
  </section>
</template>
