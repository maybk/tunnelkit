import * as v from 'valibot'
import { NonEmptyTrimmedStringSchema } from './primitives'

export const OtpCodeSchema = NonEmptyTrimmedStringSchema('请输入一次性验证码（OTP）')

export function validateOtpCode(raw: string): { valid: boolean; code: string; error: string } {
  const result = v.safeParse(OtpCodeSchema, raw)
  if (result.success) {
    return { valid: true, code: result.output, error: '' }
  }
  return {
    valid: false,
    code: raw.trim(),
    error: result.issues[0]?.message || '请输入一次性验证码（OTP）',
  }
}
