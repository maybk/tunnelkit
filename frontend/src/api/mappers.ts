import { DEFAULT_VPN_PORT } from '@/constants/profile'
import { createInitialVpnState } from '@/domain/vpn'
import { PROMPT_KINDS } from '@/constants/vpn'
import type { LogViewSnapshot } from '@/types/log'
import type {
  AuthType,
  ConfigWarning,
  DnsMode,
  ParsedProfileText,
  Profile,
  ProfileInput,
  RouteMode,
} from '@/types/profile'
import type { SystemInfo, UpdateInfo, UpdateProgress } from '@/types/system'
import type {
  ConnectionPhase,
  ConnectionStatus,
  PromptKind,
  VpnError,
  VpnPrompt,
  VpnState,
  VpnStats,
  VpnTunnelInfo,
} from '@/types/vpn'
import * as apiModels from '@bindings/tunnelkit/internal/api/models.js'
import * as profileModels from '@bindings/tunnelkit/internal/profile/models.js'
import * as updateModels from '@bindings/tunnelkit/internal/update/models.js'
import * as vpnModels from '@bindings/tunnelkit/internal/vpn/models.js'

function normalizeAuthType(raw?: string): AuthType {
  return raw === 'saml' ? 'saml' : 'password'
}

function normalizeRouteMode(raw?: string): RouteMode {
  if (raw === 'full' || raw === 'none') return raw
  return 'gateway'
}

function normalizeDnsMode(raw?: string): DnsMode {
  if (raw === 'full' || raw === 'off') return raw
  return 'split'
}

export function mapProfileFromDto(dto: Partial<profileModels.Profile> | null | undefined): Profile {
  return {
    id: dto?.id ?? '',
    name: dto?.name ?? '',
    host: dto?.host ?? '',
    port: Number(dto?.port) || DEFAULT_VPN_PORT,
    username: dto?.username ?? '',
    realm: dto?.realm ?? '',
    trustedCert: dto?.trustedCert ?? '',
    auth: normalizeAuthType(dto?.auth),
    routes: normalizeRouteMode(dto?.routes),
    extraRoutes: Array.isArray(dto?.extraRoutes) ? [...dto.extraRoutes] : [],
    dns: normalizeDnsMode(dto?.dns),
    dnsDomains: Array.isArray(dto?.dnsDomains) ? [...dto.dnsDomains] : [],
    reconnect: Boolean(dto?.reconnect ?? true),
    userAgent: dto?.userAgent ?? '',
    legacyTls: Boolean(dto?.legacyTls),
    clientCert: dto?.clientCert ?? '',
    clientKey: dto?.clientKey ?? '',
    hasPassword: Boolean(dto?.hasPassword),
    updatedAt: dto?.updatedAt ?? '',
  }
}

export function mapProfileInputToDto(input: ProfileInput): profileModels.Input {
  return new profileModels.Input({
    id: input.id,
    name: input.name,
    host: input.host,
    port: input.port,
    username: input.username,
    password: input.password,
    realm: input.realm,
    trustedCert: input.trustedCert,
    auth: input.auth,
    routes: input.routes,
    extraRoutes: [...input.extraRoutes],
    dns: input.dns,
    dnsDomains: [...input.dnsDomains],
    reconnect: input.reconnect,
    userAgent: input.userAgent,
    legacyTls: input.legacyTls,
    clientCert: input.clientCert,
    clientKey: input.clientKey,
  })
}

export function mapProfileInputFromDto(
  dto: Partial<profileModels.Input> | null | undefined,
): ProfileInput {
  return {
    id: dto?.id ?? '',
    name: dto?.name ?? '',
    host: dto?.host ?? '',
    port: Number(dto?.port) || DEFAULT_VPN_PORT,
    username: dto?.username ?? '',
    password: dto?.password ?? '',
    realm: dto?.realm ?? '',
    trustedCert: dto?.trustedCert ?? '',
    auth: normalizeAuthType(dto?.auth),
    routes: normalizeRouteMode(dto?.routes),
    extraRoutes: Array.isArray(dto?.extraRoutes) ? [...dto.extraRoutes] : [],
    dns: normalizeDnsMode(dto?.dns),
    dnsDomains: Array.isArray(dto?.dnsDomains) ? [...dto.dnsDomains] : [],
    reconnect: Boolean(dto?.reconnect ?? true),
    userAgent: dto?.userAgent ?? '',
    legacyTls: Boolean(dto?.legacyTls),
    clientCert: dto?.clientCert ?? '',
    clientKey: dto?.clientKey ?? '',
  }
}

export function mapParsedProfileTextFromDto(
  dto: Partial<apiModels.ParsedProfileText> | null | undefined,
): ParsedProfileText {
  const warnings: ConfigWarning[] = Array.isArray(dto?.warnings)
    ? dto.warnings.map((w) => ({
        line: Number(w?.line ?? 0),
        key: w?.key ?? '',
        message: w?.message ?? '',
      }))
    : []
  return {
    input: mapProfileInputFromDto(dto?.input),
    warnings,
  }
}

function normalizeStatus(raw?: string): ConnectionStatus {
  if (raw === 'connecting' || raw === 'connected' || raw === 'disconnecting') {
    return raw
  }
  return 'disconnected'
}

function mapTunnelInfoFromDto(
  status: ConnectionStatus,
  raw: Partial<vpnModels.State>['info'],
): VpnTunnelInfo | null {
  if (status !== 'connected' || !raw) return null
  const iface = raw.interface?.trim() ?? ''
  const localIp = raw.localIp?.trim() ?? ''
  const peerIp = raw.peerIp?.trim() ?? ''
  const mtu = Number(raw.mtu ?? 0)
  const routeCount = Number(raw.routeCount ?? 0)
  const routes = Array.isArray(raw.routes) ? [...raw.routes] : []
  const rawDnsMode = raw.dnsMode?.trim() ?? ''
  const dnsServers = Array.isArray(raw.dnsServers) ? [...raw.dnsServers] : []
  const dnsDomains = Array.isArray(raw.dnsDomains) ? [...raw.dnsDomains] : []
  const splitDnsNeedsDomains = Boolean(raw.splitDnsNeedsDomains)
  const gatewayHost = raw.gatewayHost?.trim() ?? ''
  const gatewayIp = raw.gatewayIp?.trim() ?? ''
  const fortiOs = raw.fortiOs?.trim() ?? ''
  const hasContent =
    Boolean(iface || localIp || peerIp || rawDnsMode || gatewayHost || gatewayIp || fortiOs) ||
    mtu > 0 ||
    routeCount > 0 ||
    routes.length > 0 ||
    dnsServers.length > 0 ||
    dnsDomains.length > 0 ||
    splitDnsNeedsDomains
  if (!hasContent) return null
  return {
    interface: iface,
    localIp,
    peerIp,
    mtu,
    routeCount,
    routes,
    dnsMode: normalizeDnsMode(rawDnsMode),
    dnsServers,
    dnsDomains,
    splitDnsNeedsDomains,
    gatewayHost,
    gatewayIp,
    fortiOs,
  }
}

function mapStatsFromDto(
  status: ConnectionStatus,
  raw: Partial<vpnModels.State>['stats'],
): VpnStats | null {
  if (status !== 'connected' || !raw) return null
  return {
    rxBytes: Number(raw.rxBytes ?? 0),
    txBytes: Number(raw.txBytes ?? 0),
    rxPackets: Number(raw.rxPackets ?? 0),
    txPackets: Number(raw.txPackets ?? 0),
    rttMillis: Number(raw.rttMillis ?? 0),
  }
}

function mapPromptFromDto(
  status: ConnectionStatus,
  phase: ConnectionPhase,
  raw: Partial<vpnModels.State>['prompt'],
): VpnPrompt | null {
  if (
    status !== 'connecting' ||
    phase !== 'waiting_input' ||
    !raw?.id ||
    !PROMPT_KINDS.some((kind) => kind === raw.kind)
  ) {
    return null
  }
  return {
    id: raw.id,
    kind: raw.kind as PromptKind,
    message: raw.message ?? '',
    url: raw.url ?? '',
  }
}

function mapErrorFromDto(raw: Partial<vpnModels.State>['error']): VpnError | null {
  const message = raw?.message?.trim() ?? ''
  const hint = raw?.hint?.trim() ?? ''
  const trustedCert = raw?.trustedCert?.trim() ?? ''
  if (!message && !hint && !trustedCert) return null
  return {
    message: message || 'VPN 会话异常',
    hint,
    trustedCert,
  }
}

export function mapVpnStateFromDto(dto: Partial<vpnModels.State> | null | undefined): VpnState {
  if (!dto) return createInitialVpnState()

  const status = normalizeStatus(dto.status)
  const phase = (dto.phase as ConnectionPhase) ?? 'disconnected'

  return {
    revision: Number(dto.revision ?? 0),
    status,
    phase,
    profileId: dto.profileId ?? '',
    profileName: dto.profileName ?? '',
    connectedAt: status === 'connected' ? (dto.connectedAt ?? '') : '',
    info: mapTunnelInfoFromDto(status, dto.info),
    stats: mapStatsFromDto(status, dto.stats),
    prompt: mapPromptFromDto(status, phase, dto.prompt),
    error: mapErrorFromDto(dto.error),
  }
}

export function mapLogViewFromDto(
  dto: Partial<vpnModels.LogView> | null | undefined,
): LogViewSnapshot {
  return {
    profileId: dto?.profileId ?? '',
    content: dto?.content ?? '',
    exists: Boolean(dto?.exists),
    updatedAt: dto?.updatedAt ?? '',
  }
}

export function mapSystemInfoFromDto(
  dto: Partial<apiModels.SystemInfo> | null | undefined,
): SystemInfo {
  return {
    appVersion: dto?.appVersion ?? '',
    platform: dto?.platform ?? '',
    helperReady: Boolean(dto?.helperReady),
    helperPath: dto?.helperPath ?? '',
  }
}

export function mapUpdateInfoFromDto(
  dto: Partial<updateModels.Info> | null | undefined,
): UpdateInfo {
  return {
    currentVersion: dto?.currentVersion ?? '',
    latestVersion: dto?.latestVersion ?? '',
    hasUpdate: Boolean(dto?.hasUpdate),
    releaseUrl: dto?.releaseUrl ?? '',
    assetName: dto?.assetName ?? '',
    assetUrl: dto?.assetUrl ?? '',
    assetSize: Number(dto?.assetSize ?? 0),
    assetSha256: dto?.assetSha256 ?? '',
    canSelfUpdate: Boolean(dto?.canSelfUpdate),
  }
}

export function mapUpdateProgressFromDto(
  dto: Partial<updateModels.Progress> | null | undefined,
): UpdateProgress {
  return {
    stage: dto?.stage ?? '',
    downloadedBytes: Number(dto?.downloadedBytes ?? 0),
    totalBytes: Number(dto?.totalBytes ?? 0),
    percent: Number(dto?.percent ?? 0),
    message: dto?.message ?? '',
  }
}
