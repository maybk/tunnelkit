export function formatBytes(bytes?: number): string {
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

export function formatCount(n?: number): string {
  const value = Math.max(0, Math.floor(Number(n ?? 0)))
  if (!Number.isFinite(value)) return '0'
  return value.toLocaleString('zh-CN')
}

export function formatRtt(ms?: number): string {
  const value = Number(ms ?? 0)
  if (!Number.isFinite(value) || value <= 0) return '—'
  return `${Math.round(value)} ms`
}

export function splitMultilineItems(raw: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const part of raw.split(/[\n,;]+/)) {
    const trimmed = part.trim()
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    out.push(trimmed)
  }
  return out
}

export function joinMultilineItems(items?: readonly string[]): string {
  return (items ?? []).filter(Boolean).join('\n')
}
