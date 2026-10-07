<script setup lang="ts">
// 客户端连接指引编辑：左侧平台列表，右侧所选平台的步骤（主从布局）
import { computed, ref, watch } from 'vue'
import { NButton, NInput } from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import Dismiss from '~icons/fluent/dismiss-20-regular'
import ArrowReset from '~icons/fluent/arrow-reset-20-regular'
import { defaultGuide, type GuideGroup } from './PortalDefs'
import { confirm } from '@/utils/ui'

const guide = defineModel<GuideGroup[]>({ required: true })
const sel = ref(0)
const cur = computed(() => guide.value[sel.value])
watch(() => guide.value.length, n => { if (sel.value >= n) sel.value = Math.max(0, n - 1) })

function addGroup() {
  guide.value.push({ name: '', steps: [''] })
  sel.value = guide.value.length - 1
}
async function removeGroup() {
  if (!cur.value) return
  if (!(await confirm(`删除「${cur.value.name || '未命名平台'}」及其全部步骤？`, '删除平台'))) return
  guide.value.splice(sel.value, 1)
}
function addStep() {
  if (!cur.value) return
  if (!cur.value.steps) cur.value.steps = []
  cur.value.steps.push('')
}
async function restore() {
  if (!(await confirm('用默认的 5 个平台指引覆盖当前内容？', '恢复默认指引', false))) return
  guide.value = defaultGuide()
  sel.value = 0
}
</script>

<template>
  <div class="pg">
    <!-- 平台列表 -->
    <div class="pg-list">
      <button
        v-for="(g, i) in guide" :key="i" type="button" class="pg-item" :class="i === sel && 'pg-item--active'" @click="sel = i"
      >
        <span class="truncate">{{ g.name || '未命名平台' }}</span>
        <span class="ml-auto flex-shrink-0 text-sm text-tertiary tabular-nums">{{ (g.steps || []).length }}</span>
      </button>
      <button type="button" class="pg-item pg-item--add" @click="addGroup"><Add />添加平台</button>
      <button type="button" class="pg-item pg-item--add" @click="restore"><ArrowReset />恢复默认</button>
    </div>

    <!-- 步骤 -->
    <div class="min-w-0 flex-1">
      <template v-if="cur">
        <div class="flex items-center gap-8px">
          <NInput v-model:value="cur.name" placeholder="平台名称，如 Windows" class="max-w-260px" />
          <div class="flex-1" />
          <NButton quaternary type="error" @click="removeGroup"><template #icon><Delete /></template>删除平台</NButton>
        </div>
        <ol class="m-0 mt-12px grid list-none gap-8px p-0">
          <li v-for="(_, si) in cur.steps" :key="si" class="flex items-start gap-8px">
            <span class="pg-num">{{ si + 1 }}</span>
            <NInput
              v-model:value="cur.steps[si]" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" class="rl-mono"
              placeholder="步骤说明，支持 HTML，可用 {{server_addr}} 占位"
            />
            <button class="rl-icon-btn flex-shrink-0 hover:text-error!" title="删除步骤" @click="cur.steps.splice(si, 1)"><Dismiss /></button>
          </li>
        </ol>
        <NButton size="small" dashed class="mt-10px" @click="addStep"><template #icon><Add /></template>添加步骤</NButton>
      </template>
      <div v-else class="py-24px text-center text-md text-tertiary">未配置任何平台，门户将展示默认指引。</div>
    </div>
  </div>
</template>

<style scoped>
.pg { display: flex; flex-direction: column; gap: 16px; }
@media (min-width: 900px) { .pg { flex-direction: row; } }
.pg-list { display: flex; flex-wrap: wrap; gap: 4px; flex-shrink: 0; }
@media (min-width: 900px) { .pg-list { flex-direction: column; flex-wrap: nowrap; width: 180px; } }
.pg-item {
  display: flex; align-items: center; gap: 6px; height: 34px; padding: 0 10px; border: 0; border-radius: 8px;
  background: transparent; color: var(--color-text-primary); font: inherit; font-size: 14px; cursor: pointer; text-align: left;
}
.pg-item:hover { background: var(--color-action-hover); }
.pg-item--active { background: var(--color-fill); font-weight: 600; }
.pg-item--add { color: var(--color-text-secondary); font-size: 13px; }
.pg-num {
  display: flex; align-items: center; justify-content: center; flex-shrink: 0; width: 22px; height: 22px; margin-top: 5px;
  border-radius: 50%; background: var(--color-fill); font-size: 12px; color: var(--color-text-secondary);
}
</style>
