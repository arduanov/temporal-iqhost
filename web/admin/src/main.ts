import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import PrimeVue from 'primevue/config'
import Aura from '@primeuix/themes/aura'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import App from './App.vue'
import Machines from './pages/Machines.vue'
import Machine from './pages/Machine.vue'
import Operations from './pages/Operations.vue'
import Operation from './pages/Operation.vue'
import Resources from './pages/Resources.vue'
import 'primeicons/primeicons.css'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/vms' },
    { path: '/vms', component: Machines },
    { path: '/vm', component: Machine },
    { path: '/operations', component: Operations },
    { path: '/operations/:id', component: Operation },
    { path: '/resources', component: Resources },
  ],
})
const app = createApp(App)
app.use(router).use(PrimeVue, { theme: { preset: Aura, options: { darkModeSelector: false } } })
app
  .component('PButton', Button)
  .component('InputText', InputText)
  .component('DataTable', DataTable)
  .component('Column', Column)
app.mount('#app')
