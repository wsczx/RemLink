<script setup lang="ts">
// 系统信息：顶部「服务状态 + 版本更新」概览，其下资源占用、主机与构建信息；支持在线升级
import { computed, onMounted, ref } from 'vue'
import { NButton, NSkeleton } from 'naive-ui'
import { useIntervalFn } from '@vueuse/core'
import dayjs from 'dayjs'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArrowSync from '~icons/fluent/arrow-sync-20-regular'
import ArrowUpload from '~icons/fluent/arrow-circle-up-20-regular'
import Server from '~icons/fluent/server-20-regular'
import Board from '~icons/fluent/developer-board-20-regular'
import Memory from '~icons/fluent/memory-16-regular'
import HardDrive from '~icons/fluent/hard-drive-20-regular'
import Pulse from '~icons/fluent/pulse-20-regular'
import Copy from '~icons/fluent/copy-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import SysUpgradeModal from './components/SysUpgradeModal.vue'
import { get, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'

const sys = ref<any>(null)
const loading = ref(false)
const uptime = ref(0)

async function load() {
  loading.value = true
  try {
    sys.value = await get('/api/v1/set/system')
    uptime.value = sys.value?.sys?.uptime || 0
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}

// 运行时长本地每分钟递增，避免频繁请求
useIntervalFn(() => { if (uptime.value) uptime.value += 60 }, 60000)
const uptimeText = computed(() => {
  const s = Math.max(0, Math.floor(uptime.value))
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  return [d && `${d} 天`, h && `${h} 小时`, `${m} 分钟`].filter(Boolean).join(' ')
})

// —— 版本检查 ——
type UpdateState = 'unknown' | 'latest' | 'available'
const updateState = ref<UpdateState>('unknown')
const checking = ref(false)
const checkedAt = ref('')
const upgradeInfo = ref<any>(null)
const showUpgrade = ref(false)

async function checkUpdate() {
  checking.value = true
  try {
    const d = await get('/api/v1/set/upgrade/check')
    checkedAt.value = dayjs().format('HH:mm')
    if (d?.need_upgrade) {
      upgradeInfo.value = d
      updateState.value = 'available'
      showUpgrade.value = true
    } else {
      upgradeInfo.value = null
      updateState.value = 'latest'
      ui.message.success('当前已是最新版本')
    }
  } catch (e) {
    ui.message.error(errMsg(e, '检查更新失败'))
  } finally {
    checking.value = false
  }
}
const sourceName = computed(() => (upgradeInfo.value?.upgrade_source === 'github' ? 'GitHub' : upgradeInfo.value?.upgrade_source ? 'Gitee' : ''))

// —— 资源 ——
type Tone = 'success' | 'warning' | 'error'
const toneOf = (p: number): Tone => (p >= 90 ? 'error' : p >= 70 ? 'warning' : 'success')
const resources = computed(() => {
  const s = sys.value
  if (!s) return []
  return [
    { label: 'CPU', icon: Board, pct: +s.cpu?.percent || 0, sub: `${s.cpu?.core ?? '-'} 核 · ${s.cpu?.ghz || '-'}` },
    { label: '内存', icon: Memory, pct: +s.mem?.percent || 0, sub: `可用 ${s.mem?.free ?? '-'} · 共 ${s.mem?.total ?? '-'}` },
    { label: '磁盘', icon: HardDrive, pct: +s.disk?.percent || 0, sub: `可用 ${s.disk?.free ?? '-'} · 共 ${s.disk?.total ?? '-'}` },
  ]
})
// 系统负载：后端返回 "1 5 15" 空格分隔
const loads = computed(() => String(sys.value?.sys?.load || '').split(/\s+/).filter(Boolean))
const hottest = computed(() => resources.value.reduce((m, r) => Math.max(m, r.pct), 0))
const health = computed(() => {
  if (!sys.value) return { tone: 'muted' as const, text: '加载中…' }
  if (hottest.value >= 90) return { tone: 'error' as const, text: '资源紧张' }
  if (hottest.value >= 70) return { tone: 'warning' as const, text: '资源占用偏高' }
  return { tone: 'success' as const, text: '运行正常' }
})

const hostRows = computed(() => {
  const s = sys.value || {}
  return [
    { label: '主机名', value: s.sys?.hostname },
    { label: '操作系统', value: s.sys?.platform },
    { label: '内核版本', value: s.sys?.kernel, mono: true },
    { label: 'CPU 型号', value: s.cpu?.modelName },
    { label: '核心 / 主频', value: s.cpu ? `${s.cpu.core ?? '-'} 核 · ${s.cpu.ghz || '-'}` : '' },
  ]
})
const buildRows = computed(() => {
  const s = sys.value?.sys || {}
  return [
    { label: '版本', value: s.appVersion, mono: true },
    { label: 'Commit', value: s.appCommitId, mono: true, copy: true },
    { label: '构建时间', value: s.appBuildDate },
    { label: 'Go 版本', value: s.goVersion, mono: true },
    { label: '平台', value: s.goOs ? `${s.goOs}/${s.goArch}` : '', mono: true },
    { label: 'Goroutine', value: s.goroutine, mono: true },
  ]
})

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ui.message.success('已复制')
  } catch {
    ui.message.warning('复制失败，请手动选择复制')
  }
}

onMounted(load)
</script>

<template>
  <PageHeader title="系统信息" description="服务器运行状况、资源占用与软件版本">
    <button class="rl-icon-btn" title="刷新" :disabled="loading" @click="load"><ArrowClockwise :class="loading && 'animate-spin'" /></button>
  </PageHeader>

  <div class="grid gap-16px">
    <!-- 概览：服务状态 + 版本 -->
    <section class="rl-panel sys-hero">
      <div class="sys-hero__main">
        <div class="sys-hero__icon"><Server /></div>
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-x-sm gap-y-4px">
            <span class="text-xl font-semibold">{{ sys?.sys?.hostname || 'RemLink 服务' }}</span>
            <StatusDot :tone="health.tone" :pulse="health.tone === 'success'">{{ health.text }}</StatusDot>
          </div>
          <div class="mt-4px flex flex-wrap items-center gap-x-md gap-y-4px text-md text-secondary">
            <span>已运行 <b class="font-semibold text-primary">{{ sys ? uptimeText : '-' }}</b></span>
            <span v-if="sys?.sys?.platform">{{ sys.sys.platform }}</span>
            <span v-if="sys?.sys?.goOs" class="rl-mono">{{ sys.sys.goOs }}/{{ sys.sys.goArch }}</span>
          </div>
        </div>
      </div>

      <div class="sys-hero__ver">
        <div class="min-w-0 flex-1">
          <div class="text-sm text-secondary">RemLink 版本</div>
          <div class="mt-2px flex flex-wrap items-center gap-sm">
            <span class="rl-mono text-xl! font-semibold">{{ sys?.sys?.appVersion || '-' }}</span>
            <template v-if="updateState === 'available'">
              <span class="ax-badge text-info">可升级到 {{ upgradeInfo?.latest?.version }}</span>
              <span v-if="sourceName" class="text-sm text-tertiary">来自 {{ sourceName }}</span>
            </template>
            <span v-else-if="updateState === 'latest'" class="ax-badge text-success">已是最新</span>
          </div>
          <div class="mt-2px text-sm text-tertiary">
            {{ checkedAt ? `上次检查 ${checkedAt}` : '尚未检查更新' }}
          </div>
        </div>
        <NButton v-if="updateState === 'available'" type="primary" @click="showUpgrade = true">
          <template #icon><ArrowUpload /></template>
          查看并升级
        </NButton>
        <NButton v-else :loading="checking" @click="checkUpdate">
          <template #icon><ArrowSync /></template>
          检查更新
        </NButton>
      </div>
    </section>

    <!-- 资源占用 -->
    <RlPanel title="资源占用" subtitle="超过 70% 标为偏高，超过 90% 标为紧张" :padded="false">
      <div class="sys-meters">
        <template v-if="sys">
          <div v-for="r in resources" :key="r.label" class="sys-meter">
            <div class="flex items-center gap-6px text-md text-secondary">
              <component :is="r.icon" class="text-lg" />{{ r.label }}
            </div>
            <div class="mt-8px flex items-baseline gap-2px">
              <span class="text-[28px] font-semibold leading-34px tabular-nums" :class="r.pct >= 70 && `text-${toneOf(r.pct)}`">{{ r.pct }}</span>
              <span class="text-md text-secondary">%</span>
            </div>
            <div class="sys-bar"><div class="sys-bar__fill" :style="{ width: `${Math.min(100, r.pct)}%`, background: `var(--color-text-${toneOf(r.pct)})` }" /></div>
            <div class="mt-6px truncate text-sm text-tertiary" :title="r.sub">{{ r.sub }}</div>
          </div>
          <div class="sys-meter">
            <div class="flex items-center gap-6px text-md text-secondary"><Pulse class="text-lg" />系统负载</div>
            <div class="mt-8px grid grid-cols-3 gap-8px">
              <div v-for="(n, i) in ['1 分钟', '5 分钟', '15 分钟']" :key="n">
                <div class="text-xl font-semibold tabular-nums">{{ loads[i] ?? '-' }}</div>
                <div class="text-sm text-tertiary">{{ n }}</div>
              </div>
            </div>
          </div>
        </template>
        <template v-else>
          <div v-for="i in 4" :key="i" class="sys-meter"><NSkeleton text :repeat="3" /></div>
        </template>
      </div>
    </RlPanel>

    <div class="grid grid-cols-1 items-start gap-16px lg:grid-cols-2">
      <RlPanel title="主机" subtitle="服务器与处理器" :padded="false">
        <dl class="sys-kv">
          <div v-for="r in hostRows" :key="r.label" class="sys-kv__row">
            <dt>{{ r.label }}</dt>
            <dd :class="r.mono && 'rl-mono'" :title="String(r.value ?? '')">{{ r.value || '-' }}</dd>
          </div>
        </dl>
      </RlPanel>
      <RlPanel title="构建" subtitle="RemLink 构建与 Go 运行时" :padded="false">
        <dl class="sys-kv">
          <div v-for="r in buildRows" :key="r.label" class="sys-kv__row">
            <dt>{{ r.label }}</dt>
            <dd :class="r.mono && 'rl-mono'" :title="String(r.value ?? '')">
              <span class="truncate">{{ r.value ?? '-' }}</span>
              <button v-if="r.copy && r.value" class="rl-icon-btn sys-kv__copy" title="复制" @click="copy(String(r.value))"><Copy /></button>
            </dd>
          </div>
        </dl>
      </RlPanel>
    </div>
  </div>

  <SysUpgradeModal v-model:show="showUpgrade" :info="upgradeInfo" :current-version="sys?.sys?.appVersion" />
</template>

<style scoped>
.sys-hero { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 420px); }
.sys-hero__main { display: flex; align-items: center; gap: 16px; padding: 20px 24px; min-width: 0; }
.sys-hero__icon {
  display: flex; flex-shrink: 0; align-items: center; justify-content: center; width: 48px; height: 48px;
  border-radius: 12px; background: var(--color-fill); font-size: 24px; color: var(--color-text-secondary);
}
.sys-hero__ver {
  display: flex; align-items: center; gap: 16px; padding: 20px 24px; min-width: 0;
  border-left: 1px solid var(--color-border-default); background: var(--color-secondary);
  border-radius: 0 12px 12px 0;
}
@media (max-width: 1000px) {
  .sys-hero { grid-template-columns: 1fr; }
  .sys-hero__ver { border-left: 0; border-top: 1px solid var(--color-border-default); border-radius: 0 0 12px 12px; }
}

.sys-meters { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); }
.sys-meter { padding: 18px 20px; min-width: 0; }
.sys-meter + .sys-meter { border-left: 1px solid var(--color-border-default); }
@media (max-width: 1000px) {
  .sys-meters { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .sys-meter:nth-child(3) { border-left: 0; }
  .sys-meter:nth-child(n + 3) { border-top: 1px solid var(--color-border-default); }
}
.sys-bar { margin-top: 10px; height: 6px; border-radius: 3px; background: var(--color-fill); overflow: hidden; }
.sys-bar__fill { height: 100%; border-radius: 3px; transition: width .4s; }

.sys-kv { margin: 0; }
.sys-kv__row {
  display: grid; grid-template-columns: 110px minmax(0, 1fr); gap: 12px; align-items: center;
  min-height: 44px; padding: 0 20px; font-size: 14px;
}
.sys-kv__row + .sys-kv__row { border-top: 1px solid var(--color-border-default); }
.sys-kv__row dt { color: var(--color-text-secondary); }
.sys-kv__row dd { display: flex; align-items: center; justify-content: flex-end; gap: 4px; min-width: 0; margin: 0; font-weight: 500; text-align: right; }
.sys-kv__copy { width: 26px; height: 26px; font-size: 16px; }
</style>
