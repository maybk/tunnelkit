import { createMemoryHistory, createRouter, createWebHashHistory } from 'vue-router'
import { APP_ROUTES, ROUTE_NAMES } from './routes'

export {
  APP_ROUTES,
  ROUTE_NAMES,
  SETTINGS_NAV_ITEMS,
  settingsRouteName,
  type SettingsNavItemConfig,
} from './routes'

export const router = createRouter({
  history: typeof window !== 'undefined' ? createWebHashHistory() : createMemoryHistory(),
  routes: APP_ROUTES,
})

let lastMainRoute = '/workspace'
let lastSettingsRoute = ''

// 再次进入设置分区时恢复上次停留的子页。
router.beforeEach((to) => {
  if (
    to.redirectedFrom?.name === ROUTE_NAMES.settings &&
    lastSettingsRoute &&
    to.fullPath !== lastSettingsRoute
  ) {
    return { path: lastSettingsRoute, replace: true }
  }
})

router.afterEach((to) => {
  if (to.meta.settings) lastSettingsRoute = to.fullPath
  else lastMainRoute = to.fullPath
})

/** 退出设置分区，回到进入设置前的主页面。 */
export function backFromSettings() {
  return router.push(lastMainRoute)
}

/** 测试或状态重置时清空路由会话记忆。 */
export function resetRouterSessionMemory() {
  lastMainRoute = '/workspace'
  lastSettingsRoute = ''
}
