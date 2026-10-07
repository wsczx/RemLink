<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { NIcon } from 'naive-ui'
import mapboxgl from 'mapbox-gl'
import 'mapbox-gl/dist/mapbox-gl.css'
import Add from '~icons/fluent/add-20-regular'
import Subtract from '~icons/fluent/subtract-20-regular'
import ArrowReset from '~icons/fluent/arrow-counterclockwise-20-regular'
import { cssVar } from '@/theme/theme'
import { useThemeStore } from '@/stores/theme'
import { formatCount } from '@/utils/format'

// 国家分级着色地图；data.code 为 ISO 3166-1 alpha-2
const props = withDefaults(defineProps<{
  data: { code: string; name?: string; value: number }[]
  unit?: string
}>(), { unit: '' })

const token = import.meta.env.VITE_MAPBOX_TOKEN as string | undefined
const ACTIVE = '0, 122, 255'
const CENTER: [number, number] = [105, 30]
const ZOOM = 0.6
const styles = { light: 'mapbox://styles/mapbox/light-v10', dark: 'mapbox://styles/mapbox/dark-v10' }

const el = ref<HTMLDivElement>()
const map = shallowRef<mapboxgl.Map>()
const theme = useThemeStore()
const max = computed(() => Math.max(0, ...props.data.map(d => d.value)))
let popup: mapboxgl.Popup | undefined

// 按占比开方映射到 [0.15, 1]：小值仍可见，数量差异也能区分
function density(v: number) {
  if (!max.value) return 0.1
  return 0.15 + 0.85 * Math.sqrt(v / max.value)
}

function fillColor(): any {
  if (!props.data.length) return 'rgba(255,255,255,0)'
  return ['match', ['get', 'iso_3166_1'], ...props.data.flatMap(d => [d.code.toUpperCase(), `rgba(${ACTIVE}, ${density(d.value)})`]), 'rgba(255,255,255,0)']
}

function paintBase(m: mapboxgl.Map) {
  const water = cssVar('--color-map-water')
  const land = cssVar('--color-map-land')
  if (m.getLayer('water')) m.setPaintProperty('water', 'fill-color', water)
  if (m.getLayer('land')) m.setPaintProperty('land', 'background-color', land)
  if (m.getLayer('landcover')) m.setPaintProperty('landcover', 'fill-color', land)
}

function addCountries(m: mapboxgl.Map) {
  if (m.getLayer('countries')) return
  m.addLayer({
    id: 'countries', type: 'fill',
    source: { type: 'vector', url: 'mapbox://mapbox.country-boundaries-v1' },
    'source-layer': 'country_boundaries',
    filter: ['all', ['==', ['get', 'disputed'], 'false'], ['any', ['==', 'all', ['get', 'worldview']], ['in', 'CN', ['get', 'worldview']]]],
    paint: { 'fill-color': fillColor() },
  })
  m.on('mousemove', 'countries', e => {
    const f = e.features?.[0] as any
    const code = f?.properties?.iso_3166_1 as string | undefined
    const d = props.data.find(x => x.code.toUpperCase() === code)
    if (!d) { popup?.remove(); return }
    m.getCanvas().style.cursor = 'pointer'
    popup = (popup ?? new mapboxgl.Popup({ closeButton: false, closeOnClick: false }))
      .setLngLat(e.lngLat)
      .setHTML(`<div>${d.name || f?.properties?.name_zh || f?.properties?.name_en || code}</div><div style="color:var(--color-text-primary);font-weight:700">${formatCount(d.value)} ${props.unit}</div>`)
      .addTo(m)
  })
  m.on('mouseleave', 'countries', () => { m.getCanvas().style.cursor = ''; popup?.remove() })
}

function init() {
  if (!token || !el.value) return
  mapboxgl.accessToken = token
  const m = new mapboxgl.Map({
    container: el.value, style: styles[theme.isDark ? 'dark' : 'light'],
    center: CENTER, zoom: ZOOM, attributionControl: false, projection: 'mercator' as any,
  })
  m.on('style.load', () => { paintBase(m); addCountries(m) })
  map.value = m
}

onMounted(init)
onBeforeUnmount(() => { popup?.remove(); map.value?.remove() })
watch(() => props.data, () => map.value?.getLayer('countries') && map.value.setPaintProperty('countries', 'fill-color', fillColor()), { deep: true })
watch(() => theme.isDark, d => map.value?.setStyle(styles[d ? 'dark' : 'light']))

const zoomIn = () => map.value?.zoomIn()
const zoomOut = () => map.value?.zoomOut()
const reset = () => map.value?.flyTo({ center: CENTER, zoom: ZOOM })
</script>

<template>
  <div class="ax-map relative h-full min-h-300px">
    <template v-if="token">
      <div ref="el" class="h-full w-full rounded-10px [&_canvas]:rounded-10px" />
      <div class="absolute right-10px top-10px z-2 flex flex-col gap-4px rounded-8px bg-secondary p-1">
        <button class="ax-map-btn" title="放大" @click="zoomIn"><NIcon :size="16"><Add /></NIcon></button>
        <button class="ax-map-btn" title="缩小" @click="zoomOut"><NIcon :size="16"><Subtract /></NIcon></button>
        <button class="ax-map-btn" title="复位" @click="reset"><NIcon :size="16"><ArrowReset /></NIcon></button>
      </div>
      <!-- 图例：渐变条，只标首尾 -->
      <div class="pointer-events-none absolute bottom-26px left-16px w-160px">
        <div class="h-6px rounded-full" :style="{ background: `linear-gradient(90deg, rgba(${ACTIVE},.1), rgba(${ACTIVE},1))` }" />
        <div class="mt-4px flex justify-between text-sm text-secondary">
          <span>0</span><span>{{ formatCount(max) }}</span>
        </div>
      </div>
    </template>
    <div v-else class="flex h-full flex-col items-center justify-center gap-xs rounded-10px bg-fill text-center">
      <div class="text-lg font-bold">地图未启用</div>
      <div class="text-sm text-secondary">构建时设置 VITE_MAPBOX_TOKEN 后显示</div>
    </div>
  </div>
</template>

<style scoped>
.ax-map-btn {
  display: flex; align-items: center; justify-content: center;
  width: 28px; height: 28px; border: 0; border-radius: 6px;
  background: transparent; color: var(--color-text-secondary); cursor: pointer;
}
.ax-map-btn:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.ax-map :deep(.mapboxgl-popup-tip) { border-top-color: var(--color-secondary); border-bottom-color: var(--color-secondary); }
.ax-map :deep(.mapboxgl-popup-content) { background: var(--color-secondary); padding: var(--spacing-sm); color: var(--color-text-secondary); border-radius: 8px; box-shadow: none; font-size: 12px; }
.ax-map :deep(.mapboxgl-ctrl-logo) { opacity: .6; }
</style>
