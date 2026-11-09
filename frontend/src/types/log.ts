import type { LOG_LEVEL_FILTERS, LOG_LINE_LEVELS } from '@/constants/log'

export type LogLevelFilter = (typeof LOG_LEVEL_FILTERS)[number]
export type LogLineLevel = (typeof LOG_LINE_LEVELS)[number]

export interface LogLine {
  index: number
  raw: string
  timestamp: string
  level: LogLineLevel
  message: string
}

export interface LogViewSnapshot {
  profileId: string
  content: string
  exists: boolean
  updatedAt: string
}
