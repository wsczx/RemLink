import { fileURLToPath, URL } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import Icons from 'unplugin-icons/vite'

// 后端接口统一在 /api/v1 下；/debug 为 pprof/statsviz 工具页。开发时代理到 VITE_API_TARGET（默认本机 8800）
const apiPrefixes = ['/api', '/debug']

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const target = env.VITE_API_TARGET || 'https://127.0.0.1:8800'
  return {
    base: '/ui/',
    plugins: [vue(), UnoCSS(), Icons({ compiler: 'vue3', scale: 1 })],
    resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
    server: {
      proxy: Object.fromEntries(apiPrefixes.map(p => [p, { target, changeOrigin: true, secure: false, ws: true }])),
    },
    build: {
      outDir: 'ui', emptyOutDir: true, chunkSizeWarningLimit: 1500,
      // 产物文件名去掉开头的 _ / .（Go embed 默认会忽略这类文件）
      rolldownOptions: { output: { chunkFileNames: c => `assets/${c.name.replace(/^[_.]+/, '')}-[hash].js` } },
    },
  }
})
