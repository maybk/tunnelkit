import * as v from 'valibot'
import { AUTH_TYPES, DNS_MODES, ROUTE_MODES } from '@/constants/profile'
import { toProfileInput } from '@/domain/profile'
import type { ProfileDraft, ProfileInput } from '@/types/profile'
import {
  ExtraRoutesListSchema,
  GatewayHostSchema,
  NonEmptyTrimmedStringSchema,
  OptionalTrustedCertSchema,
  PortSchema,
} from './primitives'

export const ProfileInputSchema = v.pipe(
  v.object({
    id: v.string(),
    name: NonEmptyTrimmedStringSchema('请填写节点名称'),
    host: GatewayHostSchema,
    port: PortSchema,
    username: v.string(),
    password: v.string(),
    realm: v.string(),
    trustedCert: OptionalTrustedCertSchema,
    auth: v.picklist(AUTH_TYPES, '不支持的认证方式'),
    routes: v.picklist(ROUTE_MODES, '不支持的路由策略'),
    extraRoutes: ExtraRoutesListSchema,
    dns: v.picklist(DNS_MODES, '不支持的 DNS 策略'),
    dnsDomains: v.array(v.string()),
    reconnect: v.boolean(),
    userAgent: v.string(),
    legacyTls: v.boolean(),
    clientCert: v.string(),
    clientKey: v.string(),
  }),
  v.forward(
    v.partialCheck(
      [['auth'], ['username']],
      (input) => input.auth === 'saml' || input.username.trim().length > 0,
      '账号密码模式下请填写用户名',
    ),
    ['username'],
  ),
  v.forward(
    v.partialCheck(
      [['clientCert'], ['clientKey']],
      (input) =>
        (input.clientCert.trim() === '' && input.clientKey.trim() === '') ||
        (input.clientCert.trim() !== '' && input.clientKey.trim() !== ''),
      '客户端证书路径与私钥路径需同时填写或同时留空',
    ),
    ['clientCert'],
  ),
)

export type ProfileFieldErrorKey =
  | 'name'
  | 'host'
  | 'port'
  | 'username'
  | 'password'
  | 'trustedCert'
  | 'extraRoutes'
  | 'clientCert'

export type ProfileFieldErrors = Partial<Record<ProfileFieldErrorKey, string>>

export interface ProfileValidationResult {
  valid: boolean
  input: ProfileInput
  errors: ProfileFieldErrors
  firstError: string
}

export function validateProfileDraft(draft: ProfileDraft): ProfileValidationResult {
  const candidate = toProfileInput(draft)
  const parsed = v.safeParse(ProfileInputSchema, candidate)
  const errors: ProfileFieldErrors = {}
  let firstError = ''

  if (!parsed.success) {
    for (const issue of parsed.issues) {
      const firstPath = issue.path?.[0]?.key
      const message = issue.message || '输入格式有误'
      if (!firstError) firstError = message
      if (
        typeof firstPath === 'string' &&
        (firstPath === 'name' ||
          firstPath === 'host' ||
          firstPath === 'port' ||
          firstPath === 'username' ||
          firstPath === 'password' ||
          firstPath === 'trustedCert' ||
          firstPath === 'extraRoutes' ||
          firstPath === 'clientCert') &&
        !errors[firstPath]
      ) {
        errors[firstPath] = message
      }
    }
  }

  if (draft.auth === 'password' && !draft.hasPassword && candidate.password.length === 0) {
    const pwdError = '账号密码模式下请填写登录密码'
    errors.password ??= pwdError
    if (!firstError) firstError = pwdError
  }

  if (Object.keys(errors).length === 0 && parsed.success) {
    return {
      valid: true,
      input: parsed.output,
      errors: {},
      firstError: '',
    }
  }

  return {
    valid: false,
    input: parsed.success ? parsed.output : candidate,
    errors,
    firstError: firstError || '表单校验未通过',
  }
}
