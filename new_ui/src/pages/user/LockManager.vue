<script setup lang="ts">
// 锁定管理：登录失败触发的用户 / IP 锁定（防爆破），可手动解锁
import { computed, h, onMounted, ref } from 'vue'
import { NButton, NDataTable, NIcon, NInput, type DataTableColumns } from 'naive-ui'
import Search from '~icons/fluent/search-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import LockOpen from '~icons/fluent/lock-open-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import StatCard from '@/components/StatCard.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import { errMsg, get, post } from '@/api/http'
import { confirm, ui } from '@/utils/ui'
import { fmtTime, userLabel } from './components/shared'

interface LockRow {
  description: string
  username: string
  nickname?: string
  ip: string
  state: { locked: boolean; attempts: number; lock_time: string; lastAttempt: string }
}

const loading = ref(false)
const rows = ref<LockRow[]>([])
const keyword = ref('')

async function load() {
  loading.value = true
  try {
    rows.value = (await get<LockRow[]>('/api/v1/locksinfo/list')) || []
  } catch (e) {
    ui.message.error(errMsg(e, '无法获取锁定信息'))
  } finally {
    loading.value = false
  }
}

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  if (!k) return rows.value
  return rows.value.filter(r => [r.description, r.username, r.nickname, r.ip].some(s => (s || '').toLowerCase().includes(k)))
})

const stats = computed(() => ({
  total: rows.value.length,
  locked: rows.value.filter(l => l.state.locked).length,
  maxAttempts: Math.max(0, ...rows.value.map(l => l.state.attempts || 0)),
  users: new Set(rows.value.map(l => l.username).filter(Boolean)).size,
}))

async function unlock(r: LockRow) {
  const target = [r.username && `用户「${r.username}」`, r.ip && `IP ${r.ip}`].filter(Boolean).join(' / ') || r.description
  if (!(await confirm(`确定解锁 ${target} 吗？`, '解锁确认', false))) return
  try {
    await post('/api/v1/locksinfo/unlok', { state: { locked: false }, username: r.username, ip: r.ip, description: r.description })
    ui.message.success('解锁成功')
    load()
  } catch (e) {
    ui.message.error(errMsg(e, '解锁失败'))
  }
}

const muted = () => h('span', { class: 'text-quaternary' }, '—')
const columns: DataTableColumns<LockRow> = [
  {
    title: '锁定对象', key: 'description', minWidth: 200, sorter: (a, b) => (a.description || '').localeCompare(b.description || ''),
    render: r => h('span', { class: 'font-semibold' }, r.description || '—'),
  },
  {
    title: '用户', key: 'username', minWidth: 140, sorter: (a, b) => (a.username || '').localeCompare(b.username || ''),
    render: r => (r.username ? userLabel(r.username, r.nickname) : muted()),
  },
  {
    title: 'IP 地址', key: 'ip', width: 160, sorter: (a, b) => (a.ip || '').localeCompare(b.ip || ''),
    render: r => (r.ip ? h('span', { class: 'rl-mono' }, r.ip) : muted()),
  },
  {
    title: '状态', key: 'locked', width: 100, sorter: (a, b) => Number(b.state.locked) - Number(a.state.locked),
    render: r => h(StatusDot, { tone: r.state.locked ? 'error' : 'success' }, { default: () => (r.state.locked ? '已锁定' : '正常') }),
  },
  {
    title: '失败次数', key: 'attempts', width: 100, sorter: (a, b) => a.state.attempts - b.state.attempts,
    render: r => h('span', { class: ['tabular-nums font-semibold', r.state.attempts > 3 && 'text-error'] }, r.state.attempts),
  },
  {
    title: '锁定截止', key: 'lock_time', width: 170, sorter: (a, b) => (a.state.lock_time || '').localeCompare(b.state.lock_time || ''),
    render: r => h('span', { class: 'text-secondary tabular-nums' }, fmtTime(r.state.lock_time)),
  },
  {
    title: '最后尝试', key: 'lastAttempt', width: 170, sorter: (a, b) => (a.state.lastAttempt || '').localeCompare(b.state.lastAttempt || ''),
    render: r => h('span', { class: 'text-secondary tabular-nums' }, fmtTime(r.state.lastAttempt)),
  },
  {
    title: '', key: 'actions', width: 80, align: 'right',
    render: r => (r.state.locked
      ? h(NButton, { text: true, size: 'small', type: 'error', onClick: () => unlock(r) }, { default: () => '解锁' })
      : h('span', { class: 'text-sm text-quaternary' }, '—')),
  },
]

onMounted(load)
</script>

<template>
  <div>
    <PageHeader title="锁定管理" description="登录失败次数超过阈值时，系统会临时锁定对应用户或来源 IP。可在此查看并手动解锁。">
      <NButton :loading="loading" @click="load">
        <template #icon><NIcon><ArrowClockwise /></NIcon></template>
        刷新
      </NButton>
    </PageHeader>

    <div class="grid gap-16px">
      <div class="grid grid-cols-2 gap-16px lg:grid-cols-4">
        <StatCard label="锁定记录" :value="stats.total" />
        <StatCard label="当前锁定" :value="stats.locked" :tone="stats.locked ? 'error' : undefined" />
        <StatCard label="最大失败次数" :value="stats.maxAttempts" />
        <StatCard label="涉及用户" :value="stats.users" />
      </div>

      <RlPanel :padded="false">
        <template #toolbar>
          <NInput v-model:value="keyword" placeholder="搜索描述 / 用户 / IP" clearable class="w-240px!">
            <template #prefix><NIcon><Search /></NIcon></template>
          </NInput>
          <div class="flex-1" />
          <span class="text-sm text-secondary">共 {{ filtered.length }} 条</span>
          <button class="rl-icon-btn" title="刷新" @click="load"><ArrowClockwise /></button>
        </template>

        <NDataTable
          :columns="columns" :data="filtered" :loading="loading" :bordered="false"
          :row-key="(r: LockRow) => `${r.description}|${r.username}|${r.ip}`" size="small" :scroll-x="1040"
        >
          <template #empty>
            <EmptyState :icon="LockOpen" title="没有锁定记录" description="当前没有被锁定的用户或 IP。" />
          </template>
        </NDataTable>
      </RlPanel>
    </div>
  </div>
</template>
