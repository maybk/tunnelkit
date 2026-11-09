import { DEFAULT_VPN_PORT } from '@/constants/profile'
import { createInitialVpnState } from '@/domain/vpn'
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

export function mapVpnStateFromDto(dto: Partial<vpnModels.State> | null | undefined): VpnState {
  if (!dto) return createInitialVpnState()

  const info: VpnTunnelInfo | null = dto.info
    ? {
        interface: dto.info.interface ?? '',
        localIp: dto.info.localIp ?? '',
        peerIp: dto.info.peerIp ?? '',
        mtu: Number(dto.info.mtu ?? 0),
        routeCount: Number(dto.info.routeCount ?? 0),
        routes: Array.isArray(dto.info.routes) ? [...dto.info.routes] : [],
        dnsMode: normalizeDnsMode(dto.info.dnsMode),
        dnsServers: Array.isArray(dto.info.dnsServers) ? [...dto.info.dnsServers] : [],
        dnsDomains: Array.isArray(dto.info.dnsDomains) ? [...dto.info.dnsDomains] : [],
        splitDnsNeedsDomains: Boolean(dto.info.splitDnsNeedsDomains),
        gatewayHost: dto.info.gatewayHost ?? '',
        gatewayIp: dto.info.gatewayIp ?? '',
        fortiOs: dto.info.fortiOs ?? '',
      }
    : null

  const stats: VpnStats | null = dto.stats
    ? {
        rxBytes: Number(dto.stats.rxBytes ?? 0),
        txBytes: Number(dto.stats.txBytes ?? 0),
        rxPackets: Number(dto.stats.rxPackets ?? 0),
        txPackets: Number(dto.stats.txPackets ?? 0),
        rttMillis: Number(dto.stats.rttMillis ?? 0),
      }
    : null

  const prompt: VpnPrompt | null = dto.prompt
    ? {
        id: dto.prompt.id ?? '',
        kind: (dto.prompt.kind as PromptKind) ?? '',
        message: dto.prompt.message ?? '',
        url: dto.prompt.url ?? '',
      }
    : null

  const error: VpnError | null = dto.error
    ? {
        message: dto.error.message ?? '',
        hint: dto.error.hint ?? '',
        trustedCert: dto.error.trustedCert ?? '',
      }
    : null

  return {
    status: normalizeStatus(dto.status),
    phase: (dto.phase as ConnectionPhase) ?? 'disconnected',
    profileId: dto.profileId ?? '',
    profileName: dto.profileName ?? '',
    connectedAt: dto.connectedAt ?? '',
    info,
    stats,
    prompt,
    error,
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
