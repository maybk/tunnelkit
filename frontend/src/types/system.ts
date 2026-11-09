import type { Tone } from '@/constants/tone'

export interface UpdateInfo {
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  releaseUrl: string
  assetName: string
  assetUrl: string
  assetSize: number
  assetSha256: string
  canSelfUpdate: boolean
}

export interface UpdateProgress {
  stage: string
  downloadedBytes: number
  totalBytes: number
  percent: number
  message: string
}

export interface SystemInfo {
  appVersion: string
  platform: string
  helperReady: boolean
  helperPath: string
}

export interface NoticeItem {
  id: number
  tone: Tone
  title: string
  message?: string
}
