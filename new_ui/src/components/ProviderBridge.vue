<script setup lang="ts">
// 把 Naive 的 message / dialog / loadingBar 挂到模块级，供非组件代码（拦截器、路由守卫）使用
import { useDialog, useLoadingBar, useMessage, useNotification } from 'naive-ui'
import { ui } from '@/utils/ui'

ui.message = useMessage()
ui.dialog = useDialog()
ui.loadingBar = useLoadingBar()
ui.notification = useNotification()
</script>

<template><slot /></template>
