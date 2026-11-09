export function formatErrorMessage(
  err: unknown,
  fallback: string = '操作失败，请稍后重试',
): string {
  if (!err) return fallback
  if (typeof err === 'string') {
    const trimmed = err.trim()
    return trimmed || fallback
  }
  if (typeof err === 'object') {
    const record = err as Record<string, unknown>
    if (typeof record.message === 'string' && record.message.trim()) {
      return record.message.trim()
    }
    if (typeof record.error === 'string' && record.error.trim()) {
      return record.error.trim()
    }
    return fallback
  }
  if (typeof err === 'number' || typeof err === 'boolean' || typeof err === 'bigint') {
    return String(err)
  }
  return fallback
}
