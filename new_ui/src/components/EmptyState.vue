<script setup lang="ts">
import type { Component } from 'vue'
defineProps<{ title: string; description?: string; icon?: Component }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center gap-6px px-md py-48px text-center">
    <div v-if="icon" class="mb-4px flex h-40px w-40px items-center justify-center rounded-10px bg-fill text-xl text-secondary"><component :is="icon" /></div>
    <div class="text-lg font-semibold">{{ title }}</div>
    <div v-if="description" class="max-w-420px text-md text-secondary">{{ description }}</div>
    <div v-if="$slots.default" class="mt-10px"><slot /></div>
  </div>
</template>
