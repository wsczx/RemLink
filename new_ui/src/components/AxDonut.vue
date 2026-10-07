<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import * as echarts from 'echarts/core'
import { PieChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { chartColors, cssVar } from '@/theme/theme'
import { useThemeStore } from '@/stores/theme'
import { formatCount } from '@/utils/format'

echarts.use([PieChart, TooltipComponent, CanvasRenderer])

// 环形图：圆心写总数，右侧自绘图例
const props = withDefaults(defineProps<{
  data: { name: string; value: number; color?: string }[]
  centerLabel: string
  emptyText?: string
}>(), { emptyText: '暂无数据' })

const el = ref<HTMLDivElement>()
const chart = shallowRef<echarts.ECharts>()
const theme = useThemeStore()
const total = computed(() => props.data.reduce((s, d) => s + d.value, 0))
const colors = computed(() => {
  const p = chartColors[theme.isDark ? 'dark' : 'light'].palette
  return props.data.map((d, i) => d.color ?? p[i % p.length])
})

function render() {
  if (!chart.value) return
  const empty = !total.value
  chart.value.setOption({
    animation: false,
    tooltip: { show: !empty, trigger: 'item', backgroundColor: cssVar('--color-secondary'), borderColor: 'transparent', textStyle: { color: cssVar('--color-text-secondary'), fontSize: 12 } },
    series: [{
      type: 'pie', radius: ['76%', '96%'], center: ['50%', '50%'], label: { show: false }, emphasis: { scale: false },
      itemStyle: { borderColor: cssVar('--color-primary'), borderWidth: empty ? 0 : 2 },
      data: empty
        ? [{ name: '', value: 1, itemStyle: { color: cssVar('--color-fill') } }]
        : props.data.map((d, i) => ({ name: d.name, value: d.value, itemStyle: { color: colors.value[i] } })),
    }],
  }, { notMerge: true })
}

onMounted(() => { chart.value = echarts.init(el.value!); render() })
onBeforeUnmount(() => chart.value?.dispose())
useResizeObserver(el, () => chart.value?.resize())
watch(() => [props.data, theme.isDark], render, { deep: true, flush: 'post' })
</script>

<template>
  <div class="flex h-130px w-full items-center gap-md px-md">
    <div class="relative h-124px w-124px flex-shrink-0">
      <div ref="el" class="h-full w-full" />
      <div class="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-md font-bold">
        <div>{{ formatCount(total) }}</div>
        <div>{{ centerLabel }}</div>
      </div>
    </div>
    <div class="ax-scrollbar-none max-h-130px min-w-0 max-w-260px flex-1 overflow-auto">
      <div v-if="!data.length" class="text-sm text-secondary">{{ emptyText }}</div>
      <div v-for="(d, i) in data" :key="d.name" class="flex h-20px items-center gap-8px text-sm">
        <span class="h-8px w-8px flex-shrink-0 rounded-full" :style="{ background: colors[i] }" />
        <span class="flex-1 truncate text-secondary">{{ d.name }}</span>
        <span class="text-secondary">{{ formatCount(d.value) }}</span>
      </div>
    </div>
  </div>
</template>
