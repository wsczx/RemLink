<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import * as echarts from 'echarts/core'
import { SankeyChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { cssVar } from '@/theme/theme'
import { useThemeStore } from '@/stores/theme'

echarts.use([SankeyChart, TooltipComponent, CanvasRenderer])

export interface SankeyNode { name: string; label?: string; depth: number; color: string }
export interface SankeyLink { source: string; target: string; value: number }

const props = defineProps<{ nodes: SankeyNode[]; links: SankeyLink[]; unit?: string }>()
const emit = defineEmits<{ select: [name: string] }>()

const el = ref<HTMLDivElement>()
const chart = shallowRef<echarts.ECharts>()
const theme = useThemeStore()

function render() {
  if (!chart.value) return
  const text = cssVar('--color-text-primary')
  const sub = cssVar('--color-text-secondary')
  const labelOf = new Map(props.nodes.map(n => [n.name, n.label ?? n.name]))
  chart.value.setOption({
    animation: false,
    tooltip: {
      trigger: 'item',
      backgroundColor: cssVar('--color-secondary'), borderColor: 'transparent',
      textStyle: { color: sub, fontSize: 12 },
      formatter: (p: any) => p.dataType === 'edge'
        ? `${labelOf.get(p.data.source)} → ${labelOf.get(p.data.target)}<br/><b style="color:${text}">${p.data.value} ${props.unit ?? ''}</b>`
        : `${labelOf.get(p.name)}<br/><b style="color:${text}">${p.value} ${props.unit ?? ''}</b>`,
    },
    series: [{
      type: 'sankey',
      left: 8, right: 140, top: 10, bottom: 10,
      nodeWidth: 10, nodeGap: 14, nodeAlign: 'justify', draggable: false,
      layoutIterations: 64,
      emphasis: { focus: 'adjacency' },
      label: {
        color: text, fontSize: 12, fontFamily: 'v-sans, system-ui, sans-serif',
        formatter: (p: any) => `${labelOf.get(p.name)}  {v|${p.value}}`,
        rich: { v: { color: sub, fontSize: 12 } },
      },
      itemStyle: { borderWidth: 0, borderRadius: 3 },
      lineStyle: { color: 'source', opacity: 0.18, curveness: 0.5 },
      data: props.nodes.map(n => ({ name: n.name, depth: n.depth, itemStyle: { color: n.color } })),
      links: props.links,
    }],
  }, { notMerge: true })
}

onMounted(() => {
  chart.value = echarts.init(el.value!)
  chart.value.on('click', (p: any) => { if (p.dataType === 'node') emit('select', p.name) })
  render()
})
onBeforeUnmount(() => chart.value?.dispose())
useResizeObserver(el, () => chart.value?.resize())
watch(() => [props.nodes, props.links, theme.isDark], render, { deep: true, flush: 'post' })
</script>

<template>
  <div ref="el" class="h-full w-full" />
</template>
