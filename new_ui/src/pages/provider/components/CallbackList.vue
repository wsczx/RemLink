<script setup lang="ts">
// 第三方开放平台需填写的回调地址列表
import { computed } from 'vue'
import CallbackUrl from './CallbackUrl.vue'
import { callbacksOf } from './types'

const props = defineProps<{ type: string; base: string }>()
const items = computed(() => callbacksOf(props.type, props.base))
</script>

<template>
  <div v-if="items.length" class="grid gap-8px">
    <CallbackUrl v-for="c in items" :key="c.url" :label="c.label" :url="c.url" :warn="c.warn">
      <template v-if="c.note">{{ c.note }}</template>
    </CallbackUrl>
  </div>
</template>
