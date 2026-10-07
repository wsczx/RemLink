<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { chartColors, cssVar } from '@/theme/theme'
import { useThemeStore } from '@/stores/theme'

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

export interface LineSeries {
  name: string
  // [时间戳(ms) | 日期字符串, 数值]
  data: [number | string, number][]
  color?: string
  areaColor?: boolean
  bold?: boolean
}

const props = withDefaults(defineProps<{
  series: LineSeries[]
  showXAxis?: boolean
  showYAxis?: boolean
  grid?: Record<string, any>
  unit?: string
  // 迷你图不显示 tooltip
  tooltip?: boolean
}>(), { showXAxis: true, showYAxis: true, tooltip: true, unit: '' })

const el = ref<HTMLDivElement>()
const chart = shallowRef<echarts.ECharts>()
const theme = useThemeStore()

function render() {
  if (!chart.value) return
  const palette = chartColors[theme.isDark ? 'dark' : 'light'].palette
  const mini = !props.showXAxis && !props.showYAxis
  chart.value.setOption({
    animation: false,
    grid: props.grid ?? (mini ? { left: 0, right: '6%', top: '10%', bottom: '10%' } : { left: 10, right: 20, top: 20, bottom: 10, containLabel: true }),
    tooltip: props.tooltip ? {
      trigger: 'axis',
      backgroundColor: cssVar('--color-secondary'),
      borderColor: 'transparent',
      textStyle: { color: cssVar('--color-text-secondary'), fontSize: 12 },
      valueFormatter: (v: number) => `${v}${props.unit ? ' ' + props.unit : ''}`,
    } : { show: false },
    xAxis: {
      type: 'time', boundaryGap: false, splitNumber: 5, show: props.showXAxis,
      axisLine: { lineStyle: { color: cssVar('--color-border-strong') } },
      splitLine: { show: false },
      axisLabel: { color: cssVar('--color-text-secondary'), padding: [4, 0, 0, 0], hideOverlap: true },
    },
    yAxis: {
      type: 'value', show: props.showYAxis, minInterval: 1,
      axisLabel: { color: cssVar('--color-text-secondary') },
      splitLine: { lineStyle: { color: cssVar('--color-border-default') } },
    },
    series: props.series.slice(0, 10).map((s, i) => {
      const color = s.color ?? palette[i]
      return {
        type: 'line', name: s.name, showSymbol: false, itemStyle: { color },
        lineStyle: { width: s.bold ? 2 : 1.2 },
        areaStyle: s.areaColor ? { color: `${color}35` } : undefined,
        data: s.data,
      }
    }),
  }, { notMerge: true })
}

onMounted(() => {
  chart.value = echarts.init(el.value!)
  render()
})
onBeforeUnmount(() => chart.value?.dispose())
useResizeObserver(el, () => chart.value?.resize())
watch(() => [props.series, theme.isDark], render, { deep: true, flush: 'post' })
</script>

<template>
  <div ref="el" class="h-full w-full" />
</template>
