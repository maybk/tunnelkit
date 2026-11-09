import { DEFAULT_VPN_PORT } from '@/constants/profile'

export interface ParsedGatewayEndpoint {
  host: string
  port?: number
}

export function normalizeHostInput(raw: string): string {
  let text = raw.trim()
  text = text.replace(/^https?:\/\//i, '')
  const slashIndex = text.indexOf('/')
  if (slashIndex >= 0) {
    text = text.slice(0, slashIndex)
  }
  return text.trim()
}

export function parseGatewayEndpoint(raw: string): ParsedGatewayEndpoint {
  const cleaned = normalizeHostInput(raw)
  if (!cleaned) return { host: '' }

  // IPv6 bracket notation: [2001:db8::1]:10443
  if (cleaned.startsWith('[')) {
    const closeIdx = cleaned.indexOf(']')
    if (closeIdx > 1) {
      const host = cleaned.slice(1, closeIdx)
      const rest = cleaned.slice(closeIdx + 1)
      if (rest.startsWith(':')) {
        const port = Number(rest.slice(1))
        if (Number.isInteger(port) && port >= 1 && port <= 65535) {
          return { host, port }
        }
      }
      return { host }
    }
  }

  const firstColon = cleaned.indexOf(':')
  const lastColon = cleaned.lastIndexOf(':')
  if (firstColon > 0 && firstColon === lastColon) {
    const hostPart = cleaned.slice(0, firstColon).trim()
    const portPart = Number(cleaned.slice(firstColon + 1).trim())
    if (hostPart && Number.isInteger(portPart) && portPart >= 1 && portPart <= 65535) {
      return { host: hostPart, port: portPart }
    }
  }

  return { host: cleaned }
}

export function isValidIpv4(ip: string): boolean {
  const parts = ip.trim().split('.')
  if (parts.length !== 4) return false
  return parts.every((part) => {
    if (!/^\d{1,3}$/.test(part)) return false
    const num = Number(part)
    return num >= 0 && num <= 255
  })
}

export function isValidIpv4Cidr(cidr: string): boolean {
  const trimmed = cidr.trim()
  const slashIdx = trimmed.indexOf('/')
  if (slashIdx <= 0) return false
  const ip = trimmed.slice(0, slashIdx)
  const prefixStr = trimmed.slice(slashIdx + 1)
  if (!isValidIpv4(ip) || !/^\d{1,2}$/.test(prefixStr)) return false
  const prefix = Number(prefixStr)
  return prefix >= 0 && prefix <= 32
}

export function formatEndpoint(host?: string, port?: number): string {
  const trimmedHost = (host ?? '').trim()
  if (!trimmedHost) return '未设置网关'
  const resolvedPort = port && port > 0 ? port : DEFAULT_VPN_PORT
  return trimmedHost.includes(':') && !trimmedHost.startsWith('[')
    ? `[${trimmedHost}]:${resolvedPort}`
    : `${trimmedHost}:${resolvedPort}`
}
