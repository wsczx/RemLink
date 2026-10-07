<script setup lang="ts">
// 服务配置：左侧分组锚点导航（搜索 / 生效方式筛选），右侧分组 SettingRow；吸底保存条统一提交，支持重启服务
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave, RouterLink } from 'vue-router'
import { NAlert, NButton, NInput, NSkeleton } from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowReset from '~icons/fluent/arrow-counterclockwise-20-regular'
import ArrowRight from '~icons/fluent/arrow-right-20-regular'
import LockClosed from '~icons/fluent/lock-closed-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import EmptyState from '@/components/EmptyState.vue'
import SvcConfigControl from './components/SvcConfigControl.vue'
import SvcSaveBar from './components/SvcSaveBar.vue'
import { useServiceRestart, warnLevel, warnMsg, warnPath, type SysWarning } from './components/SvcRestart'
import { get, post, errMsg } from '@/api/http'
import { ui, confirm } from '@/utils/ui'

const IPV4_KEYS = ['ipv4_cidr', 'ipv4_gateway', 'ipv4_start', 'ipv4_end'] as const
const GROUP_ORDER = ['基础信息', '服务监听', '数据库', '虚拟网络', '连接控制', '日志/调试', '安全/认证', '防暴破', '锁定策略', '门户设置']
const IPV4_FIELDS = [
  { k: 'ipv4_cidr', l: '网段', p: '192.168.90.0/24' },
  { k: 'ipv4_gateway', l: '网关', p: '192.168.90.1' },
  { k: 'ipv4_start', l: '起始地址', p: '192.168.90.100' },
  { k: 'ipv4_end', l: '结束地址', p: '192.168.90.200' },
]

const items = ref<any[]>([])
const loading = ref(true)
const warnings = ref<SysWarning[]>([])
const keyword = ref('')
type Effect = 'all' | 'restart' | 'immediate' | 'dirty'
const effect = ref<Effect>('all')
const ipv4 = reactive<Record<string, string>>({ ipv4_cidr: '', ipv4_gateway: '', ipv4_start: '', ipv4_end: '' })
const ipv4Orig = reactive<Record<string, string>>({ ...ipv4 })
// 已保存但需重启才生效（本次会话内）
const pendingRestart = ref(false)

async function loadSoft() {
  try {
    const list: any[] = (await get('/api/v1/set/soft')) || []
    items.value = list.map(it => ({ ...it, edit_data: it.sensitive ? '' : it.data }))
    for (const k of IPV4_KEYS) {
      const it = list.find(i => i.name === k)
      ipv4[k] = ipv4Orig[k] = it ? String(it.data ?? '') : ''
    }
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
async function loadStatus() {
  try {
    warnings.value = (await get('/api/v1/set/soft/status'))?.warnings || []
  } catch (e) {
    ui.message.error(errMsg(e, '状态获取失败'))
  }
}
// 指向本页的告警不显示「去处理」
const warnLink = (w: SysWarning) => {
  const p = warnPath(w)
  return p && !/\/(set\/soft|settings\/service)$/.test(p) ? p : ''
}

// —— 脏检查 ——
const isIpv4 = (n: string) => (IPV4_KEYS as readonly string[]).includes(n)
const isDirty = (it: any) => !it.readonly && (it.sensitive ? it.edit_data !== '' && it.edit_data != null : it.edit_data !== it.data)
const dirtyItems = computed(() => items.value.filter(i => !isIpv4(i.name) && isDirty(i)))
const ipv4Dirty = computed(() => IPV4_KEYS.some(k => ipv4[k] !== ipv4Orig[k]))
const dirtyCount = computed(() => dirtyItems.value.length + (ipv4Dirty.value ? 1 : 0))
const dirtyRestart = computed(() => dirtyItems.value.some(i => i.restart) || ipv4Dirty.value)

// —— 分组 / 过滤 ——
const restartTotal = computed(() => items.value.filter(i => i.restart).length)
const effectOptions = computed(() => [
  { label: '全部', value: 'all' as Effect, count: items.value.length },
  { label: '需重启', value: 'restart' as Effect, count: restartTotal.value },
  { label: '即时生效', value: 'immediate' as Effect, count: items.value.length - restartTotal.value },
  ...(dirtyCount.value ? [{ label: '已修改', value: 'dirty' as Effect, count: dirtyCount.value }] : []),
])
watch(dirtyCount, n => { if (!n && effect.value === 'dirty') effect.value = 'all' })

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return items.value.filter(it => {
    if (effect.value === 'restart' && !it.restart) return false
    if (effect.value === 'immediate' && it.restart) return false
    if (effect.value === 'dirty' && !(isIpv4(it.name) ? ipv4Dirty.value : isDirty(it))) return false
    return !kw || [it.name, it.usage, it.group].some(v => String(v || '').toLowerCase().includes(kw))
  })
})
const groups = computed(() => {
  const m = new Map<string, any[]>()
  for (const it of filtered.value) {
    const g = it.group || '其他'
    if (!m.has(g)) m.set(g, [])
    m.get(g)!.push(it)
  }
  const names = [...GROUP_ORDER.filter(n => m.has(n)), ...[...m.keys()].filter(n => !GROUP_ORDER.includes(n)).sort()]
  return names.map(name => {
    const all = m.get(name)!
    const hasIpv4 = all.some(i => isIpv4(i.name))
    const rows = all.filter(i => !isIpv4(i.name))
    const dirty = rows.some(isDirty) || (hasIpv4 && ipv4Dirty.value)
    return { name, hasIpv4, rows, dirty, count: rows.length + (hasIpv4 ? 1 : 0) }
  })
})

// —— 锚点导航：滚动高亮当前分组 ——
const active = ref('')
const gid = (name: string) => `svc-grp-${encodeURIComponent(name)}`
let io: IntersectionObserver | null = null
const visible = new Map<string, boolean>()
function observe() {
  io?.disconnect()
  visible.clear()
  io = new IntersectionObserver(entries => {
    for (const e of entries) visible.set((e.target as HTMLElement).dataset.group!, e.isIntersecting)
    const first = groups.value.find(g => visible.get(g.name))
    if (first) active.value = first.name
  }, { rootMargin: '-72px 0px -55% 0px' })
  document.querySelectorAll<HTMLElement>('[data-group]').forEach(el => io!.observe(el))
}
watch(() => groups.value.map(g => g.name).join('|'), () => nextTick(observe))
function jump(name: string) {
  active.value = name
  document.getElementById(gid(name))?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// —— 保存 ——
const saving = ref(false)
async function save() {
  saving.value = true
  let restart = false
  let ok = 0
  const errors: string[] = []
  const failed = new Map<string, any>() // 保存失败的项保留编辑值
  try {
    if (ipv4Dirty.value) {
      try {
        const r = await post('/api/v1/set/soft/ipv4', { ...ipv4 })
        restart ||= !!r?.restart
        ok++
      } catch (e) {
        errors.push(`IPv4 地址池：${errMsg(e, '保存失败')}`)
        failed.set('__ipv4', { ...ipv4 })
      }
    }
    for (const it of dirtyItems.value) {
      try {
        const r = await post('/api/v1/set/soft/edit', { name: it.name, data: it.edit_data })
        restart ||= !!r?.restart
        ok++
      } catch (e) {
        errors.push(`${it.usage || it.name}：${errMsg(e, '保存失败')}`)
        failed.set(it.name, it.edit_data)
      }
    }
    if (errors.length) ui.message.error(errors.join('；'), { duration: 6000 })
    if (ok) {
      if (restart) {
        pendingRestart.value = true
        ui.message.warning(`已保存 ${ok} 项，部分配置需重启服务后生效`)
      } else ui.message.success(`已保存 ${ok} 项`)
    }
    await loadSoft()
    for (const it of items.value) if (failed.has(it.name)) it.edit_data = failed.get(it.name)
    if (failed.has('__ipv4')) Object.assign(ipv4, failed.get('__ipv4'))
    loadStatus()
  } finally {
    saving.value = false
  }
}
function reset() {
  for (const it of items.value) it.edit_data = it.sensitive ? '' : it.data
  Object.assign(ipv4, ipv4Orig)
}
function resetItem(it: any) { it.edit_data = it.sensitive ? '' : it.data }

const { restarting, restartService } = useServiceRestart()

onBeforeRouteLeave(async () => (dirtyCount.value ? await confirm(`有 ${dirtyCount.value} 项修改尚未保存，离开后将丢失。`, '放弃未保存的修改？') : true))
function beforeUnload(e: BeforeUnloadEvent) { if (dirtyCount.value) e.preventDefault() }

onMounted(async () => {
  window.addEventListener('beforeunload', beforeUnload)
  loadStatus()
  await loadSoft()
  nextTick(observe)
})
onBeforeUnmount(() => {
  io?.disconnect()
  window.removeEventListener('beforeunload', beforeUnload)
})
</script>

<template>
  <PageHeader title="服务配置" description="RemLink 服务的运行参数。标记「需重启」的配置保存后要重启服务才会生效。">
    <NButton :loading="restarting" @click="restartService">
      <template #icon><ArrowReset /></template>
      重启服务
    </NButton>
  </PageHeader>

  <div class="grid gap-16px">
    <!-- 状态：系统告警 + 待重启 -->
    <div v-if="warnings.length || pendingRestart" class="grid gap-8px">
      <div v-if="pendingRestart" class="svc-pending">
        <ArrowReset class="flex-shrink-0 text-lg text-warning" />
        <div class="min-w-0 flex-1">
          <div class="text-md font-semibold">有配置等待重启生效</div>
          <div class="text-sm text-secondary">刚才保存的部分配置需要重启服务才会生效，重启会短暂中断所有在线连接。</div>
        </div>
        <NButton size="small" quaternary @click="pendingRestart = false">稍后</NButton>
        <NButton size="small" type="warning" :loading="restarting" @click="restartService">立即重启</NButton>
      </div>
      <NAlert v-for="(w, i) in warnings" :key="i" :type="warnLevel(w)" :bordered="false">
        <div class="flex flex-wrap items-center gap-x-sm">
          <span class="min-w-0 flex-1">{{ warnMsg(w) }}</span>
          <RouterLink v-if="warnLink(w)" :to="warnLink(w)" class="text-sm font-semibold text-primary">去处理 →</RouterLink>
        </div>
      </NAlert>
    </div>

    <div class="grid grid-cols-1 items-start gap-24px lg:grid-cols-[220px_minmax(0,1fr)]">
      <!-- 本页导航 -->
      <aside class="svc-aside">
        <NInput v-model:value="keyword" placeholder="查找配置项" clearable size="small">
          <template #prefix><Search /></template>
        </NInput>
        <div class="mt-10px flex flex-wrap gap-4px">
          <button
            v-for="o in effectOptions" :key="o.value" type="button" class="svc-chip"
            :class="effect === o.value && 'svc-chip--on'" @click="effect = o.value"
          >
            {{ o.label }}<span class="tabular-nums opacity-60">{{ o.count }}</span>
          </button>
        </div>
        <nav class="mt-14px hidden lg:block">
          <div class="mb-4px px-10px text-sm text-tertiary">分组</div>
          <button
            v-for="g in groups" :key="g.name" type="button" class="svc-nav"
            :class="active === g.name && 'svc-nav--on'" @click="jump(g.name)"
          >
            <span class="truncate">{{ g.name }}</span>
            <span v-if="g.dirty" class="svc-nav__dirty" title="有未保存的修改" />
            <span class="ml-auto text-sm text-tertiary tabular-nums">{{ g.count }}</span>
          </button>
        </nav>
      </aside>

      <div class="grid min-w-0 gap-16px">
        <template v-if="loading">
          <div v-for="i in 3" :key="i" class="rl-panel p-20px"><NSkeleton text :repeat="4" /></div>
        </template>
        <EmptyState v-else-if="!groups.length" title="没有匹配的配置项" description="换个关键字，或把筛选切回「全部」">
          <NButton size="small" @click="keyword = ''; effect = 'all'">清除筛选</NButton>
        </EmptyState>

        <template v-else>
        <RlPanel
          v-for="g in groups" :id="gid(g.name)" :key="g.name" :data-group="g.name"
          :title="g.name" :padded="false" class="svc-group"
        >
          <template #extra><span class="text-sm text-tertiary">{{ g.count }} 项</span></template>

          <!-- IPv4 地址池：四个字段合并编辑、一次提交 -->
          <SettingRow v-if="g.hasIpv4" label="IPv4 地址池">
            <template #description>
              <div>分配给 VPN 客户端的虚拟网段、网关与地址范围</div>
              <div class="mt-6px flex flex-wrap items-center gap-6px">
                <span class="ax-badge text-warning">需重启</span>
                <span v-if="ipv4Dirty" class="ax-badge text-info">已修改</span>
              </div>
            </template>
            <div class="grid grid-cols-1 gap-10px sm:grid-cols-2">
              <label v-for="f in IPV4_FIELDS" :key="f.k" class="block">
                <span class="mb-4px block text-sm text-secondary">{{ f.l }}</span>
                <NInput v-model:value="ipv4[f.k]" :placeholder="f.p" class="rl-mono" :disabled="saving" />
              </label>
            </div>
          </SettingRow>

          <SettingRow v-for="it in g.rows" :key="it.name" :label="it.usage || it.name">
            <template #description>
              <span class="rl-mono text-sm! text-tertiary">{{ it.name }}</span>
              <div class="mt-6px flex flex-wrap items-center gap-6px">
                <span v-if="it.restart" class="ax-badge text-warning">需重启</span>
                <span v-if="it.readonly" class="ax-badge text-secondary"><LockClosed class="mr-2px" />只读</span>
                <span v-if="it.sensitive" class="ax-badge text-secondary">敏感</span>
                <button v-if="isDirty(it)" type="button" class="ax-badge svc-undo text-info" title="撤销此项修改" @click="resetItem(it)">已修改 · 撤销</button>
              </div>
            </template>
            <SvcConfigControl :item="it" :disabled="it.readonly || saving" />
            <RouterLink v-if="it.name === 'db_type'" to="/admin/settings/backup" class="svc-link">
              更换数据库请使用「数据备份」页的切换向导（含连通测试与数据迁移）<ArrowRight />
            </RouterLink>
          </SettingRow>
        </RlPanel>
        </template>
      </div>
    </div>
  </div>

  <SvcSaveBar
    :dirty="dirtyCount" :saving="saving" :text="`${dirtyCount} 项修改未保存`"
    :hint="dirtyRestart ? '其中包含需重启服务才生效的配置' : ''" @reset="reset" @save="save"
  />
</template>

<style scoped>
.svc-aside { position: sticky; top: 16px; }
@media (max-width: 1023px) { .svc-aside { position: static; } }
.svc-chip {
  display: inline-flex; align-items: center; gap: 4px; height: 26px; padding: 0 9px; border-radius: 13px;
  border: 1px solid var(--color-border-default); background: none; color: var(--color-text-secondary);
  font: inherit; font-size: 12px; cursor: pointer;
}
.svc-chip:hover { color: var(--color-text-primary); border-color: var(--color-border-strong); }
.svc-chip--on { background: var(--color-action); border-color: var(--color-action); color: var(--color-body); }
.svc-chip--on:hover { color: var(--color-body); }
.svc-nav {
  position: relative; display: flex; align-items: center; gap: 6px; width: 100%; height: 32px; padding: 0 10px;
  border: 0; border-radius: 8px; background: none; color: var(--color-text-secondary);
  font: inherit; font-size: 14px; cursor: pointer; text-align: left;
}
.svc-nav:hover { background: var(--color-action-hover); color: var(--color-text-primary); }
.svc-nav--on { color: var(--color-text-primary); font-weight: 600; background: var(--color-fill); }
.svc-nav--on::before {
  content: ''; position: absolute; left: 0; top: 8px; bottom: 8px; width: 2px; border-radius: 2px; background: var(--color-action);
}
.svc-nav__dirty { width: 6px; height: 6px; flex-shrink: 0; border-radius: 50%; background: var(--color-text-warning); }
.svc-group { scroll-margin-top: 16px; }
.svc-undo { border: 0; font: inherit; font-size: 12px; cursor: pointer; background: none; }
.svc-link { display: inline-flex; align-items: center; gap: 4px; margin-top: 8px; font-size: 12px; color: var(--color-text-secondary); text-decoration: none; }
.svc-link:hover { color: var(--color-text-primary); }
.svc-pending {
  display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 12px 16px;
  border: 1px solid var(--color-border-strong); border-radius: 12px; background: var(--color-primary);
}
</style>
