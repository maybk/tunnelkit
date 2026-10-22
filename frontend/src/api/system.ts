import { Browser } from '@wailsio/runtime'
import { SystemAPI } from '../../bindings/tunnelkit/internal/api'
import type * as api from '../../bindings/tunnelkit/internal/api'

export type SystemInfo = api.SystemInfo
export type UpdateInfo = api.UpdateInfo

export const GITHUB_REPO_SLUG = 'maybk/tunnelkit'
export const GITHUB_REPO_LABEL = `github.com/${GITHUB_REPO_SLUG}`
export const GITHUB_REPO_URL = `https://${GITHUB_REPO_LABEL}`

const RELEASE_TAG_URL_RE = new RegExp(
  `^https://github\\.com/${GITHUB_REPO_SLUG}/releases/tag/[^/?#]+$`,
  'i',
)

export const isValidReleaseUrl = (url: string | undefined | null): url is string =>
  Boolean(url && RELEASE_TAG_URL_RE.test(url.trim()))

export const getSystemInfo = (): Promise<SystemInfo> => SystemAPI.Info()

export const checkSystemUpdate = (): Promise<UpdateInfo> => SystemAPI.CheckUpdate()

export const openGitHubRepo = async (): Promise<void> => {
  await Browser.OpenURL(GITHUB_REPO_URL)
}

export const openReleaseUrl = async (url: string): Promise<void> => {
  const target = url.trim()
  if (!isValidReleaseUrl(target)) {
    throw new Error('更新下载页面地址无效')
  }
  await Browser.OpenURL(target)
}
