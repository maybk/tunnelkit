import * as v from 'valibot'
import { isValidIpv4Cidr, normalizeHostInput } from '@/utils/network'

const SHA256_HEX_RE = /^[0-9a-fA-F]{64}$/

export const NonEmptyTrimmedStringSchema = (message: string) =>
  v.pipe(
    v.string(message),
    v.transform((s) => s.trim()),
    v.minLength(1, message),
  )

export const PortSchema = v.pipe(
  v.number('端口必须是 1 到 65535 之间的整数'),
  v.integer('端口必须是 1 到 65535 之间的整数'),
  v.minValue(1, '端口必须在 1 到 65535 之间'),
  v.maxValue(65535, '端口必须在 1 到 65535 之间'),
)

export const GatewayHostSchema = v.pipe(
  v.string('请填写 VPN 网关地址'),
  v.transform((raw) => normalizeHostInput(raw)),
  v.minLength(1, '请填写 VPN 网关地址'),
  v.check((host) => !/\s/.test(host), '网关地址不能包含空格'),
)

export const OptionalTrustedCertSchema = v.pipe(
  v.optional(v.string(), ''),
  v.transform((s) => s.trim().replaceAll(':', '').toLowerCase()),
  v.check((s) => s === '' || SHA256_HEX_RE.test(s), '证书指纹需为 64 位十六进制 SHA-256 字符串'),
)

export const ExtraRoutesListSchema = v.pipe(
  v.array(v.string()),
  v.check(
    (routes) => routes.every((item) => isValidIpv4Cidr(item)),
    '附加路由必须为合法的 IPv4 CIDR 格式（例如 10.10.0.0/16）',
  ),
)
