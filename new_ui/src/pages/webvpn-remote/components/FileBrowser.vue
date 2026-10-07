<script setup lang="ts">
// 文件浏览器：面包屑、列表（目录在前）、多选、上传文件 / 文件夹（含拖拽、分片续传）、下载（目录为 zip）、新建目录、重命名、删除
import { computed, h, nextTick, onMounted, ref, watch } from 'vue'
import { NButton, NDataTable, NDropdown, NInput, NModal, NTooltip, type DataTableColumns, type DropdownOption } from 'naive-ui'
import FolderIcon from '~icons/fluent/folder-20-regular'
import DocIcon from '~icons/fluent/document-20-regular'
import LinkIcon from '~icons/fluent/link-20-regular'
import Upload from '~icons/fluent/arrow-upload-20-regular'
import FolderUp from '~icons/fluent/folder-arrow-up-20-regular'
import FolderAdd from '~icons/fluent/folder-add-20-regular'
import Download from '~icons/fluent/arrow-download-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import Rename from '~icons/fluent/rename-20-regular'
import More from '~icons/fluent/more-horizontal-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArrowUp from '~icons/fluent/arrow-up-20-regular'
import Home from '~icons/fluent/home-20-regular'
import Eye from '~icons/fluent/eye-20-regular'
import EyeOff from '~icons/fluent/eye-off-20-regular'
import ChevronRight from '~icons/fluent/chevron-right-20-regular'
import Search from '~icons/fluent/search-20-regular'
import { formatBytes, formatDate } from '@/utils/format'
import { confirm, ui } from '@/utils/ui'
import { friendlyError, joinPath, parentPath, remoteApi, type FileEntry, type Permissions } from '../api'
import { collectDropped, useUploader, type PendingFile } from '../upload'
import UploadQueue from './UploadQueue.vue'

const props = defineProps<{ sid: string; perms: Permissions; compact?: boolean }>()

const cwd = ref('/')
const entries = ref<FileEntry[]>([])
const loading = ref(false)
const loadErr = ref('')
const checked = ref<string[]>([])
const showHidden = ref(false)
const kw = ref('')
const dragging = ref(false)
let dragDepth = 0

const fileInput = ref<HTMLInputElement>()
const dirInput = ref<HTMLInputElement>()

const up = useUploader({
  sid: () => props.sid,
  maxSize: () => props.perms.max_file_size,
  onFileDone: dir => { if (dir === cwd.value) scheduleRefresh() },
})

let refreshTimer = 0
function scheduleRefresh() {
  clearTimeout(refreshTimer)
  refreshTimer = window.setTimeout(() => load(true), 400)
}

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    const d = await remoteApi.list(props.sid, cwd.value)
    cwd.value = d.path || cwd.value
    entries.value = d.entries || []
    loadErr.value = ''
    const names = new Set(entries.value.map(e => e.path))
    checked.value = checked.value.filter(p => names.has(p))
  } catch (e) {
    loadErr.value = friendlyError(e)
    if (!silent) entries.value = []
  } finally {
    loading.value = false
  }
}

function go(p: string) {
  if (p === cwd.value) return load()
  cwd.value = p
  checked.value = []
  kw.value = ''
  load()
}
onMounted(() => load())
watch(() => props.sid, () => go('/'))

// 目录在前，再按名称
const rows = computed(() => {
  const k = kw.value.trim().toLowerCase()
  return entries.value
    .filter(e => showHidden.value || !e.name.startsWith('.'))
    .filter(e => !k || e.name.toLowerCase().includes(k))
    .sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1))
})
const hiddenCount = computed(() => entries.value.filter(e => e.name.startsWith('.')).length)

const crumbs = computed(() => {
  const parts = cwd.value.split('/').filter(Boolean)
  return parts.map((name, i) => ({ name, path: '/' + parts.slice(0, i + 1).join('/') }))
})

const selected = computed(() => entries.value.filter(e => checked.value.includes(e.path)))
const canWrite = computed(() => props.perms.upload)

// ---------- 下载 ----------
function triggerDownload(e: FileEntry) {
  const a = document.createElement('a')
  a.href = remoteApi.downloadUrl(props.sid, e.path)
  a.download = e.is_dir ? e.name + '.zip' : e.name
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
}
function download(list: FileEntry[]) {
  if (!props.perms.download) return
  const max = props.perms.max_file_size
  const ok = list.filter(e => {
    if (!e.is_dir && max > 0 && e.size > max) {
      ui.message.warning(`「${e.name}」超出下载大小上限 ${formatBytes(max)}`)
      return false
    }
    return true
  })
  // 多个下载间隔触发，避免浏览器合并拦截
  ok.forEach((e, i) => setTimeout(() => triggerDownload(e), i * 400))
  if (ok.some(e => e.is_dir)) ui.message.info('目录将打包为 zip 下载，文件较多时需要等待片刻')
}

// ---------- 新建 / 重命名 / 删除 ----------
const nameDlg = ref<{ mode: 'mkdir' | 'rename'; value: string; target?: FileEntry } | null>(null)
const nameInput = ref<InstanceType<typeof NInput>>()
const nameBusy = ref(false)
function openMkdir() {
  nameDlg.value = { mode: 'mkdir', value: '' }
  nextTick(() => nameInput.value?.focus())
}
function openRename(e: FileEntry) {
  nameDlg.value = { mode: 'rename', value: e.name, target: e }
  nextTick(() => nameInput.value?.select())
}
async function submitName() {
  const d = nameDlg.value
  if (!d) return
  const name = d.value.trim()
  if (!name || name.includes('/') || name === '.' || name === '..') {
    ui.message.warning('名称不合法')
    return
  }
  nameBusy.value = true
  try {
    if (d.mode === 'mkdir') await remoteApi.mkdir(props.sid, joinPath(cwd.value, name))
    else if (d.target && name !== d.target.name) await remoteApi.rename(props.sid, d.target.path, joinPath(parentPath(d.target.path), name))
    nameDlg.value = null
    load(true)
  } catch (e) {
    ui.message.error(friendlyError(e))
  } finally {
    nameBusy.value = false
  }
}
function onPositive() { submitName(); return false }
async function remove(list: FileEntry[]) {
  if (!list.length || !props.perms.delete) return
  const dirs = list.filter(e => e.is_dir).length
  const what = list.length === 1 ? `「${list[0].name}」` : `选中的 ${list.length} 项`
  const tip = dirs ? '，目录将连同其中所有内容一起删除' : ''
  if (!(await confirm(`确认删除${what}？此操作不可撤销${tip}。`, '删除'))) return
  try {
    await remoteApi.remove(props.sid, list.map(e => e.path))
    ui.message.success('已删除')
    checked.value = []
  } catch (e) {
    ui.message.error(friendlyError(e))
  }
  load(true)
}

// ---------- 上传 ----------
function pickFiles() { fileInput.value?.click() }
function pickDir() { dirInput.value?.click() }
function onPicked(ev: Event, isDir: boolean) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length) return
  enqueue(files.map(f => ({ file: f, relpath: isDir ? (f.webkitRelativePath || f.name) : f.name })), [])
}
async function enqueue(files: PendingFile[], emptyDirs: string[]) {
  if (!canWrite.value) return
  const dir = cwd.value
  // 空目录单独创建（有文件的目录由后端按 relpath 自动创建）
  for (const d of emptyDirs) {
    try { await remoteApi.mkdir(props.sid, joinPath(dir, d)) } catch (e) { ui.message.error(`创建目录 ${d} 失败：${friendlyError(e)}`) }
  }
  if (files.length) up.add(dir, files)
  else if (emptyDirs.length) load(true)
}

function hasFiles(e: DragEvent) { return !!e.dataTransfer?.types?.includes('Files') }
function onDragEnter(e: DragEvent) {
  if (!hasFiles(e) || !canWrite.value) return
  dragDepth++
  dragging.value = true
}
function onDragLeave(e: DragEvent) {
  if (!hasFiles(e)) return
  dragDepth = Math.max(0, dragDepth - 1)
  if (!dragDepth) dragging.value = false
}
function onDragOver(e: DragEvent) {
  if (!hasFiles(e)) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = canWrite.value ? 'copy' : 'none'
}
async function onDrop(e: DragEvent) {
  if (!hasFiles(e)) return
  e.preventDefault()
  dragDepth = 0
  dragging.value = false
  if (!canWrite.value || !e.dataTransfer) {
    ui.message.warning('管理员未允许上传')
    return
  }
  try {
    const { files, dirs } = await collectDropped(e.dataTransfer)
    enqueue(files, dirs)
  } catch (err) {
    ui.message.error('读取拖入的文件失败：' + friendlyError(err))
  }
}

// ---------- 表格 ----------
function rowMenu(e: FileEntry): DropdownOption[] {
  return [
    { key: 'download', label: e.is_dir ? '下载为 zip' : '下载', disabled: !props.perms.download },
    { key: 'rename', label: '重命名', disabled: !props.perms.upload },
    { type: 'divider', key: 'd' },
    { key: 'delete', label: () => h('span', { class: props.perms.delete ? 'text-error' : '' }, '删除'), disabled: !props.perms.delete },
  ]
}
function onMenu(key: string, e: FileEntry) {
  if (key === 'download') download([e])
  else if (key === 'rename') openRename(e)
  else if (key === 'delete') remove([e])
}

function open(e: FileEntry) {
  if (e.is_dir) go(e.path)
  else if (props.perms.download) download([e])
}

const columns = computed<DataTableColumns<FileEntry>>(() => {
  const cols: DataTableColumns<FileEntry> = [
    { type: 'selection', width: 40 },
    {
      title: '名称', key: 'name', minWidth: 200, ellipsis: { tooltip: true },
      render: e => h('div', { class: 'flex min-w-0 items-center gap-8px' }, [
        h(e.is_dir ? FolderIcon : e.is_link ? LinkIcon : DocIcon, { class: ['flex-shrink-0 text-lg', e.is_dir ? 'text-info' : 'text-secondary'] }),
        h('span', { class: ['truncate', e.is_dir && 'font-semibold', e.name.startsWith('.') && 'text-secondary'] }, e.name),
      ]),
    },
    { title: '大小', key: 'size', width: 100, align: 'right', render: e => h('span', { class: 'text-secondary' }, e.is_dir ? '—' : formatBytes(e.size)) },
  ]
  if (!props.compact) {
    cols.push(
      { title: '修改时间', key: 'mod_time', width: 170, render: e => h('span', { class: 'text-secondary' }, formatDate(e.mod_time * 1000, 'YYYY-MM-DD HH:mm')) },
      { title: '权限', key: 'mode', width: 120, render: e => h('span', { class: 'rl-mono text-sm! text-secondary' }, e.mode) },
    )
  }
  cols.push({
    title: '', key: 'actions', width: 48, align: 'right',
    render: e => h(NDropdown, { trigger: 'click', options: rowMenu(e), onSelect: (k: string) => onMenu(k, e) }, {
      default: () => h('button', { class: 'rl-icon-btn', title: '更多', onClick: (ev: MouseEvent) => ev.stopPropagation() }, [h(More)]),
    }),
  })
  return cols
})

const rowProps = (e: FileEntry) => ({
  style: 'cursor: default',
  onDblclick: () => open(e),
})

function onKey(ev: KeyboardEvent) {
  const t = ev.target as HTMLElement
  if (t?.closest('input, textarea')) return
  if (ev.key === 'Backspace' && cwd.value !== '/') { ev.preventDefault(); go(parentPath(cwd.value)) }
  else if (ev.key === 'Delete' && selected.value.length) remove(selected.value)
  else if (ev.key === 'F2' && selected.value.length === 1 && canWrite.value) openRename(selected.value[0])
}

defineExpose({ refresh: () => load(true) })
</script>

<template>
  <div
    class="fb" tabindex="-1" @keydown="onKey"
    @dragenter="onDragEnter" @dragleave="onDragLeave" @dragover="onDragOver" @drop="onDrop"
  >
    <!-- 路径 -->
    <div class="fb__bar">
      <button class="rl-icon-btn" title="上一级" :disabled="cwd === '/'" @click="go(parentPath(cwd))"><ArrowUp /></button>
      <nav class="fb__crumbs">
        <button class="fb__crumb" title="根目录" @click="go('/')"><Home /></button>
        <template v-for="c in crumbs" :key="c.path">
          <ChevronRight class="flex-shrink-0 text-sm text-quaternary" />
          <button class="fb__crumb" :class="c.path === cwd && 'fb__crumb--cur'" @click="go(c.path)">{{ c.name }}</button>
        </template>
      </nav>
      <button class="rl-icon-btn" title="刷新" @click="load()"><ArrowClockwise /></button>
    </div>

    <!-- 工具栏 -->
    <div class="fb__tools">
      <template v-if="selected.length">
        <span class="text-sm text-secondary">已选 {{ selected.length }} 项</span>
        <NButton v-if="perms.download" size="small" secondary @click="download(selected)">
          <template #icon><Download /></template>下载
        </NButton>
        <NButton v-if="perms.upload && selected.length === 1" size="small" secondary @click="openRename(selected[0])">
          <template #icon><Rename /></template>重命名
        </NButton>
        <NButton v-if="perms.delete" size="small" secondary type="error" @click="remove(selected)">
          <template #icon><Delete /></template>删除
        </NButton>
        <NButton size="small" text @click="checked = []">取消选择</NButton>
      </template>
      <template v-else>
        <template v-if="perms.upload">
          <NButton size="small" secondary @click="pickFiles"><template #icon><Upload /></template>上传文件</NButton>
          <NButton size="small" secondary @click="pickDir"><template #icon><FolderUp /></template>上传文件夹</NButton>
          <NTooltip>
            <template #trigger>
              <button class="rl-icon-btn" @click="openMkdir"><FolderAdd /></button>
            </template>
            新建目录
          </NTooltip>
        </template>
        <span v-else class="text-sm text-tertiary">只读：管理员未允许上传</span>
      </template>
      <div class="flex-1" />
      <NInput v-model:value="kw" size="small" placeholder="筛选" clearable class="fb__filter">
        <template #prefix><Search class="text-tertiary" /></template>
      </NInput>
      <NTooltip v-if="hiddenCount">
        <template #trigger>
          <button class="rl-icon-btn" @click="showHidden = !showHidden"><Eye v-if="showHidden" /><EyeOff v-else /></button>
        </template>
        {{ showHidden ? '隐藏' : '显示' }}隐藏文件（{{ hiddenCount }}）
      </NTooltip>
    </div>

    <!-- 列表 -->
    <div class="fb__body">
      <div v-if="loadErr && !entries.length" class="flex h-full flex-col items-center justify-center gap-8px p-24px text-center">
        <div class="text-md font-semibold">无法打开此目录</div>
        <div class="text-sm text-secondary">{{ loadErr }}</div>
        <div class="flex gap-8px">
          <NButton size="small" @click="go('/')">回到根目录</NButton>
          <NButton size="small" @click="load()">重试</NButton>
        </div>
      </div>
      <NDataTable
        v-else
        v-model:checked-row-keys="checked"
        :columns="columns" :data="rows" :loading="loading" :bordered="false" size="small"
        :row-key="(e: FileEntry) => e.path" :row-props="rowProps" flex-height class="h-full"
      >
        <template #empty>
          <div class="py-32px text-center">
            <div class="text-md text-secondary">{{ kw ? '没有匹配的文件' : '空目录' }}</div>
            <div v-if="perms.upload && !kw" class="mt-4px text-sm text-tertiary">把文件或文件夹拖到这里即可上传</div>
          </div>
        </template>
      </NDataTable>
    </div>

    <div v-if="dragging" class="fb__drop">
      <div class="fb__drop-inner">
        <Upload class="text-4xl" />
        <div class="mt-6px text-lg font-semibold">松开以上传到当前目录</div>
        <div class="rl-mono mt-2px text-secondary">{{ cwd }}</div>
        <div class="mt-2px text-sm text-tertiary">支持文件和文件夹；大文件自动分片、可断点续传</div>
      </div>
    </div>

    <UploadQueue :up="up" />

    <input ref="fileInput" type="file" multiple hidden @change="onPicked($event, false)">
    <input ref="dirInput" type="file" webkitdirectory multiple hidden @change="onPicked($event, true)">

    <NModal
      :show="!!nameDlg" preset="dialog" :title="nameDlg?.mode === 'mkdir' ? '新建目录' : '重命名'"
      positive-text="确定" negative-text="取消" :positive-button-props="{ loading: nameBusy }" :show-icon="false"
      @positive-click="onPositive" @negative-click="nameDlg = null" @close="nameDlg = null" @mask-click="nameDlg = null"
    >
      <NInput v-if="nameDlg" ref="nameInput" v-model:value="nameDlg.value" :placeholder="nameDlg.mode === 'mkdir' ? '目录名称' : '新名称'" class="mt-8px" @keyup.enter="submitName" />
    </NModal>
  </div>
</template>

<style scoped>
.fb { position: relative; display: flex; flex-direction: column; height: 100%; min-height: 0; background: var(--color-primary); outline: none; }
.fb__bar { display: flex; align-items: center; gap: 4px; padding: 8px 10px; border-bottom: 1px solid var(--color-border-default); }
.fb__crumbs { display: flex; align-items: center; gap: 2px; flex: 1; min-width: 0; overflow-x: auto; scrollbar-width: none; }
.fb__crumb {
  display: inline-flex; align-items: center; height: 28px; padding: 0 6px; border: 0; border-radius: 6px; background: transparent;
  color: var(--color-text-secondary); font: inherit; font-size: 13px; cursor: pointer; white-space: nowrap;
}
.fb__crumb:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.fb__crumb--cur { color: var(--color-text-primary); font-weight: 600; }
.fb__tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--color-border-default); }
.fb__filter { width: 160px !important; }
.fb__body { flex: 1; min-height: 0; }
.fb__drop { position: absolute; inset: 0; z-index: 6; padding: 12px; background: color-mix(in srgb, var(--color-primary) 80%, transparent); pointer-events: none; }
.fb__drop-inner {
  display: flex; height: 100%; flex-direction: column; align-items: center; justify-content: center; text-align: center;
  border: 2px dashed var(--color-text-info); border-radius: 12px; color: var(--color-text-primary);
}
.rl-icon-btn:disabled { opacity: .4; cursor: not-allowed; }
</style>
