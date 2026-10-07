<script setup lang="ts">
// 国旗：flag-icons（lipis/flag-icons，MIT），按 ISO 3166-1 alpha-2 渲染 SVG
// 内网 / 未知地区显示占位图标
import { computed } from 'vue'
import 'flag-icons/css/flag-icons.min.css'
import { countryName } from '@/utils/net'

const props = withDefaults(defineProps<{ code?: string; size?: number; square?: boolean; withName?: boolean }>(), { size: 16 })
const cc = computed(() => (props.code || '').toLowerCase())
const valid = computed(() => /^[a-z]{2}$/.test(cc.value) && cc.value !== 'zz')
const title = computed(() => (valid.value ? countryName(cc.value) : props.code === 'LAN' ? '内网' : '未知地区'))
</script>

<template>
  <span class="inline-flex min-w-0 items-center gap-6px align-middle" :title="title">
    <span
      v-if="valid"
      class="fi rl-flag flex-shrink-0" :class="[`fi-${cc}`, square && 'fis']"
      :style="{ width: (square ? size : Math.round(size * 4 / 3)) + 'px', height: size + 'px' }"
    />
    <span v-else class="rl-flag rl-flag--none flex-shrink-0" :style="{ width: (square ? size : Math.round(size * 4 / 3)) + 'px', height: size + 'px' }" />
    <span v-if="withName" class="truncate">{{ title }}</span>
  </span>
</template>
