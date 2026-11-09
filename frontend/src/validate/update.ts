import { ALLOWED_RELEASE_HOSTS, GITHUB_REPO_SLUG } from '@/constants/releases'

const RELEASE_PATH_PREFIX = `/${GITHUB_REPO_SLUG}/releases`

export function isTrustedReleaseUrl(rawUrl?: string | null): rawUrl is string {
  if (!rawUrl) return false
  try {
    const parsed = new URL(rawUrl.trim())
    if (parsed.protocol !== 'https:' || parsed.username || parsed.password || parsed.port)
      return false
    const host = parsed.hostname.toLowerCase()
    if (!(ALLOWED_RELEASE_HOSTS as readonly string[]).includes(host)) return false
    const path = parsed.pathname.toLowerCase()
    const prefix = RELEASE_PATH_PREFIX.toLowerCase()
    return path === prefix || path.startsWith(`${prefix}/`)
  } catch {
    return false
  }
}

export function isTrustedRepositoryUrl(rawUrl?: string | null): rawUrl is string {
  if (!rawUrl) return false
  try {
    const parsed = new URL(rawUrl.trim())
    return (
      parsed.protocol === 'https:' &&
      parsed.hostname.toLowerCase() === 'github.com' &&
      !parsed.username &&
      !parsed.password &&
      !parsed.port &&
      (parsed.pathname === `/${GITHUB_REPO_SLUG}` || parsed.pathname === `/${GITHUB_REPO_SLUG}/`)
    )
  } catch {
    return false
  }
}
