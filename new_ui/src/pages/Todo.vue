<script setup lang="ts">
// 未迁移页面的占位
import { useRoute } from 'vue-router'
import RlPanel from '@/components/RlPanel.vue'
import PageHeader from '@/components/PageHeader.vue'

const route = useRoute()
</script>

<template>
  <div :class="route.meta.public ? 'p-lg' : ''">
    <PageHeader :title="route.meta.title || '页面'" />
    <RlPanel>
      <div class="flex min-h-200px flex-col items-center justify-center gap-xs">
        <div class="text-xl font-bold">该页面正在迁移到新界面</div>
        <div class="text-md text-secondary">旧版页面路径：{{ route.path }}</div>
      </div>
    </RlPanel>
  </div>
</template>
