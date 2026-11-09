import type { ConnectionPhase } from '@/types/vpn'
import type { Tone } from './tone'

export const CONNECTION_STATUSES = [
  'disconnected',
  'connecting',
  'connected',
  'disconnecting',
] as const

export const CONNECTION_PHASES = [
  'connecting',
  'authenticating',
  'waiting_input',
  'negotiating',
  'configuring',
  'connected',
  'reconnecting',
  'disconnecting',
  'disconnected',
] as const

export const PROMPT_KINDS = ['otp', 'ftm_push', 'saml'] as const

export interface StatusMeta {
  label: string
  tone: Tone
  pulse: boolean
}

export const VPN_STATUS_META: Record<(typeof CONNECTION_STATUSES)[number], StatusMeta> = {
  connected: { label: '已连接', tone: 'success', pulse: false },
  connecting: { label: '正在连接…', tone: 'warning', pulse: true },
  disconnecting: { label: '正在断开…', tone: 'warning', pulse: true },
  disconnected: { label: '未连接', tone: 'neutral', pulse: false },
}

export const VPN_PHASE_LABELS: Record<ConnectionPhase, string> = {
  '': '正在处理连接请求…',
  connecting: '正在启动连接…',
  authenticating: '正在验证身份…',
  waiting_input: '等待双因素验证…',
  negotiating: '正在协商隧道…',
  configuring: '正在配置网络…',
  connected: '已连接',
  reconnecting: '网络中断，重新连接中…',
  disconnecting: '正在断开…',
  disconnected: '未连接',
}
