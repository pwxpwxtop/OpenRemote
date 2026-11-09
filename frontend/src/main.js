import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'
import App from './App.vue'
import router from './router'

import 'primeicons/primeicons.css'
import '@fortawesome/fontawesome-free/css/all.css'

// RustDesk 风格的主色（蓝色），深色模式由 html.dark 固定开启。
const AppPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '#eaf2ff',
      100: '#d3e4ff',
      200: '#a8c9ff',
      300: '#7cabff',
      400: '#4d8dfb',
      500: '#2f7cf6',
      600: '#1b66e0',
      700: '#1554bd',
      800: '#124496',
      900: '#0e3575',
      950: '#0a2555'
    }
  }
})

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(PrimeVue, {
  theme: {
    preset: AppPreset,
    options: {
      darkModeSelector: '.dark',
      cssLayer: false
    }
  }
})
app.use(ToastService)

app.mount('#app')
