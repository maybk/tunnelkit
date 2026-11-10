import { VPN_PHASE_LABELS, VPN_STATUS_META, type StatusMeta } from '@/constants/vpn'
import { resolveViewingStatus } from '@/domain/vpn'
import type { Profile } from '@/types/profile'
import type { ConnectionStatus, VpnState } from '@/types/vpn'
import { formatEndpoint } from '@/utils/network'

export function getStatusMeta(status?: ConnectionStatus): StatusMeta {
  return VPN_STATUS_META[status ?? 'disconnected'] ?? VPN_STATUS_META.disconnected
}

export function getViewingStatusMeta(state: VpnState, viewingProfileId?: string): StatusMeta {
  const status = resolveViewingStatus(state, viewingProfileId)
  const meta = getStatusMeta(status)
  if (status === 'connecting')
    return { ...meta, label: VPN_PHASE_LABELS[state.phase] || meta.label }
  if (status === 'disconnected' && state.profileId === viewingProfileId && state.error)
    return { ...meta, tone: 'danger' }
  return meta
}

export function describeVpnPhase(state: VpnState, selectedProfile?: Profile | null): string {
  const isViewingActive = Boolean(
    selectedProfile && state.profileId && state.profileId === selectedProfile.id,
  )

  if (isViewingActive) {
    if (state.status === 'connected') {
      const ip = state.info?.localIp
      return ip ? `隧道已就绪 · 内网地址 ${ip}` : '安全隧道已建立，正在承载加密流量'
    }
    if (state.status === 'connecting' || state.status === 'disconnecting') {
      return VPN_PHASE_LABELS[state.phase] || '正在处理连接请求…'
    }
    if (state.error?.message) {
      return state.error.message
    }
  }

  if (state.status !== 'disconnected' && state.profileName && !isViewingActive) {
    return `当前「${state.profileName}」${getStatusMeta(state.status).label}`
  }

  if (selectedProfile) {
    return `目标网关 ${formatEndpoint(selectedProfile.host, selectedProfile.port)}`
  }

  return '选择或新建一个 VPN 连接以开始使用'
}
