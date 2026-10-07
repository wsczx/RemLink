<script setup lang="ts">
// ACL 规则编辑器：自上而下匹配，支持上下移动排序
import { NButton, NInput, NSelect } from 'naive-ui'
import Add from '~icons/fluent/add-20-regular'
import Delete from '~icons/fluent/delete-20-regular'
import ArrowUp from '~icons/fluent/arrow-up-20-regular'
import ArrowDown from '~icons/fluent/arrow-down-20-regular'
import { newAcl, type AclRule } from './policy'

const rows = defineModel<AclRule[]>({ required: true })

const actionOptions = [
  { label: '允许', value: 'allow' },
  { label: '禁止', value: 'deny' },
]
const protoOptions = ['all', 'tcp', 'udp', 'icmp'].map(v => ({ label: v, value: v }))

function move(i: number, d: number) {
  const j = i + d
  if (j < 0 || j >= rows.value.length) return
  const arr = rows.value
  ;[arr[i], arr[j]] = [arr[j], arr[i]]
}
</script>

<template>
  <div class="acl">
    <div v-if="rows.length" class="acl__head">
      <span class="w-24px" />
      <span class="w-92px">动作</span>
      <span class="flex-[3]">目标 CIDR</span>
      <span class="w-92px">协议</span>
      <span class="flex-[2]">端口</span>
      <span class="flex-[2]">备注</span>
      <span class="w-96px" />
    </div>
    <div :class="rows.length > 10 && 'max-h-420px overflow-y-auto'">
      <div v-for="(item, i) in rows" :key="i" class="acl__row">
        <span class="w-24px text-right text-sm text-tertiary tabular-nums">{{ i + 1 }}</span>
        <NSelect
          v-model:value="item.action" :options="actionOptions" class="w-92px! flex-shrink-0"
          :class="item.action === 'deny' ? 'acl__deny' : 'acl__allow'"
        />
        <NInput v-model:value="item.val" placeholder="如 10.0.0.0/8" class="flex-[3] rl-mono" />
        <NSelect v-model:value="item.protocol" :options="protoOptions" filterable tag class="w-92px! flex-shrink-0" />
        <NInput v-model:value="item.port" placeholder="0 = 全部" class="flex-[2] rl-mono" />
        <NInput v-model:value="item.note" placeholder="备注" class="flex-[2]" />
        <div class="flex w-96px flex-shrink-0">
          <button type="button" class="rl-icon-btn" :disabled="i === 0" title="上移" @click="move(i, -1)"><ArrowUp /></button>
          <button type="button" class="rl-icon-btn" :disabled="i === rows.length - 1" title="下移" @click="move(i, 1)"><ArrowDown /></button>
          <button type="button" class="rl-icon-btn acl__del" title="删除" @click="rows.splice(i, 1)"><Delete /></button>
        </div>
      </div>
    </div>
    <div v-if="!rows.length" class="px-12px py-14px text-center text-sm text-tertiary">暂无规则：未匹配的流量默认拒绝</div>
    <div class="acl__foot">
      <NButton text size="small" @click="rows.push(newAcl())"><template #icon><Add /></template>添加 ACL 规则</NButton>
    </div>
  </div>
</template>

<style scoped>
.acl { border: 1px solid var(--color-border-default); border-radius: 8px; }
.acl__head { display: flex; align-items: center; gap: 8px; padding: 8px 12px 4px; font-size: 12px; color: var(--color-text-tertiary); }
.acl__row { display: flex; align-items: center; gap: 8px; padding: 4px 12px; }
.acl__foot { padding: 8px 12px; border-top: 1px solid var(--color-border-default); }
.acl .rl-icon-btn:disabled { opacity: .35; cursor: not-allowed; }
.acl__del:hover { color: var(--color-text-error); }
.acl__allow :deep(.n-base-selection-input__content) { color: var(--color-text-success); }
.acl__deny :deep(.n-base-selection-input__content) { color: var(--color-text-error); }
</style>
