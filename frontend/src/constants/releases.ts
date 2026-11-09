export const GITHUB_REPO_SLUG = 'maybk/tunnelkit'
export const GITHUB_REPO_LABEL = `github.com/${GITHUB_REPO_SLUG}`
export const GITHUB_REPO_URL = `https://${GITHUB_REPO_LABEL}`
export const GITHUB_RELEASES_URL = `${GITHUB_REPO_URL}/releases`

export const ALLOWED_RELEASE_HOSTS = ['github.com'] as const
export const HELPER_BUILD_COMMAND = 'go build -o bin/tunnelkit-helper ./cmd/tunnelkit-helper'
