<script setup lang="ts">
// 事件量迷你柱状图：调用后端按天统计接口一次取回，点击柱子把时间范围切到当天
import { computed, ref, watch } from 'vue'
import { NSpin, NTooltip } from 'naive-ui'
import dayjs from 'dayjs'

const props = withDefaults(defineProps<{
  // 返回截至 end（YYYY-MM-DD）的 days 天内，符合当前筛选的每日事件数
  fetcher: (end: string, days: number) => Promise<{ date: string; count: number }[]>
  // 依赖变化（筛选条件）时重新统计
  depKey: string
  end?: string
  days?: number
  selected?: [string, string] | null
}>(), { days: 14 })
const emit = defineEmits<{ pick: [day: string] }>()

const bars = ref<{ day: string; n: number }[]>([])
const loading = ref(false)
const failed = ref(false)
let token = 0

async function load() {
  const my = ++token
  const end = props.end ? dayjs(props.end) : dayjs()
  const days = Array.from({ length: props.days }, (_, i) => end.subtract(props.days - 1 - i, 'day').format('YYYY-MM-DD'))
  loading.value = true
  try {
    const rows = await props.fetcher(end.format('YYYY-MM-DD'), props.days)
    if (my !== token) return
    const byDay = new Map((rows || []).map(r => [r.date, r.count]))
    bars.value = days.map(day => ({ day, n: byDay.get(day) || 0 }))
    failed.value = false
  } catch {
    if (my === token) failed.value = true
  } finally {
    if (my === token) loading.value = false
  }
}
watch(() => [props.depKey, props.end, props.days], load, { immediate: true })

const max = computed(() => Math.max(1, ...bars.value.map(b => b.n)))
const total = computed(() => bars.value.reduce((s, b) => s + b.n, 0))
const peak = computed(() => bars.value.reduce<{ day: string; n: number } | null>((p, b) => (!p || b.n > p.n ? b : p), null))
const inSel = (day: string) => {
  const s = props.selected
  if (!s) return true
  return day >= s[0].slice(0, 10) && day <= s[1].slice(0, 10)
}
const today = dayjs().format('YYYY-MM-DD')
const lastDay = computed(() => bars.value[bars.value.length - 1]?.day)
const lastLabel = computed(() => (lastDay.value === today ? '今天' : lastDay.value ? dayjs(lastDay.value).format('MM-DD') : ''))
const WEEK = '日一二三四五六'
defineExpose({ reload: load })
</script>

<template>
  <div v-if="!failed" class="rl-ev">
    <div class="flex flex-col justify-between gap-6px">
      <div class="text-sm text-secondary">{{ lastLabel === '今天' || !lastLabel ? `近 ${days} 天` : `截至 ${lastLabel} 的 ${days} 天` }}事件量</div>
      <div class="flex items-baseline gap-6px">
        <span class="text-4xl font-semibold tabular-nums">{{ total.toLocaleString() }}</span>
        <span class="text-sm text-tertiary">条</span>
        <NSpin v-if="loading" :size="12" class="ml-4px" />
      </div>
      <div class="text-sm text-tertiary">
        <template v-if="peak && peak.n">峰值 {{ dayjs(peak.day).format('MM-DD') }} · {{ peak.n.toLocaleString() }}</template>
        <template v-else>暂无事件</template>
      </div>
    </div>
    <div class="min-w-0 flex-1">
      <div class="rl-ev__bars">
        <NTooltip v-for="b in bars" :key="b.day" :show-arrow="false" placement="top">
          <template #trigger>
            <button class="rl-ev__col" :class="!inSel(b.day) && 'rl-ev__col--dim'" @click="emit('pick', b.day)">
              <span class="rl-ev__bar" :class="!b.n && 'rl-ev__bar--zero'" :style="{ height: b.n ? `${Math.max(6, (b.n / max) * 100)}%` : '2px' }" />
            </button>
          </template>
          <div class="text-sm">{{ dayjs(b.day).format('MM-DD') }} 周{{ WEEK[dayjs(b.day).day()] }}{{ b.day === today ? '（今天）' : '' }}</div>
          <div class="font-semibold">{{ b.n.toLocaleString() }} 条</div>
          <div class="text-sm opacity-70">点击查看当天</div>
        </NTooltip>
      </div>
      <div class="mt-4px flex justify-between text-sm text-tertiary tabular-nums">
        <span>{{ bars[0] && dayjs(bars[0].day).format('MM-DD') }}</span>
        <span>{{ lastLabel }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rl-ev { display: flex; gap: 32px; padding: 16px 20px; border-bottom: 1px solid var(--color-border-default); }
.rl-ev__bars { display: flex; align-items: flex-end; gap: 4px; height: 64px; }
.rl-ev__col {
  flex: 1; display: flex; align-items: flex-end; height: 100%; padding: 0; border: 0; border-radius: 3px;
  background: none; cursor: pointer;
}
.rl-ev__col:hover { background: var(--color-action-hover); }
.rl-ev__bar { width: 100%; border-radius: 3px 3px 1px 1px; background: var(--color-text-info); transition: height .25s, opacity .15s; }
.rl-ev__bar--zero { background: var(--color-border-default); }
.rl-ev__col--dim .rl-ev__bar { opacity: .25; }
.rl-ev__col:hover .rl-ev__bar { opacity: .85; }
@media (max-width: 640px) { .rl-ev { flex-direction: column; gap: 12px; } }
</style>
