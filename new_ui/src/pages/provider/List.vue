<script setup lang="ts">
// 认证源：上半部分是已接入的认证源，下半部分是可接入类型的集成画廊
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NInput, NSpin } from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArrowRight from '~icons/fluent/arrow-right-20-regular'
import ChevronRight from '~icons/fluent/chevron-right-20-regular'
import Search from '~icons/fluent/search-20-regular'
import PlugDisconnected from '~icons/fluent/plug-disconnected-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import { errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import { formatDate } from '@/utils/format'
import { loadAllGroups, loadAllProviders } from './components/api'
import {
  PROVIDER_TYPES, connSummary, groupRefs, parseCfg, typeMeta,
  type GroupRef, type ProviderMeta, type ProviderRow,
} from './components/types'

const router = useRouter()
const route = useRoute()
const loading = ref(false)
const rows = ref<ProviderRow[]>([])
const refs = ref<Record<string, GroupRef[]>>({})
const kw = ref('')

async function load() {
  loading.value = true
  try {
    const [p, groups] = await Promise.all([loadAllProviders(), loadAllGroups()])
    rows.value = p.rows
    refs.value = groupRefs(groups)
  } catch (e) {
    ui.message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  await load()
  if (route.hash === '#pv-add') nextTick(scrollToAdd)
})

const list = computed(() => {
  const k = kw.value.trim().toLowerCase()
  return rows.value
    .map(r => {
      const cfg = parseCfg(r.config)
      return { ...r, meta: typeMeta(r.type), cfg, conn: connSummary(r.type, cfg), groups: refs.value[r.name] || [] }
    })
    .filter(r => !k || r.name.toLowerCase().includes(k) || (r.meta?.full || r.type).toLowerCase().includes(k) || r.conn.toLowerCase().includes(k))
})

// 顶部状态：启用数、未被引用（不会生效）数
const activeCount = computed(() => rows.value.filter(r => r.status === 1).length)
const unusedCount = computed(() => rows.value.filter(r => !(refs.value[r.name] || []).length).length)
const countByType = computed(() => {
  const m: Record<string, number> = {}
  rows.value.forEach(r => { m[r.type] = (m[r.type] || 0) + 1 })
  return m
})

const groupsOfKind = (kind: ProviderMeta['kind']) => PROVIDER_TYPES.filter(t => t.kind === kind)
const KINDS: { kind: ProviderMeta['kind']; title: string; desc: string }[] = [
  { kind: 'directory', title: '目录与 AAA', desc: '用户在客户端输入账号密码，由外部目录或 RADIUS 校验' },
  { kind: 'im', title: '办公平台扫码', desc: '用户在客户端弹出的页面中扫码登录，无需记忆 VPN 密码' },
]

// 相对时间：最近更新
function ago(v?: string) {
  if (!v) return '—'
  const t = new Date(v).getTime()
  if (Number.isNaN(t) || t <= 0) return '—'
  const s = Math.max(0, (Date.now() - t) / 1000)
  if (s < 60) return '刚刚'
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  if (s < 86400 * 30) return `${Math.floor(s / 86400)} 天前`
  return formatDate(v, 'YYYY-MM-DD')
}

const open = (id: number) => router.push(`/admin/provider/${id}`)
const create = (t: string) => router.push({ path: '/admin/provider/new', query: { type: t } })
const scrollToAdd = () => document.getElementById('pv-add')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
</script>

<template>
  <div class="pb-40px">
    <PageHeader title="认证源" description="让员工用企业目录或办公平台账号登录 VPN。接入后，在用户组的认证流程中引用即可生效。">
      <NButton @click="router.push('/admin/group/list')">用户组认证流程</NButton>
      <NButton type="primary" @click="scrollToAdd">
        <template #icon><Add /></template>添加认证源
      </NButton>
    </PageHeader>

    <div class="grid gap-32px">
      <!-- 已接入 -->
      <NSpin :show="loading">
        <RlPanel :padded="false">
          <template #header>
            <div class="flex min-w-0 items-center gap-10px">
              <span class="text-lg font-semibold">已接入</span>
              <span class="ax-badge text-secondary">{{ rows.length }}</span>
            </div>
          </template>
          <template #extra>
            <div v-if="rows.length" class="mr-6px hidden items-center gap-16px text-sm md:flex">
              <StatusDot tone="success">{{ activeCount }} 个启用</StatusDot>
              <StatusDot v-if="rows.length - activeCount" tone="muted">{{ rows.length - activeCount }} 个停用</StatusDot>
              <StatusDot v-if="unusedCount" tone="warning">{{ unusedCount }} 个未被用户组引用</StatusDot>
            </div>
            <NInput v-if="rows.length > 6" v-model:value="kw" size="small" placeholder="搜索名称 / 地址" clearable class="w-200px!">
              <template #prefix><Search /></template>
            </NInput>
            <button class="rl-icon-btn" title="刷新" @click="load"><ArrowClockwise /></button>
          </template>

          <EmptyState
            v-if="!loading && !rows.length" :icon="PlugDisconnected" title="还没有接入任何认证源"
            description="目前用户只能使用本地账号密码登录。从下方选择一种身份源开始接入。"
          >
            <NButton @click="scrollToAdd">选择认证源类型</NButton>
          </EmptyState>
          <div v-else-if="!list.length && kw" class="px-20px py-32px text-center text-md text-secondary">没有匹配「{{ kw }}」的认证源</div>

          <div v-else class="pv-list">
            <div class="pv-row pv-row--head">
              <span>名称</span><span>连接</span><span>用于用户组</span><span>状态</span><span>更新</span><span />
            </div>
            <button v-for="r in list" :key="r.id" type="button" class="pv-row pv-row--item" @click="open(r.id)">
              <span class="flex min-w-0 items-center gap-12px">
                <span class="pv-tile"><component :is="r.meta?.icon" /></span>
                <span class="min-w-0">
                  <span class="block truncate text-md font-semibold text-primary">{{ r.name }}</span>
                  <span class="block truncate text-sm text-secondary">{{ r.meta?.full || r.type }}</span>
                </span>
              </span>
              <span class="pv-col-conn min-w-0">
                <span v-if="r.conn" class="rl-mono block truncate text-sm text-secondary">{{ r.conn }}</span>
                <span v-else class="text-tertiary">—</span>
                <span v-if="r.cfg.sync_users" class="text-sm text-tertiary">自动同步用户</span>
              </span>
              <span class="pv-col-groups min-w-0">
                <span v-if="r.groups.length" class="block truncate text-md text-primary" :title="r.groups.map(g => g.name).join('、')">
                  {{ r.groups.slice(0, 2).map(g => g.name).join('、') }}<span v-if="r.groups.length > 2" class="text-secondary"> 等 {{ r.groups.length }} 个</span>
                </span>
                <span v-else class="ax-badge text-warning">未引用，暂不生效</span>
              </span>
              <span><StatusDot :tone="r.status === 1 ? 'success' : 'muted'">{{ r.status === 1 ? '启用' : '停用' }}</StatusDot></span>
              <span class="pv-col-time text-sm text-secondary" :title="formatDate(r.updated_at)">{{ ago(r.updated_at) }}</span>
              <ChevronRight class="pv-chev text-lg text-tertiary" />
            </button>
          </div>
        </RlPanel>
      </NSpin>

      <!-- 添加认证源 -->
      <section id="pv-add" class="scroll-mt-16px">
        <div class="mb-16px">
          <h2 class="m-0 text-xl font-semibold text-primary">添加认证源</h2>
          <p class="m-0 mt-4px text-md text-secondary">选择员工现在用来登录办公系统的那套账号，按向导完成接入。同一类型可以接入多个（例如多个 AD 域）。</p>
        </div>

        <div class="grid gap-24px">
          <div v-for="k in KINDS" :key="k.kind">
            <div class="mb-10px flex flex-wrap items-baseline gap-x-10px">
              <span class="text-md font-semibold text-primary">{{ k.title }}</span>
              <span class="text-sm text-secondary">{{ k.desc }}</span>
            </div>
            <div class="pv-gallery">
              <button v-for="t in groupsOfKind(k.kind)" :key="t.value" type="button" class="pv-card" @click="create(t.value)">
                <span class="flex items-start justify-between gap-8px">
                  <span class="pv-tile pv-tile--lg"><component :is="t.icon" /></span>
                  <StatusDot v-if="countByType[t.value]" tone="success">已接入 {{ countByType[t.value] }} 个</StatusDot>
                  <StatusDot v-else tone="muted">未配置</StatusDot>
                </span>
                <span class="mt-14px block text-lg font-semibold text-primary">{{ t.full }}</span>
                <span class="mt-4px block text-md text-secondary">{{ t.desc }}</span>
                <span class="mt-10px block text-sm text-tertiary">适用：{{ t.scene }}</span>
                <span class="mt-12px flex flex-wrap gap-6px">
                  <span v-for="c in t.caps" :key="c" class="rl-kbd">{{ c }}</span>
                </span>
                <span class="pv-card__cta">
                  {{ countByType[t.value] ? `再接入一个${t.label}` : `接入${t.label}` }}<ArrowRight class="pv-card__arrow" />
                </span>
              </button>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.pv-tile {
  display: inline-flex; flex-shrink: 0; align-items: center; justify-content: center;
  width: 36px; height: 36px; border-radius: 10px; font-size: 20px;
  background: var(--color-fill); color: var(--color-text-primary);
}
.pv-tile--lg { width: 44px; height: 44px; border-radius: 12px; font-size: 22px; }

/* 已接入列表 */
.pv-row {
  display: grid; align-items: center; gap: 16px;
  grid-template-columns: minmax(200px, 1.4fr) minmax(160px, 1.2fr) minmax(140px, 1fr) 80px 96px 20px;
  padding: 12px 20px; text-align: left;
}
.pv-row--head { padding-top: 10px; padding-bottom: 10px; font-size: 12px; color: var(--color-text-tertiary); border-bottom: 1px solid var(--color-border-default); }
.pv-row--item {
  width: 100%; border: 0; background: transparent; font: inherit; color: inherit; cursor: pointer;
  transition: background-color .15s;
}
.pv-row--item + .pv-row--item { border-top: 1px solid var(--color-border-default); }
.pv-row--item:hover { background: var(--color-action-hover); }
.pv-row--item:hover .pv-chev { color: var(--color-text-primary); }
.pv-col-conn { display: flex; flex-direction: column; }
@media (max-width: 900px) {
  .pv-row { grid-template-columns: minmax(0, 1fr) auto 20px; }
  .pv-row--head { display: none; }
  .pv-col-conn, .pv-col-groups, .pv-col-time { display: none; }
}

/* 类型画廊 */
.pv-gallery { display: grid; gap: 16px; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); }
.pv-card {
  position: relative; display: flex; flex-direction: column; padding: 20px; text-align: left;
  border: 1px solid var(--color-border-default); border-radius: 12px; background: var(--color-primary);
  font: inherit; color: inherit; cursor: pointer;
  transition: box-shadow .2s, transform .2s;
}
.pv-card:hover { box-shadow: 0 6px 20px -8px rgb(0 0 0 / .18); transform: translateY(-1px); }
.pv-card:focus-visible { outline: 2px solid var(--color-text-info); outline-offset: 2px; }
.pv-card__cta {
  display: inline-flex; align-items: center; gap: 6px; margin-top: auto; padding-top: 18px;
  font-size: 14px; font-weight: 600; color: var(--color-text-primary);
}
.pv-card__arrow { transition: transform .2s; }
.pv-card:hover .pv-card__arrow { transform: translateX(3px); }
</style>
