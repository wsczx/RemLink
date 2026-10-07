<script setup lang="ts">
// 访问权限：所属用户组（认证方式、DNS、组策略）+ 个人策略
import { computed } from 'vue'
import PeopleTeam from '~icons/fluent/people-team-20-regular'
import PageHeader from '@/components/PageHeader.vue'
import RlPanel from '@/components/RlPanel.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusDot from '@/components/StatusDot.vue'
import PolicySummary from '../PolicySummary.vue'
import { usePortal } from '../store'

const p = usePortal()
const { user, cardVisible } = p

const groups = computed<any[]>(() => user.value.groups_detail || [])
const showGroups = computed(() => cardVisible('groups'))
const showPersonal = computed(() => cardVisible('personal_policy') && !!user.value.user_policy)
</script>

<template>
  <div>
    <PageHeader title="访问权限" description="你能访问哪些资源由所属用户组和个人策略决定，如需调整请联系管理员" />

    <div class="grid gap-16px">
      <!-- 个人策略：优先级高于组策略，放在最前 -->
      <RlPanel v-if="showPersonal" title="个人策略" :subtitle="user.user_policy.name">
        <template #extra><span class="ax-badge text-info">优先于组策略</span></template>
        <p v-if="user.user_policy.note" class="m-0 mb-12px text-md text-secondary">{{ user.user_policy.note }}</p>
        <PolicySummary :policy="user.user_policy" :traffic-used="user.traffic_used" show-exclude />
      </RlPanel>

      <RlPanel v-if="showGroups" title="用户组" :subtitle="`你属于 ${groups.length} 个用户组`" :padded="false">
        <EmptyState v-if="!groups.length" title="暂无分组信息" :icon="PeopleTeam" />
        <div v-for="g in groups" :key="g.name" class="grp-row grid gap-12px px-20px py-16px">
          <div class="flex items-center justify-between gap-12px">
            <div class="min-w-0">
              <div class="truncate text-md font-semibold">{{ g.name }}</div>
              <div v-if="g.note" class="truncate text-sm text-secondary">{{ g.note }}</div>
            </div>
            <StatusDot :tone="g.status === 1 ? 'success' : 'warning'">{{ g.status === 1 ? '正常' : '停用' }}</StatusDot>
          </div>
          <dl class="m-0 grid grid-cols-[72px_minmax(0,1fr)] gap-x-12px gap-y-8px text-md">
            <template v-if="(g.auth_types || []).length">
              <dt class="text-secondary">认证方式</dt>
              <dd class="m-0 flex flex-wrap items-center gap-4px">
                <template v-for="(at, i) in g.auth_types" :key="at">
                  <span class="rl-kbd">{{ at }}</span>
                  <span v-if="Number(i) < g.auth_types.length - 1" class="text-tertiary">→</span>
                </template>
              </dd>
            </template>
            <template v-if="(g.dns || []).length">
              <dt class="text-secondary">DNS 域</dt>
              <dd class="m-0 flex flex-wrap gap-4px"><span v-for="d in g.dns" :key="d" class="rl-kbd">{{ d }}</span></dd>
            </template>
            <template v-if="g.policy">
              <dt class="text-secondary">组策略</dt>
              <dd class="m-0 grid gap-6px">
                <span class="font-medium">{{ g.policy.name }}</span>
                <PolicySummary :policy="g.policy" :traffic-used="user.traffic_used" />
              </dd>
            </template>
          </dl>
        </div>
      </RlPanel>
    </div>
  </div>
</template>

<style scoped>
.grp-row + .grp-row { border-top: 1px solid var(--color-border-default); }
</style>
