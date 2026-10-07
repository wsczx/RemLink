<script setup lang="ts">
// 数据备份：顶部状态（最近备份 / 存储 / 当前数据库），备份文件列表（还原 / 删除），创建备份抽屉；
// 数据库切换向导也放在本页——切换前需要备份、切换后从备份还原，都在这里完成
import { computed, h, onMounted, ref } from 'vue'
import { NButton, NDataTable, type DataTableColumns } from 'naive-ui'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'
import Add from '~icons/fluent/add-20-regular'
import ArrowClockwise from '~icons/fluent/arrow-clockwise-20-regular'
import ArchiveIcon from '~icons/fluent/archive-20-regular'
import DatabaseArrow from '~icons/fluent/database-arrow-right-20-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import SettingRow from '@/components/SettingRow.vue'
import StatusDot from '@/components/StatusDot.vue'
import EmptyState from '@/components/EmptyState.vue'
import BakCreateDrawer from './components/BakCreateDrawer.vue'
import BakDbSwitchDrawer from './components/BakDbSwitchDrawer.vue'
import { useServiceRestart } from './components/SvcRestart'
import { get, post, errMsg } from '@/api/http'
import { ui, confirm } from '@/utils/ui'
import { formatBytes, formatDate } from '@/utils/format'

dayjs.extend(relativeTime)

const backups = ref<any[]>([])
const loading = ref(false)
const kind = ref<'all' | 'full' | 'config'>('all')

async function load() {
  loading.value = true
  try {
    const list: any[] = (await get('/api/v1/set/db/backups')) || []
    backups.value = list.sort((a, b) => String(b.mod_time).localeCompare(String(a.mod_time)))
  } catch (e) {
    ui.message.error(errMsg(e, '获取备份列表失败'))
  } finally {
    loading.value = false
  }
}

const latest = computed(() => backups.value[0])
const totalSize = computed(() => backups.value.reduce((s, b) => s + (b.size || 0), 0))
const ageDays = computed(() => (latest.value ? dayjs().diff(dayjs(latest.value.mod_time), 'day') : Infinity))
const fresh = computed(() => {
  if (!latest.value) return { tone: 'warning' as const, text: '还没有任何备份' }
  if (ageDays.value > 30) return { tone: 'warning' as const, text: `${ageDays.value} 天未备份` }
  return { tone: 'success' as const, text: '已有近期备份' }
})
const rel = (t: string) => dayjs(t).locale('zh-cn').fromNow()
const typeName = (t: string) => (t === 'full' ? '全量' : '仅配置')
const rows = computed(() => (kind.value === 'all' ? backups.value : backups.value.filter(b => (kind.value === 'full') === (b.type === 'full'))))
const kinds = computed(() => [
  { label: '全部', value: 'all' as const, count: backups.value.length },
  { label: '全量', value: 'full' as const, count: backups.value.filter(b => b.type === 'full').length },
  { label: '仅配置', value: 'config' as const, count: backups.value.filter(b => b.type !== 'full').length },
])

// —— 还原 / 删除 ——
async function restore(row: any) {
  const scope = row.type === 'full' ? '数据库中的业务数据和配置' : '当前的软件配置项'
  if (!(await confirm(`将用备份「${row.name}」覆盖${scope}，覆盖后无法撤销。建议先创建一份当前数据的备份。`, '从备份还原'))) return
  try {
    const r = (await post('/api/v1/set/db/restore', { file: row.name })) || {}
    ui.message.success('还原成功')
    if (r.needs_restart) ui.message.warning('监听地址、证书等启动期配置需重启服务后生效', { duration: 6000 })
    switchedTip.value = false
    load()
    loadDb()
  } catch (e) {
    ui.message.error(errMsg(e, '还原失败'))
  }
}
async function remove(row: any) {
  if (!(await confirm(`备份文件「${row.name}」将从服务器删除，无法恢复。`, '删除备份'))) return
  try {
    await post('/api/v1/set/db/backup/delete', { file: row.name })
    ui.message.success('已删除')
    load()
  } catch (e) {
    ui.message.error(errMsg(e, '删除失败'))
  }
}

const columns: DataTableColumns<any> = [
  {
    title: '备份文件', key: 'name', minWidth: 300,
    render: r => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'rl-mono truncate font-semibold', title: r.name }, r.name),
      h('div', { class: 'mt-2px text-sm text-secondary' }, r === latest.value ? '最近一次备份' : typeName(r.type) + '备份'),
    ]),
  },
  {
    title: '范围', key: 'type', width: 100,
    render: r => h('span', { class: ['ax-badge', r.type === 'full' ? 'text-info' : 'text-secondary'] }, typeName(r.type)),
  },
  { title: '大小', key: 'size', width: 110, sorter: (a, b) => (a.size || 0) - (b.size || 0), render: r => h('span', { class: 'tabular-nums' }, formatBytes(r.size)) },
  {
    title: '创建时间', key: 'mod_time', width: 200, sorter: (a, b) => String(a.mod_time).localeCompare(String(b.mod_time)),
    render: r => h('div', null, [h('div', null, formatDate(r.mod_time)), h('div', { class: 'text-sm text-tertiary' }, rel(r.mod_time))]),
  },
  {
    title: '', key: 'actions', width: 130, align: 'right',
    render: r => h('div', { class: 'flex justify-end gap-14px' }, [
      h(NButton, { text: true, onClick: () => restore(r) }, () => '还原'),
      h(NButton, { text: true, type: 'error', onClick: () => remove(r) }, () => '删除'),
    ]),
  },
]

// —— 创建 ——
const showCreate = ref(false)

// —— 数据库 / 切换向导 ——
const dbType = ref('')
const dbSource = ref('')
const DB_NAMES: Record<string, string> = { sqlite3: 'SQLite', mysql: 'MySQL', postgres: 'PostgreSQL', mssql: 'SQL Server' }
const dbName = computed(() => DB_NAMES[dbType.value] || dbType.value || '-')
async function loadDb() {
  try {
    const list: any[] = (await get('/api/v1/set/soft')) || []
    dbType.value = String(list.find(i => i.name === 'db_type')?.data ?? '')
    dbSource.value = String(list.find(i => i.name === 'db_source')?.data ?? '')
  } catch { /* 仅用于展示 */ }
}
const showSwitch = ref(false)
const { restarting, waitForRestart } = useServiceRestart()

// 数据库切换后首次进入：提示从备份还原
const switchedTip = ref(false)
function checkSwitched() {
  let flag: string | null = null
  try { flag = localStorage.getItem('remlink_db_switched') } catch { /* ignore */ }
  if (flag !== '1') return
  try { localStorage.removeItem('remlink_db_switched') } catch { /* ignore */ }
  switchedTip.value = true
}

onMounted(async () => {
  checkSwitched()
  loadDb()
  await load()
  // 切换后没有备份文件则无需提示
  if (switchedTip.value && !backups.value.length) switchedTip.value = false
})
</script>

<template>
  <PageHeader title="数据备份" description="在服务器上保存数据库快照，用于改配置前留底、迁移服务器或更换数据库。">
    <NButton type="primary" @click="showCreate = true">
      <template #icon><Add /></template>
      创建备份
    </NButton>
  </PageHeader>

  <div class="grid gap-16px">
    <!-- 数据库切换后的还原提示 -->
    <div v-if="switchedTip" class="bak-tip">
      <DatabaseArrow class="flex-shrink-0 text-xl text-info" />
      <div class="min-w-0 flex-1">
        <div class="text-md font-semibold">数据库已切换到新库</div>
        <div class="text-sm text-secondary">如果切换时选择了「仅备份不迁移」，可以在下方列表中选择对应的全量备份，点击「还原」导入新库。</div>
      </div>
      <button class="rl-icon-btn" title="关闭" @click="switchedTip = false"><Dismiss /></button>
    </div>

    <!-- 状态概览 -->
    <div class="rl-panel bak-stat">
      <div class="bak-stat__cell">
        <div class="text-sm text-secondary">备份状态</div>
        <div class="mt-6px"><StatusDot :tone="fresh.tone">{{ fresh.text }}</StatusDot></div>
        <div class="mt-4px text-sm text-tertiary">
          {{ latest ? `最近：${rel(latest.mod_time)} · ${typeName(latest.type)} · ${formatBytes(latest.size)}` : '建议在修改重要配置前先创建一份全量备份' }}
        </div>
      </div>
      <div class="bak-stat__cell">
        <div class="text-sm text-secondary">已保存</div>
        <div class="mt-4px text-xl font-semibold tabular-nums">{{ backups.length }} <span class="text-md font-normal text-secondary">个文件</span></div>
        <div class="mt-2px text-sm text-tertiary">共 {{ formatBytes(totalSize) }} · <span class="rl-mono">conf/backup/</span></div>
      </div>
      <div class="bak-stat__cell">
        <div class="text-sm text-secondary">当前数据库</div>
        <div class="mt-4px text-xl font-semibold">{{ dbName }}</div>
        <div class="rl-mono mt-2px truncate text-sm! text-tertiary" :title="dbSource">{{ dbSource || '-' }}</div>
      </div>
    </div>

    <!-- 备份列表 -->
    <RlPanel :padded="false">
      <template #toolbar>
        <div class="flex gap-4px">
          <button
            v-for="k in kinds" :key="k.value" type="button" class="bak-chip"
            :class="kind === k.value && 'bak-chip--on'" @click="kind = k.value"
          >{{ k.label }}<span class="opacity-60 tabular-nums">{{ k.count }}</span></button>
        </div>
        <div class="flex-1" />
        <button class="rl-icon-btn" title="刷新" @click="load"><ArrowClockwise /></button>
      </template>
      <NDataTable
        v-if="rows.length || loading" :columns="columns" :data="rows" :loading="loading" :bordered="false"
        :row-key="(r: any) => r.name" size="small"
      />
      <EmptyState v-else :icon="ArchiveIcon" :title="backups.length ? '没有该类型的备份' : '还没有备份'" description="备份文件保存在服务器 conf/backup/ 目录，可随时从这里还原。">
        <NButton v-if="!backups.length" @click="showCreate = true">创建第一份备份</NButton>
      </EmptyState>
    </RlPanel>

    <!-- 更换数据库 -->
    <RlPanel title="更换数据库" subtitle="把 RemLink 迁移到另一种或另一台数据库" :padded="false">
      <SettingRow label="当前数据库" description="连接信息只读，只能通过切换向导修改">
        <div class="flex items-center gap-sm">
          <span class="ax-badge text-info">{{ dbName }}</span>
          <span class="rl-mono min-w-0 truncate text-secondary" :title="dbSource">{{ dbSource || '-' }}</span>
        </div>
      </SettingRow>
      <SettingRow label="切换数据库">
        <template #description>
          向导会依次完成：填写新连接 → 测试连通 → 选择迁移方式（自动迁移 / 仅备份 / 跳过）→ 确认。
          <b class="text-primary">执行后服务会自动重启</b>，所有在线连接中断。
        </template>
        <NButton :loading="restarting" @click="showSwitch = true">
          <template #icon><DatabaseArrow /></template>
          {{ restarting ? '等待服务重启…' : '打开切换向导' }}
        </NButton>
      </SettingRow>
    </RlPanel>
  </div>

  <BakCreateDrawer v-model:show="showCreate" @created="load" />
  <BakDbSwitchDrawer v-model:show="showSwitch" :current-type="dbType" :current-source="dbSource" @switched="waitForRestart" />
</template>

<style scoped>
.bak-stat { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
.bak-stat__cell { min-width: 0; padding: 16px 20px; font-size: 14px; }
.bak-stat__cell + .bak-stat__cell { border-left: 1px solid var(--color-border-default); }
@media (max-width: 860px) {
  .bak-stat { grid-template-columns: 1fr; }
  .bak-stat__cell + .bak-stat__cell { border-left: 0; border-top: 1px solid var(--color-border-default); }
}
.bak-tip {
  display: flex; align-items: center; gap: 12px; padding: 12px 12px 12px 16px;
  border: 1px solid var(--color-border-strong); border-radius: 12px; background: var(--color-primary);
}
.bak-chip {
  display: inline-flex; align-items: center; gap: 4px; height: 28px; padding: 0 10px; border-radius: 14px;
  border: 1px solid var(--color-border-default); background: none; color: var(--color-text-secondary);
  font: inherit; font-size: 13px; cursor: pointer;
}
.bak-chip:hover { color: var(--color-text-primary); border-color: var(--color-border-strong); }
.bak-chip--on, .bak-chip--on:hover { background: var(--color-action); border-color: var(--color-action); color: var(--color-body); }
</style>
