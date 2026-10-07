<script setup lang="ts">
// 访问详情抽屉：概要 → 回放 → 文件操作（远程会话）/ 请求（HTTP 访问）
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import { NButton, NDrawer, NDrawerContent, NPagination, NSpin, NSwitch } from 'naive-ui'
import Play from '~icons/fluent/play-20-regular'
import ArrowDownload from '~icons/fluent/arrow-download-20-regular'
import VideoOff from '~icons/fluent/video-off-20-regular'
import Info from '~icons/fluent/info-20-regular'
import Document from '~icons/fluent/document-20-regular'
import ArrowUpload from '~icons/fluent/arrow-upload-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import Rename from '~icons/fluent/rename-20-regular'
import FolderAdd from '~icons/fluent/folder-add-20-regular'
import FolderOpen from '~icons/fluent/folder-open-20-regular'
import StatusDot from '@/components/StatusDot.vue'
import CountryFlag from '@/components/CountryFlag.vue'
import { get, post, http, errMsg } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { formatBytes, formatDate } from '@/utils/format'
import { countryName } from '@/utils/net'
import { flagCode } from '@/pages/audit/components/cells'
import {
  type AuditReq, type FileLog, type Visit, FILE_OPS, PROTO_LABEL, downloadBlob, fmtDuration, hasScreen, isFileProto,
  protoIcon, recName, recReasonText, recUrl,
} from './visit'
// 播放器按需加载（xterm / guacamole-common-js 体积较大）
const CastPlayer = defineAsyncComponent(() => import('./CastPlayer.vue'))
const GuacPlayer = defineAsyncComponent(() => import('./GuacPlayer.vue'))

const props = defineProps<{ id: string | null }>()
const emit = defineEmits<{ close: []; changed: [] }>()

const loading = ref(false)
const visit = ref<Visit | null>(null)
const logs = ref<FileLog[]>([])
const logTotal = ref(0)
const logLimit = ref(0)
const showList = ref(false)
const reqs = ref<AuditReq[]>([])
const reqPage = ref(1)
const reqSize = 20
const reqTotal = ref(0)
const reqLoading = ref(false)

// 回放
const part = ref(1)
const cast = ref('')
const blob = ref<Blob | null>(null)
const recLoading = ref(false)
const recErr = ref('')

function resetRec() {
  part.value = 1
  cast.value = ''
  blob.value = null
  recErr.value = ''
}

async function load() {
  if (!props.id) return
  loading.value = true
  try {
    const d = await get('/api/v1/webvpn/visits/detail', { id: props.id, page: 1, page_size: reqSize })
    visit.value = d.visit
    logs.value = d.file_logs || []
    logTotal.value = d.file_log_total || 0
    logLimit.value = d.file_log_limit || 0
    reqs.value = d.requests || []
    reqTotal.value = d.count || 0
    reqPage.value = 1
  } catch (e) {
    ui.message.error(errMsg(e, '获取访问详情失败'))
    emit('close')
  } finally {
    loading.value = false
  }
}

watch(() => props.id, id => {
  visit.value = null
  logs.value = []
  reqs.value = []
  showList.value = false
  resetRec()
  if (id) load()
}, { immediate: true })

async function loadReqs(p: number) {
  if (!props.id) return
  reqLoading.value = true
  try {
    const d = await get('/api/v1/webvpn/visits/detail', { id: props.id, page: p, page_size: reqSize })
    reqs.value = d.requests || []
    reqPage.value = p
  } catch (e) {
    ui.message.error(errMsg(e, '获取请求失败'))
  } finally {
    reqLoading.value = false
  }
}

async function playRec(p = part.value) {
  const v = visit.value
  if (!v?.sid) return
  recLoading.value = true
  recErr.value = ''
  try {
    if (v.recording.format === 'guac') {
      const resp = await http.get(recUrl(v.sid, p), { responseType: 'blob' })
      const b = resp.data as Blob
      if ((await b.slice(0, 8).text()).startsWith('{"code"')) throw new Error(JSON.parse(await b.text()).msg || '录像不可用')
      part.value = p
      blob.value = b
    } else {
      const resp = await http.get(recUrl(v.sid), { responseType: 'text', transformResponse: x => x })
      const text = String(resp.data || '')
      if (text.startsWith('{"code"')) throw new Error(JSON.parse(text).msg || '录像不可用')
      cast.value = text
    }
  } catch (e) {
    recErr.value = errMsg(e, '加载录像失败')
  } finally {
    recLoading.value = false
  }
}
const playing = computed(() => !!cast.value || !!blob.value)

async function kick() {
  const v = visit.value
  if (!v) return
  if (v.kind === 'session') {
    if (!(await confirm(`确认断开「${v.username}」在「${v.app_name}」上的会话？终端与文件操作会立即中断。`, '断开会话'))) return
    try {
      const d = await post('/api/v1/webvpn/remote/kick', { sid: v.sid })
      ui.message.success(d?.kicked ? '已断开' : '会话已结束')
    } catch (e) {
      return ui.message.error(errMsg(e, '操作失败'))
    }
  } else {
    if (!(await confirm(`确认踢出「${v.username}」的全部 WebVPN 会话？该用户需要重新登录门户。`, '踢出用户'))) return
    try {
      await post('/api/v1/webvpn/session/kick', { username: v.username })
      ui.message.success('已踢出该用户的全部 WebVPN 会话')
    } catch (e) {
      return ui.message.error(errMsg(e, '操作失败'))
    }
  }
  emit('changed')
  load()
}

// 本次 HTTP 访问的逐请求导出（沿用审计导出接口）
const exporting = ref(false)
async function exportReqs() {
  const v = visit.value
  if (!v) return
  exporting.value = true
  try {
    await downloadBlob('/api/v1/webvpn/audit/export', {
      username: v.username, app_name: v.app_name,
      start_date: formatDate(v.started_at), end_date: formatDate(v.ended_at || Date.now()),
    }, `webvpn_audit_${v.username}_${formatDate(v.started_at, 'YYYYMMDD_HHmmss')}.csv`)
  } catch (e) {
    ui.message.error(errMsg(e, '导出失败'))
  } finally {
    exporting.value = false
  }
}

const OP_ICON: Record<string, any> = {
  list: FolderOpen, download: ArrowDownload, download_dir: ArrowDownload, upload: ArrowUpload,
  mkdir: FolderAdd, rename: Rename, delete: Delete,
}
const shownLogs = computed(() => (showList.value ? logs.value : logs.value.filter(l => l.op !== 'list')))
const listCount = computed(() => logs.value.filter(l => l.op === 'list').length)
const showFiles = computed(() => {
  const v = visit.value
  return !!v && v.kind === 'session' && (v.file_ops > 0 || isFileProto(v.protocol) || v.protocol === 'ssh' || logs.value.length > 0)
})

const statusTone = (c: number) => (c >= 500 ? 'text-error' : c >= 400 ? 'text-warning' : c >= 300 ? 'text-info' : 'text-secondary')
const recNote = computed(() => (visit.value ? recReasonText(visit.value) : ''))

const facts = computed(() => {
  const v = visit.value
  if (!v) return []
  const rows: { label: string; value: string; mono?: boolean }[] = [
    { label: '开始', value: formatDate(v.started_at) },
    v.online
      ? { label: v.kind === 'http' ? '最近请求' : '状态', value: v.kind === 'http' ? formatDate(v.last_active) : (v.terminals ? (hasScreen(v.protocol) ? '在线 · 终端 / 桌面已打开' : '在线') : '在线 · 后台保持') }
      : { label: '结束', value: formatDate(v.ended_at) },
    { label: '时长', value: fmtDuration(v.duration) },
    { label: '流量', value: `↑ ${formatBytes(v.bytes_in)}  ↓ ${formatBytes(v.bytes_out)}` },
  ]
  if (v.kind === 'session') {
    rows.push({ label: '目标', value: v.target, mono: true })
    if (!v.online) rows.push({ label: '断开原因', value: v.close_reason_text || '—' })
  } else {
    rows.push({ label: '请求', value: `${v.requests} 个${v.errors ? `，其中 ${v.errors} 个失败` : ''}` })
    if (v.target) rows.push({ label: '访问域名', value: v.target, mono: true })
  }
  return rows
})
</script>

<template>
  <NDrawer :show="!!id" :width="720" placement="right" @update:show="v => { if (!v) emit('close') }">
    <NDrawerContent closable :native-scrollbar="false" body-content-class="p-0!">
      <template #header>
        <div v-if="visit" class="flex min-w-0 items-center gap-10px">
          <component :is="protoIcon(visit.protocol)" class="flex-shrink-0 text-xl text-secondary" />
          <span class="truncate">{{ visit.username }} · {{ visit.app_name }}</span>
          <StatusDot v-if="visit.online" tone="success" pulse class="text-sm font-normal">在线</StatusDot>
        </div>
        <span v-else>访问详情</span>
      </template>

      <NSpin :show="loading && !visit">
        <div v-if="visit" class="grid gap-24px px-24px py-20px">
          <!-- 概要 -->
          <section>
            <div class="flex flex-wrap items-center gap-x-16px gap-y-6px text-md">
              <span class="ax-badge text-info">{{ PROTO_LABEL[visit.protocol] || visit.protocol.toUpperCase() }}</span>
              <span class="text-secondary">{{ visit.group || '无用户组' }}</span>
              <span class="inline-flex items-center gap-6px">
                <CountryFlag :code="flagCode(visit.client_ip, visit.country_code)" />
                <span class="rl-mono">{{ visit.client_ip }}</span>
                <span v-if="visit.country_code" class="text-sm text-tertiary">{{ countryName(visit.country_code) }}</span>
              </span>
              <div class="flex-1" />
              <NButton v-if="visit.online && visit.kind === 'session'" size="small" type="error" secondary @click="kick">断开</NButton>
              <NButton v-else-if="visit.kind === 'http'" size="small" quaternary type="error" @click="kick">踢出该用户</NButton>
            </div>
            <dl class="m-0 mt-16px grid grid-cols-2 gap-x-24px gap-y-10px">
              <div v-for="f in facts" :key="f.label" class="min-w-0">
                <dt class="text-sm text-tertiary">{{ f.label }}</dt>
                <dd class="m-0 truncate text-md" :class="f.mono && 'rl-mono'" :title="f.value">{{ f.value }}</dd>
              </div>
            </dl>
          </section>

          <!-- 回放 -->
          <section v-if="visit.kind === 'session'">
            <h3 class="m-0 mb-10px text-lg font-semibold">回放</h3>
            <template v-if="visit.recording.available">
              <div v-if="!playing" class="flex flex-wrap items-center gap-12px rounded-10px bg-fill px-16px py-14px">
                <NButton type="primary" :loading="recLoading" @click="playRec(1)">
                  <template #icon><Play /></template>播放录像
                </NButton>
                <span class="text-sm text-secondary">
                  {{ visit.recording.format === 'guac' ? '远程桌面录像' : '终端录像' }}<template v-if="visit.recording.parts > 1"> · 共 {{ visit.recording.parts }} 段</template>
                </span>
                <div class="flex-1" />
                <a :href="recUrl(visit.sid!)" :download="recName(visit.sid!, visit.recording.format)" class="inline-flex items-center gap-4px text-sm text-secondary hover:text-primary">
                  <ArrowDownload />下载
                </a>
              </div>
              <div v-if="recErr" class="mt-8px text-sm text-error">{{ recErr }}</div>
              <template v-if="cast">
                <CastPlayer :key="visit.sid" :cast="cast" />
              </template>
              <template v-else-if="blob">
                <div v-if="visit.recording.parts > 1" class="mb-8px flex flex-wrap items-center gap-6px text-sm">
                  <span class="text-secondary">分段</span>
                  <NButton
                    v-for="p in visit.recording.parts" :key="p" size="tiny" :secondary="p !== part" :type="p === part ? 'primary' : 'default'"
                    :loading="recLoading && p !== part" @click="playRec(p)"
                  >
                    {{ p }}
                  </NButton>
                </div>
                <GuacPlayer :key="`${visit.sid}:${part}`" :blob="blob" />
              </template>
              <div v-if="playing" class="mt-8px flex flex-wrap items-center gap-12px text-sm text-tertiary">
                <span class="flex-1">只记录屏幕输出，不记录键盘输入。空格 播放 / 暂停，← → 快退 / 快进 5 秒。</span>
                <a :href="recUrl(visit.sid!, part)" :download="recName(visit.sid!, visit.recording.format, part)" class="inline-flex items-center gap-4px text-secondary hover:text-primary">
                  <ArrowDownload />下载{{ visit.recording.parts > 1 ? '本段' : '' }}
                </a>
              </div>
              <div v-if="recNote" class="mt-8px flex items-center gap-6px text-sm text-tertiary"><Info />{{ recNote }}</div>
            </template>
            <div v-else class="flex items-start gap-8px text-md text-secondary">
              <VideoOff class="mt-2px flex-shrink-0 text-tertiary" />
              <span>
                {{ recNote }}
                <RouterLink v-if="visit.recording.reason === 'disabled'" to="/admin/webvpn" class="ml-4px text-info">前往应用设置</RouterLink>
              </span>
            </div>
          </section>

          <!-- 文件操作 -->
          <section v-if="showFiles">
            <div class="mb-10px flex items-center gap-12px">
              <h3 class="m-0 text-lg font-semibold">文件操作</h3>
              <span class="text-sm text-tertiary">{{ visit.file_ops }} 次</span>
              <div class="flex-1" />
              <label v-if="listCount" class="flex items-center gap-6px text-sm text-secondary">
                <NSwitch v-model:value="showList" size="small" />显示目录浏览（{{ listCount }}）
              </label>
            </div>
            <div v-if="!shownLogs.length" class="text-md text-tertiary">没有文件操作</div>
            <ol v-else class="m-0 list-none p-0">
              <li v-for="l in shownLogs" :key="l.id" class="vd-log" :class="l.op === 'list' && 'opacity-60'">
                <span class="rl-mono w-64px flex-shrink-0 text-sm! text-tertiary" :title="formatDate(l.created_at)">{{ formatDate(l.created_at, 'HH:mm:ss') }}</span>
                <span class="flex w-84px flex-shrink-0 items-center gap-6px" :class="l.result !== 'ok' ? 'text-error' : l.op === 'delete' ? 'text-warning' : ''">
                  <component :is="OP_ICON[l.op] || Document" class="flex-shrink-0" />{{ FILE_OPS[l.op] || l.op }}
                </span>
                <span class="min-w-0 flex-1">
                  <span class="rl-mono block truncate" :title="l.path">{{ l.path }}</span>
                  <span v-if="l.result !== 'ok'" class="block truncate text-sm text-error" :title="l.result">失败：{{ l.result }}</span>
                </span>
                <span class="w-72px flex-shrink-0 text-right text-sm tabular-nums text-secondary">{{ l.size ? formatBytes(l.size) : '' }}</span>
              </li>
            </ol>
            <div v-if="logTotal > logs.length" class="mt-8px text-sm text-tertiary">仅显示前 {{ logLimit }} 条，共 {{ logTotal }} 条</div>
          </section>

          <!-- HTTP 请求 -->
          <section v-if="visit.kind === 'http'">
            <div class="mb-10px flex items-center gap-12px">
              <h3 class="m-0 text-lg font-semibold">请求</h3>
              <span class="text-sm text-tertiary">{{ reqTotal }} 个</span>
              <div class="flex-1" />
              <NButton size="small" quaternary :loading="exporting" @click="exportReqs">
                <template #icon><ArrowDownload /></template>导出请求明细
              </NButton>
            </div>
            <NSpin :show="reqLoading">
              <ol class="m-0 list-none p-0">
                <li v-for="r in reqs" :key="r.id" class="vd-log">
                  <span class="rl-mono w-64px flex-shrink-0 text-sm! text-tertiary" :title="formatDate(r.created_at)">{{ formatDate(r.created_at, 'HH:mm:ss') }}</span>
                  <span class="rl-kbd w-60px flex-shrink-0 justify-center">{{ r.method }}</span>
                  <span class="rl-mono min-w-0 flex-1 truncate" :title="r.path">{{ r.path }}</span>
                  <span v-if="r.risk_level" class="ax-badge flex-shrink-0" :class="r.risk_level > 1 ? 'text-error' : 'text-warning'">{{ r.risk_level > 1 ? '高危' : '可疑' }}</span>
                  <span class="rl-mono w-40px flex-shrink-0 text-right font-semibold" :class="statusTone(r.status_code)">{{ r.status_code }}</span>
                  <span class="w-64px flex-shrink-0 text-right text-sm tabular-nums text-tertiary">{{ r.duration_ms }} ms</span>
                </li>
              </ol>
            </NSpin>
            <div v-if="reqTotal > reqSize" class="mt-12px flex justify-end">
              <NPagination :page="reqPage" :page-size="reqSize" :item-count="reqTotal" simple @update:page="loadReqs" />
            </div>
          </section>
        </div>
      </NSpin>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.vd-log { display: flex; align-items: center; gap: 12px; min-height: 36px; padding: 6px 0; font-size: 14px; }
.vd-log + .vd-log { border-top: 1px solid var(--color-border-default); }
</style>
