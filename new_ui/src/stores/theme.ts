import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

// 主题：localStorage('dark-mode') 优先，未设置时跟随系统
export const useThemeStore = defineStore('theme', () => {
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const saved = localStorage.getItem('dark-mode')
  const isDark = ref(saved !== null ? saved === 'true' : mq.matches)

  mq.addEventListener('change', e => {
    if (localStorage.getItem('dark-mode') === null) isDark.value = e.matches
  })
  watch(isDark, v => {
    if (v) document.documentElement.dataset.theme = 'dark'
    else delete document.documentElement.dataset.theme
  }, { immediate: true })

  function toggle() {
    isDark.value = !isDark.value
    localStorage.setItem('dark-mode', String(isDark.value))
  }
  return { isDark, toggle }
})
