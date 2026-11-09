const SENSITIVE_KV_PATTERN =
  /\b(password|passwd|secret|token|cookie|svpn_cookie|otp|authorization)(\s*[=:]\s*)([^\s,;]+)/gi

export function redactSensitiveText(text: string): string {
  if (!text) return ''
  return text.replace(SENSITIVE_KV_PATTERN, '$1$2[REDACTED]')
}
