import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'vfonts/Lato.css'
import 'vfonts/FiraCode.css'
import 'virtual:uno.css'
import './styles/global.css'
import { injectThemeVars } from './theme/theme'
import App from './App.vue'
import router from './router'

injectThemeVars()

createApp(App).use(createPinia()).use(router).mount('#app')
