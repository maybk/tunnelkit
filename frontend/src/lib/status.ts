import type { VpnError, VpnPhase, VpnState, VpnStatus } from '@/api/vpn'

export type StatusTone = 'idle' | 'busy' | 'ok' | 'danger'

export const STATUS_LABEL: Record<VpnStatus, string> = {
  disconnected: '未连接',
  connecting: '正在连接…',
  connected: '已连接',
  disconnecting: '正在断开…',
}

export const PHASE_LABEL: Record<VpnPhase, string> = {
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

/** 某个配置在当前全局 VPN 状态下的展示视图。 */
export interface ProfileStatus {
  /** 全局状态是否指向该配置。 */
  isTarget: boolean
  /** 该配置自身的状态；全局状态指向其他配置时视为 disconnected。 */
  status: VpnStatus
  phase: VpnPhase
  tone: StatusTone
  label: string
  error: VpnError | null
  /** 其他配置正在连接、已连接或断开中。 */
  otherActive: boolean
}

export const profileStatus = (profileId: string, state: VpnState): ProfileStatus => {
  const isTarget = state.profileId === profileId
  const status: VpnStatus = isTarget ? state.status : 'disconnected'
  const phase: VpnPhase = isTarget ? (state.phase ?? (status as VpnPhase)) : 'disconnected'
  const error = isTarget ? (state.error ?? null) : null

  let tone: StatusTone = 'idle'
  if (status === 'connected') tone = 'ok'
  else if (status === 'connecting' || status === 'disconnecting') tone = 'busy'
  else if (error) tone = 'danger'

  const label =
    status === 'connecting' && PHASE_LABEL[phase] ? PHASE_LABEL[phase] : STATUS_LABEL[status]

  return {
    isTarget,
    status,
    phase,
    tone,
    label,
    error,
    otherActive: !isTarget && state.status !== 'disconnected',
  }
}
