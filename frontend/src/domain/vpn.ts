import type { ConnectionStatus, VpnState } from '@/types/vpn'

export function createInitialVpnState(): VpnState {
  return {
    revision: 0,
    status: 'disconnected',
    phase: 'disconnected',
    profileId: '',
    profileName: '',
    connectedAt: '',
    info: null,
    stats: null,
    prompt: null,
    error: null,
  }
}

export function isTransitionStatus(status?: ConnectionStatus): boolean {
  return status === 'connecting' || status === 'disconnecting'
}

export function isActiveStatus(status?: ConnectionStatus): boolean {
  return status === 'connected' || status === 'connecting'
}

export function isProfileActive(state: VpnState, profileId?: string): boolean {
  if (!profileId || !state.profileId) return false
  return state.profileId === profileId && state.status !== 'disconnected'
}

export function resolveViewingStatus(state: VpnState, viewingProfileId?: string): ConnectionStatus {
  if (!viewingProfileId) return state.status
  if (state.profileId && state.profileId === viewingProfileId) {
    return state.status
  }
  return 'disconnected'
}

export function needsSwitchConfirm(state: VpnState, targetProfileId: string): boolean {
  if (!targetProfileId) return false
  if (state.status === 'disconnected' || state.status === 'disconnecting') return false
  return Boolean(state.profileId && state.profileId !== targetProfileId)
}
