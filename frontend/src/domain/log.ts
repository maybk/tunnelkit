import { MAX_RENDERED_LOG_LINES } from '@/constants/log'
import type { LogLevelFilter, LogLine, LogLineLevel } from '@/types/log'
import { redactSensitiveText } from '@/utils/redact'

const STRUCTURED_LOG_RE =
  /^(\d{4}[-/]\d{2}[-/]\d{2}[T\s]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+(?:\[?(INFO|WARN|WARNING|ERROR|ERR|DEBUG)\]?)\s*:?\s*(.*)$/i
const LEVEL_TAG_RE =
  /\b(level=(INFO|WARN|WARNING|ERROR|DEBUG)|\[(INFO|WARN|WARNING|ERROR|DEBUG)\])/i

function normalizeLineLevel(rawLevel: string): LogLineLevel {
  const upper = rawLevel.toUpperCase()
  if (upper === 'WARN' || upper === 'WARNING') return 'WARN'
  if (upper === 'ERROR' || upper === 'ERR') return 'ERROR'
  if (upper === 'DEBUG') return 'DEBUG'
  if (upper === 'INFO') return 'INFO'
  return 'PLAIN'
}

export function parseLogLine(raw: string, index: number): LogLine {
  const matched = STRUCTURED_LOG_RE.exec(raw)
  if (matched) {
    return {
      index,
      raw,
      timestamp: matched[1] ?? '',
      level: normalizeLineLevel(matched[2] ?? ''),
      message: matched[3] || raw,
    }
  }

  const tagMatch = LEVEL_TAG_RE.exec(raw)
  if (tagMatch) {
    const levelText = tagMatch[2] || tagMatch[3] || ''
    return {
      index,
      raw,
      timestamp: '',
      level: normalizeLineLevel(levelText),
      message: raw,
    }
  }

  return {
    index,
    raw,
    timestamp: '',
    level: 'PLAIN',
    message: raw,
  }
}

export function parseLogContent(
  content: string,
  maxLines: number = MAX_RENDERED_LOG_LINES,
): LogLine[] {
  if (!content) return []
  const rawLines = content.split(/\r?\n/)
  if (rawLines.length > 0 && rawLines[rawLines.length - 1] === '') {
    rawLines.pop()
  }
  const sliced = rawLines.length > maxLines ? rawLines.slice(rawLines.length - maxLines) : rawLines
  const offset = Math.max(0, rawLines.length - sliced.length)
  return sliced.map((line, idx) => parseLogLine(line, offset + idx + 1))
}

export function filterLogLines(
  lines: readonly LogLine[],
  levelFilter: LogLevelFilter,
  keyword: string,
): LogLine[] {
  const normalizedKeyword = keyword.trim().toLowerCase()
  return lines.filter((line) => {
    if (levelFilter !== 'ALL' && line.level !== levelFilter) {
      return false
    }
    if (normalizedKeyword && !line.raw.toLowerCase().includes(normalizedKeyword)) {
      return false
    }
    return true
  })
}

export function formatLogsForExport(lines: readonly LogLine[]): string {
  return redactSensitiveText(lines.map((line) => line.raw).join('\n'))
}
