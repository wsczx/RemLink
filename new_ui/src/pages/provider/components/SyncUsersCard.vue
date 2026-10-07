<script setup lang="ts">
// 同步用户：从认证源拉取用户加入指定用户组（后台异步执行）
// 后端要求目标用户组已启用且认证流程引用了该认证源，因此优先只列出引用它的组
import { computed, onMounted, ref, watch } from 'vue'
import { NButton, NSelect } from 'naive-ui'
import { RouterLink } from 'vue-router'
import { get, post, errMsg } from '@/api/http'
import { ui } from '@/utils/ui'
import type { GroupRef } from './types'

const props = defineProps<{ providerId: number; refs: GroupRef[] }>()

const loading = ref(false)
const group = ref<string | null>(null)
const allNames = ref<string[]>([])
const lastMsg = ref('')

// 没有任何组引用时（或用户组列表加载失败），回退为全部组名，由后端给出明确错误
const options = computed(() => props.refs.length
  ? props.refs.map(g => ({ label: g.status === 1 ? g.name : `${g.name}（已停用）`, value: g.name, disabled: g.status !== 1 }))
  : allNames.value.map(n => ({ label: n, value: n })))

async function loadNames() {
  if (props.refs.length || allNames.value.length) return
  try { allNames.value = (await get('/api/v1/group/names'))?.datas || [] } catch { /* 忽略 */ }
}
onMounted(loadNames)
watch(() => props.refs, loadNames)

async function submit() {
  if (!group.value) { ui.message.warning('请选择目标用户组'); return }
  loading.value = true
  try {
    const msg = await post('/api/v1/provider/sync_users', { id: props.providerId, group_name: group.value })
    lastMsg.value = typeof msg === 'string' && msg ? msg : '正在后台同步，请查看后台日志确认同步结果'
    ui.message.success(lastMsg.value)
  } catch (e) {
    ui.message.error(errMsg(e, '同步失败'))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <div class="mb-6px text-md">目标用户组</div>
    <NSelect v-model:value="group" filterable placeholder="同步的用户将加入该用户组" :options="options" />
    <div v-if="!refs.length" class="mt-6px text-sm text-warning">
      还没有用户组在认证流程中引用该认证源，同步会被拒绝。
      <RouterLink to="/admin/group/list" class="text-info">去配置用户组</RouterLink>
    </div>
    <div class="mt-12px flex flex-wrap items-center gap-10px">
      <NButton :loading="loading" @click="submit">立即同步</NButton>
      <span class="text-sm text-tertiary">在后台执行，结果见系统日志</span>
    </div>
    <div v-if="lastMsg" class="mt-12px rounded-8px bg-fill px-12px py-10px text-sm text-secondary">{{ lastMsg }}</div>
  </div>
</template>
