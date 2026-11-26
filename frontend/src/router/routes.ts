import type { Component } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { Info, Palette, SlidersHorizontal } from '@lucide/vue'
import type { SettingsTabKey } from '@/types/settings'

declare module 'vue-router' {
  interface RouteMeta {
    /** 顶栏展示的固定页面标题；未指定时由布局层按当前状态决定。 */
    title?: string
    /** 设置分区下的页面，用于驱动整栏切换与顶栏层级面包屑。 */
    settings?: boolean
  }
}

export const ROUTE_NAMES = {
  workspace: 'workspace',
  profileCreate: 'profile-create',
  settings: 'settings',
} as const

export interface SettingsNavItemConfig {
  key: SettingsTabKey
  label: string
  icon: Component
}

export const SETTINGS_NAV_ITEMS: readonly SettingsNavItemConfig[] = [
  { key: 'general', label: '通用', icon: SlidersHorizontal },
  { key: 'appearance', label: '外观', icon: Palette },
  { key: 'about', label: '关于', icon: Info },
]

/** 设置子页路由名，例如 `settings-general`。 */
export function settingsRouteName(tab: SettingsTabKey): string {
  return `settings-${tab}`
}

const SETTINGS_VIEWS: Record<SettingsTabKey, () => Promise<Component>> = {
  general: () => import('@/views/settings/GeneralView.vue'),
  appearance: () => import('@/views/settings/AppearanceView.vue'),
  about: () => import('@/views/settings/AboutView.vue'),
}

export const APP_ROUTES: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: { name: ROUTE_NAMES.workspace },
  },
  {
    path: '/workspace',
    name: ROUTE_NAMES.workspace,
    component: () => import('@/views/WorkspaceView.vue'),
    meta: {},
  },
  {
    path: '/profiles/new',
    name: ROUTE_NAMES.profileCreate,
    component: () => import('@/views/ProfileCreateView.vue'),
    meta: { title: '新建连接' },
  },
  {
    path: '/settings',
    component: () => import('@/views/SettingsView.vue'),
    meta: { settings: true },
    children: [
      { path: '', name: ROUTE_NAMES.settings, redirect: { name: settingsRouteName('general') } },
      ...SETTINGS_NAV_ITEMS.map((item): RouteRecordRaw => ({
        path: item.key,
        name: settingsRouteName(item.key),
        component: SETTINGS_VIEWS[item.key],
        meta: { title: item.label },
      })),
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: { name: ROUTE_NAMES.workspace },
  },
]
