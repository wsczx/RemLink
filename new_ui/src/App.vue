<script setup lang="ts">
import { computed } from 'vue'
import { NConfigProvider, NMessageProvider, NDialogProvider, NLoadingBarProvider, NNotificationProvider, darkTheme, zhCN, dateZhCN } from 'naive-ui'
import { darkOverrides, lightOverrides } from './theme/theme'
import { useThemeStore } from './stores/theme'
import ProviderBridge from './components/ProviderBridge.vue'

const theme = useThemeStore()
const naiveTheme = computed(() => (theme.isDark ? darkTheme : null))
const overrides = computed(() => (theme.isDark ? darkOverrides : lightOverrides))
</script>

<template>
  <NConfigProvider :theme="naiveTheme" :theme-overrides="overrides" :locale="zhCN" :date-locale="dateZhCN" class="h-full">
    <NLoadingBarProvider>
      <NDialogProvider>
        <NNotificationProvider>
          <NMessageProvider>
            <ProviderBridge />
            <RouterView />
          </NMessageProvider>
        </NNotificationProvider>
      </NDialogProvider>
    </NLoadingBarProvider>
  </NConfigProvider>
</template>
