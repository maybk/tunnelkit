export const formatDurationSince = (isoTime?: string, nowMs: number = Date.now()): string => {
  if (!isoTime) return ''
  const started = Date.parse(isoTime)
  if (Number.isNaN(started)) return ''

  const totalSec = Math.max(0, Math.floor((nowMs - started) / 1000))
  if (totalSec < 60) return `${totalSec} 秒`

  const minutes = Math.floor(totalSec / 60)
  if (minutes < 60) return `${minutes} 分钟`

  const hours = Math.floor(minutes / 60)
  const remMin = minutes % 60
  return remMin > 0 ? `${hours} 小时 ${remMin} 分钟` : `${hours} 小时`
}

export const formatClockTime = (isoTime?: string): string => {
  if (!isoTime) return '—'
  const date = new Date(isoTime)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export const formatBytes = (bytes?: number): string => {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let val = bytes
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx++
  }
  return idx === 0 ? `${Math.round(val)} ${units[idx]}` : `${val.toFixed(1)} ${units[idx]}`
}

/** 将 Wails 调用抛出的错误转为用户可读文本。 */
export const errorMessage = (err: unknown): string => {
  if (err instanceof Error && err.message) return err.message
  if (typeof err === 'string' && err) return err
  return '操作失败，请重试'
}
