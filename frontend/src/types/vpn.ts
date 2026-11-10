import type { CONNECTION_PHASES, CONNECTION_STATUSES, PROMPT_KINDS } from '@/constants/vpn'
import type { DnsMode } from './profile'

export type ConnectionStatus = (typeof CONNECTION_STATUSES)[number]
export type ConnectionPhase = (typeof CONNECTION_PHASES)[number] | ''
export type PromptKind = (typeof PROMPT_KINDS)[number] | ''

export interface VpnPrompt {
  id: string
  kind: PromptKind
  message: string
  url: string
}

export interface VpnTunnelInfo {
  interface: string
  localIp: string
  peerIp: string
  mtu: number
  routeCount: number
  routes: string[]
  dnsMode: DnsMode
  dnsServers: string[]
  dnsDomains: string[]
  splitDnsNeedsDomains: boolean
  gatewayHost: string
  gatewayIp: string
  fortiOs: string
}

export interface VpnStats {
  rxBytes: number
  txBytes: number
  rxPackets: number
  txPackets: number
  rttMillis: number
}

export interface VpnError {
  message: string
  hint: string
  trustedCert: string
}

export interface VpnState {
  revision: number
  status: ConnectionStatus
  phase: ConnectionPhase
  profileId: string
  profileName: string
  connectedAt: string
  info: VpnTunnelInfo | null
  stats: VpnStats | null
  prompt: VpnPrompt | null
  error: VpnError | null
}

export interface VpnPendingAction {
  kind: 'connect' | 'disconnect'
  profileId: string
}
