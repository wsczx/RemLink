<script setup lang="ts">
// asciicast v2 录像回放：基于 xterm 按时间戳重放（播放 / 暂停、倍速、拖动进度、跳过空闲）
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NSelect, NSlider, NSwitch } from 'naive-ui'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import Play from '~icons/fluent/play-20-regular'
import Pause from '~icons/fluent/pause-20-regular'

const props = defineProps<{ cast: string }>()

interface Ev { t: number; raw: number; type: string; data: string }

const el = ref<HTMLDivElement>()
const err = ref('')
const header = ref<{ width: number; height: number; timestamp?: number; title?: string }>({ width: 80, height: 24 })
const rawEvents = ref<Ev[]>([])
const skipIdle = ref(true)
const speed = ref(1)
const playing = ref(false)
const cur = ref(0)
const screenBg = ref('')

const IDLE_CAP = 2 // 跳过空闲：两帧间隔最多 2 秒

let term: Terminal | null = null
let idx = 0
let raf = 0
let last = 0

// 解析 .cast（首行 header，其余每行 [time, type, data]）
function parse(text: string) {
  const lines = text.split('\n').filter(l => l.trim())
  if (!lines.length) throw new Error('录像为空')
  const h = JSON.parse(lines[0])
  if (h.version !== 2) throw new Error(h.msg || '不支持的录像格式')
  header.value = { width: h.width || 80, height: h.height || 24, timestamp: h.timestamp, title: h.title }
  const out: Ev[] = []
  for (let i = 1; i < lines.length; i++) {
    try {
      const [t, type, data] = JSON.parse(lines[i])
      if (typeof t === 'number' && (type === 'o' || type === 'r')) out.push({ t, raw: t, type, data: String(data) })
    } catch { /* 跳过损坏行（录像可能在写入中途被截断） */ }
  }
  rawEvents.value = out
}

// 应用「跳过空闲」后的时间轴
const events = computed<Ev[]>(() => {
  if (!skipIdle.value) return rawEvents.value
  let shift = 0
  let prev = 0
  return rawEvents.value.map(e => {
    const gap = e.raw - prev
    if (gap > IDLE_CAP) shift += gap - IDLE_CAP
    prev = e.raw
    return { ...e, t: e.raw - shift }
  })
})
const duration = computed(() => events.value[events.value.length - 1]?.t || 0)

function applyResize(data: string) {
  const m = /^(\d+)x(\d+)$/.exec(data)
  if (m && term) term.resize(Math.max(2, +m[1]), Math.max(2, +m[2]))
}

// 输出至 t 为止的全部事件（批量合并写入）
function flushTo(t: number) {
  if (!term) return
  let buf = ''
  const evs = events.value
  while (idx < evs.length && evs[idx].t <= t) {
    const e = evs[idx++]
    if (e.type === 'o') buf += e.data
    else {
      if (buf) { term.write(buf); buf = '' }
      applyResize(e.data)
    }
  }
  if (buf) term.write(buf)
}

function seek(t: number) {
  if (!term) return
  term.reset()
  term.resize(header.value.width, header.value.height)
  idx = 0
  cur.value = Math.max(0, Math.min(t, duration.value))
  flushTo(cur.value)
}

function frame(now: number) {
  if (!playing.value) return
  const dt = (now - last) / 1000
  last = now
  cur.value = Math.min(duration.value, cur.value + dt * speed.value)
  flushTo(cur.value)
  if (cur.value >= duration.value) { playing.value = false; return }
  raf = requestAnimationFrame(frame)
}

function toggle() {
  if (playing.value) { playing.value = false; cancelAnimationFrame(raf); return }
  if (cur.value >= duration.value) seek(0)
  playing.value = true
  last = performance.now()
  raf = requestAnimationFrame(frame)
}

// 切换跳过空闲：按原始时间位置换算后重新定位
watch(skipIdle, (_v, _o) => {
  const e = events.value[Math.max(0, idx - 1)]
  seek(e ? e.t : 0)
})

const fmt = (s: number) => {
  s = Math.floor(s)
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = s % 60
  return (h ? h + ':' + String(m).padStart(2, '0') : String(m)) + ':' + String(x).padStart(2, '0')
}
const speedOpts = [0.5, 1, 2, 4, 8].map(v => ({ label: v + 'x', value: v }))

function onKey(e: KeyboardEvent) {
  if (e.key === ' ') { e.preventDefault(); toggle() }
  else if (e.key === 'ArrowRight') seek(cur.value + 5)
  else if (e.key === 'ArrowLeft') seek(cur.value - 5)
}

onMounted(() => {
  try {
    parse(props.cast)
  } catch (e) {
    err.value = (e as Error).message || '录像解析失败'
    return
  }
  // 回放画面始终为深色：浅色主题取反色，深色主题取页面底色
  const css = getComputedStyle(document.documentElement)
  const dark = document.documentElement.dataset.theme === 'dark'
  const bg = css.getPropertyValue(dark ? '--color-primary' : '--color-inverse').trim()
  const fg = css.getPropertyValue(dark ? '--color-text-primary' : '--color-text-inverse').trim()
  screenBg.value = bg
  term = new Terminal({
    cols: header.value.width, rows: header.value.height, disableStdin: true, cursorBlink: false, scrollback: 2000,
    fontFamily: 'v-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace', fontSize: 13, lineHeight: 1.15,
    theme: { background: bg, foreground: fg },
  })
  term.open(el.value!)
  seek(0)
  toggle()
})
onBeforeUnmount(() => { playing.value = false; cancelAnimationFrame(raf); term?.dispose() })
</script>

<template>
  <div tabindex="0" class="outline-none" @keydown="onKey">
    <div v-if="err" class="py-32px text-center text-error">{{ err }}</div>
    <template v-else>
      <div class="cast-screen" :style="{ background: screenBg }"><div ref="el" /></div>
      <div class="mt-12px flex items-center gap-12px">
        <NButton circle secondary @click="toggle">
          <template #icon><Pause v-if="playing" /><Play v-else /></template>
        </NButton>
        <span class="rl-mono w-100px flex-shrink-0 text-sm! text-secondary">{{ fmt(cur) }} / {{ fmt(duration) }}</span>
        <NSlider
          :value="cur" :min="0" :max="duration || 1" :step="0.1" :tooltip="false" class="flex-1"
          @update:value="(v: number) => seek(v)"
        />
        <NSelect v-model:value="speed" :options="speedOpts" size="small" class="w-80px!" />
        <label class="flex flex-shrink-0 items-center gap-6px text-sm text-secondary">
          <NSwitch v-model:value="skipIdle" size="small" />跳过空闲
        </label>
      </div>
    </template>
  </div>
</template>

<style scoped>
.cast-screen { overflow: auto; max-height: 64vh; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--color-border-default); }
</style>
