import type { UpdateInfo, UpdatePhase, UpdateProgress } from '@/types/system'
import { formatBytes } from '@/utils/format'

export interface UpdateDialogViewModel {
  title: string
  description: string
  metaText: string
  progressPercent: number
  isIndeterminate: boolean
  isLocked: boolean
}

export function buildUpdateDialogViewModel(input: {
  phase: UpdatePhase
  updateInfo: UpdateInfo | null
  progress: UpdateProgress | null
  vpnActive: boolean
}): UpdateDialogViewModel {
  const { phase, updateInfo, progress, vpnActive } = input
  const canSelfUpdate = Boolean(updateInfo?.canSelfUpdate)
  const isLocked = phase === 'downloading' || phase === 'verifying' || phase === 'installing'

  const rawTotal = progress?.totalBytes || updateInfo?.assetSize || 0
  const totalBytes = Number.isFinite(rawTotal) && rawTotal > 0 ? rawTotal : 0
  const rawDownloaded = progress?.downloadedBytes ?? 0
  const downloadedBytes = Number.isFinite(rawDownloaded) && rawDownloaded > 0 ? rawDownloaded : 0

  const rawPct = progress?.percent ?? 0
  const progressPercent =
    phase === 'verifying' || phase === 'ready' || phase === 'installing'
      ? 100
      : Number.isFinite(rawPct)
        ? Math.max(0, Math.min(100, Math.round(rawPct)))
        : 0

  const isIndeterminate =
    phase === 'verifying' || phase === 'installing' || (phase === 'downloading' && totalBytes <= 0)

  const title = (() => {
    switch (phase) {
      case 'downloading':
        return progress?.message || '正在下载更新…'
      case 'verifying':
        return progress?.message || '正在校验更新…'
      case 'ready':
        return '可以开始安装了'
      case 'installing':
        return progress?.message || '正在安装更新…'
      default:
        return '发现新版本'
    }
  })()

  const description = (() => {
    switch (phase) {
      case 'downloading':
        return '正在下载更新安装包'
      case 'verifying':
        return '正在校验更新包完整性'
      case 'ready':
        return vpnActive ? '安装前将自动断开当前 VPN 并重启应用' : '更新包已通过完整性校验'
      case 'installing':
        return '正在安装更新，完成后将自动重启'
      default:
        return canSelfUpdate ? '发现可安装的新版本' : '请前往发布页下载安装包完成更新'
    }
  })()

  const metaText = (() => {
    switch (phase) {
      case 'downloading':
        if (totalBytes > 0) {
          return `${formatBytes(downloadedBytes)} / ${formatBytes(totalBytes)}`
        }
        if (downloadedBytes > 0) {
          return `已下载 ${formatBytes(downloadedBytes)}`
        }
        return '正在连接更新源…'
      case 'verifying':
        return '请稍候'
      case 'ready':
        return vpnActive ? '将先断开当前 VPN' : '更新包已就绪'
      case 'installing':
        return progress?.stage === 'restarting' ? '请稍候' : '完成后将自动重启'
      default:
        return ''
    }
  })()

  return {
    title,
    description,
    metaText,
    progressPercent,
    isIndeterminate,
    isLocked,
  }
}
