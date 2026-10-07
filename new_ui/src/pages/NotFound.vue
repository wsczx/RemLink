<script setup lang="ts">
import { NButton } from 'naive-ui'
</script>

<template>
  <div class="flex min-h-screen flex-col items-center justify-center gap-sm bg-body px-md text-center">
    <div class="text-[96px] font-bold leading-none text-quaternary">404</div>
    <div class="text-3xl font-bold">页面不存在</div>
    <div class="text-md text-secondary">你访问的地址不存在或已被移除</div>
    <RouterLink to="/" class="mt-md"><NButton type="primary" size="large" class="min-w-100px">返回首页</NButton></RouterLink>
  </div>
</template>
