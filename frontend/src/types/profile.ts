import type { AUTH_TYPES, DNS_MODES, ROUTE_MODES, WORKSPACE_TABS } from '@/constants/profile'

export type AuthType = (typeof AUTH_TYPES)[number]
export type RouteMode = (typeof ROUTE_MODES)[number]
export type DnsMode = (typeof DNS_MODES)[number]
export type WorkspaceTab = (typeof WORKSPACE_TABS)[number]

export interface Profile {
  id: string
  name: string
  host: string
  port: number
  username: string
  realm: string
  trustedCert: string
  auth: AuthType
  routes: RouteMode
  extraRoutes: string[]
  dns: DnsMode
  dnsDomains: string[]
  reconnect: boolean
  userAgent: string
  legacyTls: boolean
  clientCert: string
  clientKey: string
  hasPassword: boolean
  updatedAt: string
}

export interface ProfileInput {
  id: string
  name: string
  host: string
  port: number
  username: string
  password: string
  realm: string
  trustedCert: string
  auth: AuthType
  routes: RouteMode
  extraRoutes: string[]
  dns: DnsMode
  dnsDomains: string[]
  reconnect: boolean
  userAgent: string
  legacyTls: boolean
  clientCert: string
  clientKey: string
}

export interface ProfileDraft {
  id: string
  name: string
  host: string
  port: number
  username: string
  password: string
  realm: string
  trustedCert: string
  auth: AuthType
  routes: RouteMode
  extraRoutesText: string
  dns: DnsMode
  dnsDomainsText: string
  reconnect: boolean
  userAgent: string
  legacyTls: boolean
  clientCert: string
  clientKey: string
  hasPassword: boolean
}

export interface ConfigWarning {
  line: number
  key: string
  message: string
}

export interface ParsedProfileText {
  input: ProfileInput
  warnings: ConfigWarning[]
}
