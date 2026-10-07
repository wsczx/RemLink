<script setup lang="ts">
// 创建备份：选择备份范围（仅配置 / 全量），全量时可勾选是否包含日志、统计等大表
import { computed, ref, watch } from 'vue'
import { NButton, NCheckbox, NDrawer, NDrawerContent, NSkeleton } from 'naive-ui'
import Settings from '~icons/fluent/settings-20-regular'
import Database from '~icons/fluent/database-20-regular'
import { get, post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'

const show = defineModel<boolean>('show', { required: true })
const emit = defineEmits<{ created: [] }>()

const type = ref<'config' | 'full'>('full')
const tables = ref<any[]>([])
const loading = ref(false)
const saving = ref(false)

const types = [
  { value: 'config' as const, icon: Settings, title: '仅配置', desc: '只备份「服务配置」中的软件设置项，文件很小，适合改配置前留底。' },
  { value: 'full' as const, icon: Database, title: '全量备份', desc: '备份全部业务数据（用户、用户组、策略、证书等）和配置，用于迁移或灾备。' },
]

watch(show, async v => {
  if (!v) return
  type.value = 'full'
  loading.value = true
  try {
    const list = (await get('/api/v1/set/db/table_sizes')) || []
    // 业务表始终包含；日志/统计表默认不选
    tables.value = list.map((t: any) => ({ ...t, checked: t.group === 'business' }))
  } catch (e) {
    ui.message.error(errMsg(e, '获取表信息失败'))
    show.value = false
  } finally {
    loading.value = false
  }
})

const business = computed(() => tables.value.filter(t => t.group === 'business'))
const optional = computed(() => tables.value.filter(t => t.group !== 'business'))
const businessRows = computed(() => business.value.reduce((s, t) => s + (t.rows || 0), 0))
const groupLabel = (g: string) => (g === 'log' ? '日志' : '统计')

async function submit() {
  saving.value = true
  const payload: any = { type: type.value }
  if (type.value === 'full') payload.include_tables = tables.value.filter(t => t.checked).map(t => t.table)
  try {
    await post('/api/v1/set/db/backup', payload)
    ui.message.success('备份创建成功')
    show.value = false
    emit('created')
  } catch (e) {
    ui.message.error(errMsg(e, '备份创建失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NDrawer v-model:show="show" :width="600" :mask-closable="!saving">
    <NDrawerContent title="创建备份" :closable="!saving">
      <div class="mb-10px text-md font-semibold">备份范围</div>
      <div class="grid grid-cols-1 gap-10px sm:grid-cols-2">
        <button
          v-for="t in types" :key="t.value" type="button" class="bak-type"
          :class="type === t.value && 'bak-type--on'" @click="type = t.value"
        >
          <span class="flex items-center gap-8px text-md font-semibold">
            <component :is="t.icon" class="text-lg" />{{ t.title }}
            <span class="bak-type__radio ml-auto" />
          </span>
          <span class="mt-6px block text-sm leading-20px text-secondary">{{ t.desc }}</span>
        </button>
      </div>

      <template v-if="type === 'full'">
        <div class="mb-8px mt-24px flex items-baseline justify-between">
          <span class="text-md font-semibold">包含的数据表</span>
          <span class="text-sm text-tertiary">业务表 {{ business.length }} 张 · {{ businessRows.toLocaleString() }} 行，始终包含</span>
        </div>
        <div v-if="loading" class="py-10px"><NSkeleton text :repeat="4" /></div>
        <template v-else>
          <p class="m-0 mb-10px text-sm text-secondary">以下日志 / 统计表数据量可能很大，默认不备份；需要时勾选。</p>
          <div class="b-1px b-[var(--color-border-default)] b-solid rounded-10px">
            <label v-for="t in optional" :key="t.table" class="bak-row">
              <NCheckbox v-model:checked="t.checked" />
              <span class="min-w-0 flex-1">
                <span class="text-md">{{ t.name }}</span>
                <span class="rl-mono ml-6px text-sm! text-tertiary">{{ t.table }}</span>
              </span>
              <span class="ax-badge" :class="t.group === 'log' ? 'text-warning' : 'text-info'">{{ groupLabel(t.group) }}</span>
              <span class="w-90px text-right text-sm text-secondary tabular-nums">{{ (t.rows ?? 0).toLocaleString() }} 行</span>
            </label>
            <div v-if="!optional.length" class="px-14px py-12px text-sm text-tertiary">没有可选的大表</div>
          </div>
        </template>
      </template>

      <div class="mt-20px text-sm text-tertiary">备份文件保存在服务器 <span class="rl-kbd">conf/backup/</span> 目录。</div>

      <template #footer>
        <div class="flex gap-sm">
          <NButton :disabled="saving" @click="show = false">取消</NButton>
          <NButton type="primary" :loading="saving" :disabled="loading" @click="submit">开始备份</NButton>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.bak-type {
  display: block; padding: 14px 16px; text-align: left; border: 1px solid var(--color-border-default); border-radius: 10px;
  background: var(--color-primary); color: inherit; font: inherit; cursor: pointer; transition: border-color .15s;
}
.bak-type:hover { border-color: var(--color-border-strong); }
.bak-type--on { border-color: var(--color-action); box-shadow: 0 0 0 1px var(--color-action); }
.bak-type__radio { width: 16px; height: 16px; border-radius: 50%; border: 1px solid var(--color-border-strong); }
.bak-type--on .bak-type__radio { border: 5px solid var(--color-action); }
.bak-row { display: flex; align-items: center; gap: 10px; padding: 10px 14px; cursor: pointer; }
.bak-row + .bak-row { border-top: 1px solid var(--color-border-default); }
</style>
