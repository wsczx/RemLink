<script setup lang="ts">
// 认证流程编辑器：多步骤按序执行，可排序；LDAP / RADIUS / 企微 / 飞书 / 钉钉 步骤可关联认证源
import { computed, reactive } from 'vue'
import { NAlert, NButton, NSelect } from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import ArrowUp from '~icons/fluent/arrow-up-20-regular'
import ArrowDown from '~icons/fluent/arrow-down-20-regular'
import ChevronRight from '~icons/fluent/chevron-right-12-regular'
import { get } from '@/api/http'
import { ui } from '@/utils/ui'
import { AUTH_LABELS, AUTH_OPTIONS, PROVIDER_TYPES, authConflict, newStep, stepDefaults, type AuthStep } from './auth'

const props = defineProps<{ groupName?: string }>()
const steps = defineModel<AuthStep[]>({ required: true })

const providerNames = reactive<Record<string, string[]>>({})
const conflict = computed(() => authConflict(steps.value))

async function loadProviders(type: string, force = false) {
  if (!PROVIDER_TYPES.includes(type) || (!force && providerNames[type]?.length)) return
  try {
    const d = await get('/api/v1/provider/names', { type })
    providerNames[type] = d?.datas || []
  } catch { /* 忽略 */ }
}
function preload() {
  new Set(steps.value.map(s => s.type)).forEach(t => loadProviders(t))
}
defineExpose({ preload })

const providerOptions = (type: string) => (providerNames[type] || []).map(n => ({ label: n, value: n }))

async function onTypeChange(step: AuthStep, type: string) {
  step.type = type
  step.config = stepDefaults(type)
  step.provider = ''
  loadProviders(type)
  // 证书认证需该组已签发客户端证书
  if (type === 'cert' && props.groupName) {
    try {
      const d = await get('/api/v1/group/cert_check', { groupname: props.groupName })
      if (d?.cert_count === 0) {
        ui.message.warning(`当前组「${props.groupName}」尚未签发任何客户端证书，需在「系统设置 › 证书设置 › 客户端证书」中生成证书后再启用证书认证`, { duration: 6000 })
      }
    } catch { /* 忽略 */ }
  }
}

function move(i: number, d: number) {
  const j = i + d
  if (j < 0 || j >= steps.value.length) return
  const a = steps.value
  ;[a[i], a[j]] = [a[j], a[i]]
}
function remove(i: number) {
  if (steps.value.length <= 1) { ui.message.warning('至少保留一个认证步骤'); return }
  steps.value.splice(i, 1)
}
</script>

<template>
  <div class="grid gap-12px">
    <div class="af">
      <div v-for="(step, i) in steps" :key="step._key" class="af__row">
        <span class="af__num">{{ i + 1 }}</span>
        <NSelect :value="step.type" :options="AUTH_OPTIONS" class="w-150px! flex-shrink-0" @update:value="(v: string) => onTypeChange(step, v)" />
        <NSelect
          v-if="PROVIDER_TYPES.includes(step.type)" v-model:value="step.provider" :options="providerOptions(step.type)"
          placeholder="选择认证源" clearable filterable class="min-w-0 flex-1"
          @update:show="(v: boolean) => v && loadProviders(step.type)"
        />
        <span v-else class="flex-1 text-sm text-tertiary">无需认证源</span>
        <div class="flex flex-shrink-0">
          <button type="button" class="rl-icon-btn" :disabled="i === 0" title="上移" @click="move(i, -1)"><ArrowUp /></button>
          <button type="button" class="rl-icon-btn" :disabled="i === steps.length - 1" title="下移" @click="move(i, 1)"><ArrowDown /></button>
          <button type="button" class="rl-icon-btn af__del" title="删除" @click="remove(i)"><Delete /></button>
        </div>
      </div>
      <div class="af__foot">
        <NButton text size="small" @click="steps.push(newStep('local'))"><template #icon><Add /></template>添加认证步骤</NButton>
      </div>
    </div>

    <div v-if="steps.length" class="flex flex-wrap items-center gap-6px rounded-8px bg-fill px-12px py-10px text-md">
      <span class="mr-4px text-sm text-secondary">认证流程</span>
      <template v-for="(s, i) in steps" :key="'p' + s._key">
        <span class="font-semibold">{{ AUTH_LABELS[s.type] || s.type }}</span>
        <span v-if="s.provider" class="text-sm text-secondary">（{{ s.provider }}）</span>
        <ChevronRight v-if="i < steps.length - 1" class="text-tertiary" />
      </template>
    </div>

    <NAlert v-if="conflict" type="error" :bordered="false">{{ conflict }}</NAlert>

    <div class="text-sm text-secondary">
      认证步骤按序执行，前一步通过才进入下一步。常用组合：本地密码 → OTP、证书 → LDAP、RADIUS → OTP。
    </div>
  </div>
</template>

<style scoped>
.af { border: 1px solid var(--color-border-default); border-radius: 8px; }
.af__row { display: flex; align-items: center; gap: 8px; padding: 8px 12px; }
.af__row + .af__row { border-top: 1px solid var(--color-border-default); }
.af__num { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; flex-shrink: 0; border-radius: 50%; background: var(--color-action); color: var(--color-text-on-action); font-size: 12px; font-weight: 600; }
.af__foot { padding: 8px 12px; border-top: 1px solid var(--color-border-default); }
.af .rl-icon-btn:disabled { opacity: .35; cursor: not-allowed; }
.af__del:hover { color: var(--color-text-error); }
</style>
