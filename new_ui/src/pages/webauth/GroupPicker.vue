<script setup lang="ts">
import { NIcon } from 'naive-ui'
import PeopleTeam from '~icons/fluent/people-team-20-regular'
import Checkmark from '~icons/fluent/checkmark-circle-20-filled'

// 用户组单选列表
defineProps<{ groups: string[] }>()
const model = defineModel<string>({ default: '' })
</script>

<template>
  <div class="grid max-h-300px gap-8px overflow-y-auto">
    <button
      v-for="g in groups" :key="g" type="button"
      class="flex cursor-pointer items-center gap-12px rounded-10px border border-solid bg-transparent px-14px py-12px text-left text-primary transition-colors"
      :class="model === g ? 'border-[var(--color-action)] bg-action-hover!' : 'border-[var(--color-border-default)] hover:border-[var(--color-border-strong)]'"
      style="font: inherit"
      @click="model = g"
    >
      <span class="flex h-32px w-32px flex-shrink-0 items-center justify-center rounded-8px bg-fill text-lg text-secondary"><PeopleTeam /></span>
      <span class="min-w-0 flex-1 truncate text-md font-medium">{{ g }}</span>
      <NIcon v-if="model === g" size="20"><Checkmark /></NIcon>
    </button>
  </div>
</template>
