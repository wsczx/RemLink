<script setup lang="ts">
// Guacamole 会话录像（.guac）回放：Guacamole.SessionRecording 解析 Blob，支持播放 / 暂停、拖动进度
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { NButton, NSlider } from 'naive-ui'
import Guacamole from 'guacamole-common-js'
import Play from '~icons/fluent/play-20-regular'
import Pause from '~icons/fluent/pause-20-regular'

const props = defineProps<{ blob: Blob }>()

const box = ref<HTMLDivElement>()
const host = ref<HTMLDivElement>()
const loaded = ref(false)
const err = ref('')
const playing = ref(false)
const seeking = ref(false)
const duration = ref(0) // ms
const position = ref(0) // ms
const size = ref({ w: 0, h: 0 })

let rec: Guacamole.SessionRecording | null = null
let ro: ResizeObserver | null = null
let timer = 0

const MAX_H = () => Math.max(240, Math.round(window.innerHeight * 0.62))

function rescale() {
  const d = rec?.getDisplay()
  const el = box.value
  if (!d || !el) return
  const w = d.getWidth()
  const h = d.getHeight()
  if (!w || !h) return
  size.value = { w, h }
  d.scale(Math.min(el.clientWidth / w, MAX_H() / h, 1))
}

function fmt(ms: number) {
  const s = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}
const label = computed(() => `${fmt(position.value)} / ${fmt(duration.value)}`)

function toggle() {
  if (!rec || !loaded.value) return
  if (rec.isPlaying()) rec.pause()
  else {
    // 播放结束后从头开始
    if (position.value >= duration.value && duration.value > 0) rec.seek(0, () => rec?.play())
    else rec.play()
  }
}

function seekTo(v: number) {
  if (!rec) return
  position.value = v
  seeking.value = true
  rec.seek(v, () => { seeking.value = false })
}

function onKey(e: KeyboardEvent) {
  if (!rec || !loaded.value) return
  if (e.key === ' ') { e.preventDefault(); toggle() }
  else if (e.key === 'ArrowRight') seekTo(Math.min(duration.value, position.value + 5000))
  else if (e.key === 'ArrowLeft') seekTo(Math.max(0, position.value - 5000))
}

onMounted(() => {
  const r = new Guacamole.SessionRecording(props.blob)
  rec = r
  const d = r.getDisplay()
  host.value!.appendChild(d.getElement())
  d.onresize = () => rescale()
  r.onprogress = dur => { duration.value = dur }
  r.onload = () => {
    loaded.value = true
    duration.value = r.getDuration()
    // 渲染首帧
    r.seek(0, () => rescale())
  }
  r.onerror = msg => { err.value = msg || '录像解析失败' }
  r.onplay = () => { playing.value = true }
  r.onpause = () => { playing.value = false; position.value = r.getPosition() }
  r.onseek = pos => { position.value = pos }
  // 播放中刷新进度
  timer = window.setInterval(() => {
    if (r.isPlaying()) position.value = r.getPosition()
  }, 250)
  ro = new ResizeObserver(rescale)
  ro.observe(box.value!)
  window.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  clearInterval(timer)
  ro?.disconnect()
  window.removeEventListener('keydown', onKey)
  if (rec) {
    rec.onplay = rec.onpause = rec.onseek = rec.onload = null
    try { rec.pause(); rec.abort() } catch { /* 忽略 */ }
  }
  rec = null
})
</script>

<template>
  <div>
    <div ref="box" class="gp-screen">
      <div ref="host" class="gp-display" />
      <div v-if="err" class="gp-mask text-error">{{ err }}</div>
      <div v-else-if="!loaded" class="gp-mask text-secondary">正在解析录像… {{ duration ? fmt(duration) : '' }}</div>
    </div>
    <div class="mt-10px flex items-center gap-12px">
      <NButton size="small" circle :disabled="!loaded || !!err" @click="toggle">
        <template #icon><Pause v-if="playing" /><Play v-else /></template>
      </NButton>
      <NSlider
        class="flex-1" :value="position" :min="0" :max="Math.max(duration, 1)" :step="100" :disabled="!loaded || !!err"
        :format-tooltip="(v: number) => fmt(v)" @update:value="seekTo"
      />
      <span class="tabular-nums text-sm text-secondary">{{ label }}</span>
      <span v-if="size.w" class="rl-mono text-sm! text-tertiary">{{ size.w }}×{{ size.h }}</span>
    </div>
  </div>
</template>

<style scoped>
/* 回放画面固定深色底（功能性底色，与远端桌面边缘过渡） */
.gp-screen { position: relative; display: flex; justify-content: center; min-height: 240px; border-radius: 8px; overflow: hidden; background: #000; }
.gp-display { flex-shrink: 0; }
.gp-mask { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; font-size: 13px; background: var(--color-primary); }
</style>
