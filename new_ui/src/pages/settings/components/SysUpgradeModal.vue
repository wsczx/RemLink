<script setup lang="ts">
// 软件在线升级：版本对比 + 更新日志 → SSE 进度 → 完成后轮询服务恢复并刷新
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NModal, NProgress } from 'naive-ui'
import ArrowRight from '~icons/fluent/arrow-right-20-regular'
import CheckmarkCircle from '~icons/fluent/checkmark-circle-20-regular'
import ErrorCircle from '~icons/fluent/error-circle-20-regular'
import { http } from '@/api/http'
import { ui } from '@/utils/ui'

const props = defineProps<{ info: any; currentVersion?: string }>()
const show = defineModel<boolean>('show', { required: true })

type Step = 'info' | 'progress' | 'done'
const step = ref<Step>('info')
const running = ref(false)
const progress = ref(0)
const stage = ref('')
const error = ref('')
let timer: ReturnType<typeof setTimeout> | null = null
let disposed = false

const stageText = computed(() => ({
  downloading: '正在下载…',
  replacing: '正在替换文件…',
  restarting: '正在重启服务…',
  done: '升级完成',
} as Record<string, string>)[stage.value] || stage.value)

// 每次打开重置到版本信息页
watch(show, v => {
  if (v) {
    step.value = 'info'
    running.value = false
    progress.value = 0
    stage.value = ''
    error.value = ''
  }
})

async function start() {
  step.value = 'progress'
  running.value = true
  progress.value = 0
  error.value = ''
  try {
    // fetch + 手动解析 SSE（HttpOnly Cookie 自动携带 JWT）
    const resp = await fetch('/api/v1/set/upgrade/start', { method: 'POST', credentials: 'include' })
    if (!resp.ok || !resp.body) {
      const text = await resp.text().catch(() => '')
      let msg = '升级请求失败'
      try { msg = JSON.parse(text).msg || msg } catch { /* 响应体可能为空 */ }
      throw new Error(msg)
    }
    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        try {
          const d = JSON.parse(line.slice(6))
          progress.value = d.progress
          stage.value = d.stage
          if (d.stage === 'error') {
            error.value = d.error
            running.value = false
          }
          if (d.stage === 'done') {
            step.value = 'done'
            running.value = false
            reconnect()
          }
        } catch { /* 忽略单行解析错误 */ }
      }
    }
  } catch (e: any) {
    // 重启时连接被断开属正常，已进入完成态则忽略
    if (step.value !== 'done') {
      error.value = e?.message || '升级连接失败'
      running.value = false
    }
  }
}

// 服务重启后轮询，恢复即刷新页面
function reconnect() {
  let attempts = 0
  const tick = () => {
    timer = setTimeout(async () => {
      if (disposed) return
      attempts++
      try {
        await http.get('/api/v1/set/soft/status', { timeout: 3000 })
        show.value = false
        ui.message.success('升级完成，服务已重启')
        setTimeout(() => location.reload(), 1000)
      } catch {
        if (attempts < 20) tick()
        else {
          show.value = false
          ui.message.warning('服务恢复超时，请手动刷新页面')
        }
      }
    }, 2000)
  }
  tick()
}

onBeforeUnmount(() => {
  disposed = true
  if (timer) clearTimeout(timer)
})
</script>

<template>
  <NModal
    v-model:show="show" preset="card" title="软件升级" class="w-520px! max-w-[calc(100vw-32px)]"
    :mask-closable="false" :closable="!running" :close-on-esc="!running"
  >
    <template v-if="step === 'info'">
      <div class="flex items-center justify-center gap-24px rounded-10px bg-secondary py-20px">
        <div class="text-center">
          <div class="text-sm text-secondary">当前版本</div>
          <div class="rl-mono mt-6px text-xl! font-semibold">{{ currentVersion || '-' }}</div>
        </div>
        <ArrowRight class="text-xl text-tertiary" />
        <div class="text-center">
          <div class="text-sm text-secondary">最新版本</div>
          <div class="rl-mono mt-6px text-xl! font-semibold text-info">{{ info?.latest?.version || '-' }}</div>
        </div>
      </div>
      <div v-if="info?.upgrade_source" class="mt-10px text-sm text-tertiary">
        更新源：{{ info.upgrade_source === 'github' ? 'GitHub' : 'Gitee' }}
      </div>
      <div v-if="info?.latest?.body" class="mt-16px b-t-1px b-t-solid b-[var(--color-border-default)] pt-16px">
        <div class="mb-8px text-md font-semibold">更新日志</div>
        <div class="max-h-220px overflow-y-auto whitespace-pre-wrap text-md leading-22px text-secondary">{{ info.latest.body }}</div>
      </div>
    </template>

    <template v-else-if="step === 'progress'">
      <div class="flex flex-col items-center py-20px">
        <NProgress
          type="circle" :percentage="progress || 0" :stroke-width="8" class="w-140px!"
          :color="error ? 'var(--color-text-error)' : 'var(--color-action)'" rail-color="var(--color-fill)"
        />
        <div class="mt-16px text-md text-secondary">{{ stageText || '准备中…' }}</div>
      </div>
      <div v-if="error" class="flex items-start gap-8px rounded-8px bg-fill px-12px py-10px text-md text-error">
        <ErrorCircle class="mt-2px flex-shrink-0" /> <span>{{ error }}</span>
      </div>
    </template>

    <template v-else>
      <div class="flex flex-col items-center py-32px text-center">
        <CheckmarkCircle class="text-[48px] text-success" />
        <div class="mt-12px text-lg font-semibold">升级成功，服务正在重启…</div>
        <div class="mt-4px text-md text-secondary">重启完成后将自动刷新页面</div>
      </div>
    </template>

    <template #footer>
      <div class="flex justify-end gap-sm">
        <template v-if="step === 'info'">
          <NButton @click="show = false">取消</NButton>
          <NButton type="primary" @click="start">立即升级</NButton>
        </template>
        <NButton v-else-if="step === 'progress' && error" @click="show = false">关闭</NButton>
      </div>
    </template>
  </NModal>
</template>
