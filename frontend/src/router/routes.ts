import type { RouteRecordRaw } from 'vue-router'

export const ROUTE_NAMES = {
  workspace: 'workspace',
  profileCreate: 'profile-create',
} as const

export const APP_ROUTES: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/workspace',
  },
  {
    path: '/workspace',
    name: ROUTE_NAMES.workspace,
    component: () => import('@/views/WorkspaceView.vue'),
    meta: { title: 'TunnelKit' },
  },
  {
    path: '/profiles/new',
    name: ROUTE_NAMES.profileCreate,
    component: () => import('@/views/ProfileCreateView.vue'),
    meta: { title: '新建节点' },
  },
]
