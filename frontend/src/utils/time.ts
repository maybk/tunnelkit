export function formatDurationSeconds(totalSeconds: number): string {
  const safe = Math.max(0, Math.floor(Number(totalSeconds) || 0))
  const hours = String(Math.floor(safe / 3600)).padStart(2, '0')
  const minutes = String(Math.floor((safe % 3600) / 60)).padStart(2, '0')
  const seconds = String(safe % 60).padStart(2, '0')
  return `${hours}:${minutes}:${seconds}`
}

export function formatElapsedSince(connectedAt?: string, nowMs: number = Date.now()): string {
  if (!connectedAt) return '00:00:00'
  const startedAt = new Date(connectedAt).getTime()
  if (Number.isNaN(startedAt)) return '00:00:00'
  const diffSeconds = Math.max(0, Math.floor((nowMs - startedAt) / 1000))
  return formatDurationSeconds(diffSeconds)
}

export function formatDateTime(iso?: string): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString('zh-CN', { hour12: false })
}

export function formatDurationSince(isoTime?: string, nowMs = Date.now()): string {
  if (!isoTime || Number.isNaN(Date.parse(isoTime))) return ''
  const total = Math.max(0, Math.floor((nowMs - Date.parse(isoTime)) / 1000))
  if (total < 60) return total + ' 秒'
  const minutes = Math.floor(total / 60)
  if (minutes < 60) return minutes + ' 分钟'
  const hours = Math.floor(minutes / 60),
    rest = minutes % 60
  return rest ? hours + ' 小时 ' + rest + ' 分钟' : hours + ' 小时'
}
export function formatClockTime(iso?: string): string {
  if (!iso || Number.isNaN(Date.parse(iso))) return '—'
  return new Date(iso).toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}
