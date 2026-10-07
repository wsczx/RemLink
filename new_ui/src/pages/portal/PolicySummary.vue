<script setup lang="ts">
import { computed } from 'vue'
import { formatBandwidth, formatTraffic, resetLabel } from './shared'

// 策略摘要：局域网 / 带宽 / 流量配额 / 路由 / ACL，以 key-value 小标签横排
const props = defineProps<{ policy: any; trafficUsed?: number; showExclude?: boolean }>()

const items = computed(() => {
  const p = props.policy || {}
  const list: { k: string; v: string }[] = []
  if (p.allow_lan) list.push({ k: '本地局域网', v: '允许' })
  if (p.bandwidth > 0) list.push({ k: '下行', v: formatBandwidth(p.bandwidth) })
  if (p.bandwidth_up > 0) list.push({ k: '上行', v: formatBandwidth(p.bandwidth_up) })
  if (p.traffic_quota > 0) {
    list.push({ k: '流量', v: `${formatTraffic(props.trafficUsed || 0)} / ${formatTraffic(p.traffic_quota)}${resetLabel(p.traffic_reset)}` })
  }
  if (p.route_include > 0) list.push({ k: props.showExclude ? '包含路由' : '路由', v: `${p.route_include} 条` })
  if (props.showExclude && p.route_exclude > 0) list.push({ k: '排除路由', v: `${p.route_exclude} 条` })
  if (p.acl_count > 0) list.push({ k: 'ACL', v: `${p.acl_count} 条` })
  return list
})
</script>

<template>
  <div v-if="items.length" class="flex flex-wrap gap-6px">
    <span v-for="it in items" :key="it.k" class="inline-flex h-24px items-center gap-6px rounded-6px bg-fill px-8px text-sm">
      <span class="text-secondary">{{ it.k }}</span><span class="font-medium">{{ it.v }}</span>
    </span>
  </div>
  <div v-else class="text-sm text-tertiary">无额外限制</div>
</template>
