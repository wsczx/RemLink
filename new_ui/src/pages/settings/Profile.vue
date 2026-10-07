<script setup lang="ts">
// 客户端 Profile：AnyConnect 客户端配置 XML 编辑；顶部显示格式校验与下发指纹（SHA1），吸底保存
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { refDebounced } from '@vueuse/core'
import { NSkeleton } from 'naive-ui'
import Copy from '~icons/fluent/copy-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import PrfCodeEditor from './components/PrfCodeEditor.vue'
import SvcSaveBar from './components/SvcSaveBar.vue'
import { get, post, errMsg } from '@/api/http'
import { ui, confirm } from '@/utils/ui'
import { formatBytes } from '@/utils/format'

const content = ref('')
const orig = ref('')
const hash = ref('')
const loading = ref(true)
const saving = ref(false)
const editor = ref<InstanceType<typeof PrfCodeEditor> | null>(null)

async function load() {
  loading.value = true
  try {
    const d = await get('/api/v1/set/profile')
    content.value = orig.value = d?.content || ''
    hash.value = d?.hash || ''
  } catch (e) {
    ui.message.error(errMsg(e, 'Profile 获取失败'))
  } finally {
    loading.value = false
  }
}

const dirty = computed(() => content.value !== orig.value)

// —— XML 格式校验（仅本地提示，不阻止保存）——
const debounced = refDebounced(content, 300)
const xmlCheck = computed(() => {
  const text = debounced.value.trim()
  if (!text) return { ok: false, line: 0, msg: '内容为空' }
  const doc = new DOMParser().parseFromString(text, 'application/xml')
  const err = doc.getElementsByTagName('parsererror')[0]
  if (!err) return { ok: true, line: 0, msg: '' }
  const raw = err.textContent || ''
  const m = raw.match(/line(?: number)?\s*(\d+)/i)
  return { ok: false, line: m ? +m[1] : 0, msg: raw.split('\n').find(Boolean)?.trim() || 'XML 格式错误' }
})
const stats = computed(() => ({
  lines: content.value.split('\n').length,
  size: formatBytes(new Blob([content.value]).size),
}))

async function save() {
  if (!content.value) return ui.message.warning('Profile 内容不能为空')
  if (!xmlCheck.value.ok && !(await confirm(`当前内容不是合法的 XML${xmlCheck.value.line ? `（第 ${xmlCheck.value.line} 行附近）` : ''}，客户端可能无法解析。仍要保存吗？`, '格式校验未通过'))) return
  saving.value = true
  try {
    const d = await post('/api/v1/set/profile/edit', { content: content.value })
    hash.value = d?.hash || ''
    orig.value = content.value
    ui.message.success('Profile 已保存，客户端下次连接时自动更新')
  } catch (e) {
    ui.message.error(errMsg(e, 'Profile 保存失败'))
  } finally {
    saving.value = false
  }
}
function reset() { content.value = orig.value }

async function copy(text: string, tip = '已复制') {
  try {
    await navigator.clipboard.writeText(text)
    ui.message.success(tip)
  } catch {
    ui.message.warning('复制失败，请手动选择复制')
  }
}
function download() {
  const url = URL.createObjectURL(new Blob([content.value], { type: 'application/xml' }))
  const a = document.createElement('a')
  a.href = url
  a.download = 'profile.xml'
  a.click()
  URL.revokeObjectURL(url)
}

onBeforeRouteLeave(async () => (dirty.value ? await confirm('Profile 修改尚未保存，离开后将丢失。', '放弃未保存的修改？') : true))
function beforeUnload(e: BeforeUnloadEvent) { if (dirty.value) e.preventDefault() }
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload)
  load()
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>

<template>
  <PageHeader title="客户端 Profile" description="下发给 AnyConnect / OpenConnect 客户端的配置文件（XML），客户端连接时按指纹比对并自动更新。" />

  <div class="grid gap-16px">
    <!-- 状态概览 -->
    <div class="rl-panel prf-status">
      <div class="prf-status__cell">
        <div class="text-sm text-secondary">格式校验</div>
        <div class="mt-4px">
          <StatusDot v-if="loading" tone="muted">检查中…</StatusDot>
          <StatusDot v-else-if="xmlCheck.ok" tone="success">XML 格式正确</StatusDot>
          <button v-else type="button" class="prf-errbtn" :title="xmlCheck.msg" @click="xmlCheck.line && editor?.gotoLine(xmlCheck.line)">
            <StatusDot tone="error">{{ xmlCheck.line ? `第 ${xmlCheck.line} 行附近有错误` : xmlCheck.msg }}</StatusDot>
          </button>
        </div>
      </div>
      <div class="prf-status__cell min-w-0 flex-[2]!">
        <div class="text-sm text-secondary">当前下发指纹 · SHA1</div>
        <div class="mt-4px flex min-w-0 items-center gap-4px">
          <span class="rl-mono truncate">{{ hash || '-' }}</span>
          <button v-if="hash" class="rl-icon-btn prf-mini" title="复制指纹" @click="copy(hash)"><Copy /></button>
        </div>
      </div>
      <div class="prf-status__cell">
        <div class="text-sm text-secondary">大小</div>
        <div class="mt-4px tabular-nums">{{ stats.lines }} 行 · {{ stats.size }}</div>
      </div>
    </div>

    <RlPanel :padded="false">
      <template #header>
        <div class="flex min-w-0 items-center gap-sm">
          <span class="rl-mono font-semibold">profile.xml</span>
          <span v-if="dirty" class="ax-badge text-warning">未保存</span>
        </div>
      </template>
      <template #extra>
        <span class="mr-6px hidden text-sm text-tertiary md:inline">Ctrl + S 保存</span>
        <button class="rl-icon-btn" title="复制全部内容" @click="copy(content, '内容已复制')"><Copy /></button>
        <button class="rl-icon-btn" title="下载 profile.xml" :disabled="!content" @click="download"><ArrowDownload /></button>
        <button class="rl-icon-btn" title="重新加载" @click="load"><ArrowClockwise /></button>
      </template>
      <div v-if="loading" class="p-20px"><NSkeleton text :repeat="10" /></div>
      <PrfCodeEditor
        v-else ref="editor" v-model="content" :error-line="xmlCheck.ok ? 0 : xmlCheck.line" :min-rows="24"
        placeholder="<?xml version=&quot;1.0&quot; encoding=&quot;UTF-8&quot;?>&#10;<AnyConnectProfile …>" @save="dirty && save()"
      />
    </RlPanel>
  </div>

  <SvcSaveBar :dirty="dirty" :saving="saving" text="Profile 有未保存的修改" hint="保存后指纹会变化，客户端下次连接时拉取新配置" @reset="reset" @save="save" />
</template>

<style scoped>
.prf-status { display: flex; flex-wrap: wrap; }
.prf-status__cell { flex: 1; min-width: 180px; padding: 14px 20px; font-size: 14px; }
.prf-status__cell + .prf-status__cell { border-left: 1px solid var(--color-border-default); }
@media (max-width: 760px) { .prf-status__cell + .prf-status__cell { border-left: 0; border-top: 1px solid var(--color-border-default); } }
.prf-errbtn { padding: 0; border: 0; background: none; font: inherit; cursor: pointer; max-width: 100%; }
.prf-mini { width: 24px; height: 24px; font-size: 15px; }
</style>
