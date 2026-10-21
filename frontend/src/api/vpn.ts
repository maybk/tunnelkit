import { VPNAPI } from '../../bindings/tunnelkit/internal/api'
import type * as vpn from '../../bindings/tunnelkit/internal/vpn'
import type * as fortivpn from '../../bindings/tunnelkit/fortivpn'

export type VpnStatus = 'disconnected' | 'connecting' | 'connected' | 'disconnecting'
export type VpnPhase =
  | 'connecting'
  | 'authenticating'
  | 'waiting_input'
  | 'negotiating'
  | 'configuring'
  | 'connected'
  | 'reconnecting'
  | 'disconnecting'
  | 'disconnected'

export type VpnError = vpn.Error
export type VpnLogView = vpn.LogView
export type VpnTunnelInfo = fortivpn.TunnelInfo
export type VpnStats = fortivpn.Stats
export type VpnPrompt = fortivpn.Prompt

export interface VpnState {
  status: VpnStatus
  phase?: VpnPhase
  profileId?: string
  profileName?: string
  connectedAt?: string
  info?: VpnTunnelInfo | null
  stats?: VpnStats | null
  prompt?: VpnPrompt | null
  error?: VpnError | null
}

export const VPN_STATE_EVENT = 'vpn:state'

export const toVpnState = (raw: vpn.State): VpnState => ({
  status: (raw.status || 'disconnected') as VpnStatus,
  phase: (raw.phase || raw.status || 'disconnected') as VpnPhase,
  profileId: raw.profileId,
  profileName: raw.profileName,
  connectedAt: raw.connectedAt,
  info: raw.info ?? null,
  stats: raw.stats ?? null,
  prompt: raw.prompt ?? null,
  error: raw.error ?? null,
})

export const getVpnState = async (): Promise<VpnState> => toVpnState(await VPNAPI.State())

export const connectVpn = (profileId: string): Promise<void> => VPNAPI.Connect(profileId)

export const cancelConnect = (): Promise<void> => VPNAPI.Cancel()

export const answerVpnPrompt = (promptId: string, value: string, cancel = false): Promise<void> =>
  VPNAPI.Answer(promptId, value, cancel)

export const disconnectVpn = (): Promise<void> => VPNAPI.Disconnect()

export const getVpnLog = (profileId: string): Promise<VpnLogView> => VPNAPI.Log(profileId)

export const clearVpnLog = (profileId: string): Promise<void> => VPNAPI.ClearLog(profileId)
