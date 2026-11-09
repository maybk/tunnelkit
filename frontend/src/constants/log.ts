import type { Tone } from './tone'

export const LOG_LEVEL_FILTERS = ['ALL', 'INFO', 'WARN', 'ERROR', 'DEBUG'] as const
export const LOG_LINE_LEVELS = ['INFO', 'WARN', 'ERROR', 'DEBUG', 'PLAIN'] as const

export const LOG_FILTER_OPTIONS = [
  { value: 'ALL', label: '全部' },
  { value: 'INFO', label: 'INFO' },
  { value: 'WARN', label: 'WARN' },
  { value: 'ERROR', label: 'ERROR' },
  { value: 'DEBUG', label: 'DEBUG' },
] as const

export const LOG_LEVEL_META: Record<
  (typeof LOG_LINE_LEVELS)[number],
  { label: string; tone: Tone }
> = {
  INFO: { label: 'INFO', tone: 'info' },
  WARN: { label: 'WARN', tone: 'warning' },
  ERROR: { label: 'ERROR', tone: 'danger' },
  DEBUG: { label: 'DEBUG', tone: 'neutral' },
  PLAIN: { label: 'LOG', tone: 'neutral' },
}

export const LOG_POLL_INTERVAL_MS = 1800
export const MAX_RENDERED_LOG_LINES = 2000
