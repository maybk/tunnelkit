import { createMemoryHistory, createRouter, createWebHashHistory } from 'vue-router'
import { APP_ROUTES, ROUTE_NAMES } from './routes'

export { APP_ROUTES, ROUTE_NAMES }

export const router = createRouter({
  history: typeof window !== 'undefined' ? createWebHashHistory() : createMemoryHistory(),
  routes: APP_ROUTES,
})
