#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage:
  bash scripts/render_release_notes.sh --version VERSION [options]
  bash scripts/render_release_notes.sh --rolling --version VERSION --app-version APP_VERSION --source-sha SHA [options]

Options:
  --version VERSION         Semantic version MAJOR.MINOR.PATCH in CHANGELOG.md (required)
  --changelog PATH          Path to CHANGELOG.md (default: CHANGELOG.md)
  --repo OWNER/NAME         GitHub repository slug (default: maybk/tunnelkit)
  --server-url URL          GitHub server base URL (default: https://github.com)
  --rolling                 Render Rolling prerelease notes instead of a formal release
  --app-version VERSION     Embedded application version for Rolling metadata (required with --rolling)
  --source-sha SHA          Full 40-character commit SHA for Rolling assets and metadata (required with --rolling)
  --built-at TIMESTAMP      UTC build timestamp for Rolling metadata (default: current UTC time)
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
extractor="$script_dir/extract_release_notes.sh"

version=""
changelog="CHANGELOG.md"
repo="maybk/tunnelkit"
server_url="https://github.com"
rolling=false
app_version=""
source_sha=""
built_at=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || { usage; exit 64; }
      version="$2"
      shift 2
      ;;
    --changelog)
      [ "$#" -ge 2 ] || { usage; exit 64; }
      changelog="$2"
      shift 2
      ;;
    --repo)
      [ "$#" -ge 2 ] || { usage; exit 64; }
      repo="$2"
      shift 2
      ;;
    --server-url)
      [ "$#" -ge 2 ] || { usage; exit 64; }
      server_url="$2"
      while [[ "$server_url" == */ ]]; do
        server_url="${server_url%/}"
      done
      shift 2
      ;;
    --rolling)
      rolling=true
      shift
      ;;
    --app-version)
      [ "$#" -ge 2 ] || { usage; exit 64; }
      app_version="$2"
      shift 2
      ;;
    --source-sha)
      [ "$#" -ge 2 ] || { usage; exit 64; }
      source_sha="$2"
      shift 2
      ;;
    --built-at)
      [ "$#" -ge 2 ] || { usage; exit 64; }
      built_at="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 64
      ;;
  esac
done

if [ -z "$version" ]; then
  usage
  exit 64
fi

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Release version must match MAJOR.MINOR.PATCH: $version" >&2
  exit 1
fi

if [[ ! "$repo" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]; then
  echo "Invalid repository slug: $repo" >&2
  exit 1
fi

if [[ ! "$server_url" =~ ^https?://[^[:space:]]+$ ]]; then
  echo "Invalid GitHub server URL: $server_url" >&2
  exit 1
fi

if [ "$rolling" = true ]; then
  if [ -z "$app_version" ] || [ -z "$source_sha" ]; then
    echo "--rolling requires both --app-version and --source-sha" >&2
    exit 64
  fi
  if [[ ! "$app_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-[A-Za-z0-9._-]+$ ]]; then
    echo "Rolling app version must match MAJOR.MINOR.PATCH-PRERELEASE: $app_version" >&2
    exit 1
  fi
  if [[ ! "$source_sha" =~ ^[0-9a-f]{40}$ ]]; then
    echo "Rolling source SHA must be a 40-character lowercase hex digest: $source_sha" >&2
    exit 1
  fi
  if [ -z "$built_at" ]; then
    built_at="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  fi
  short_sha="${source_sha:0:12}"
  release_tag="v${version}-pre"
  asset_prefix="tunnelkit-${version}-rolling-${short_sha}"
else
  if [ -n "$app_version" ] || [ -n "$source_sha" ] || [ -n "$built_at" ]; then
    echo "--app-version, --source-sha, and --built-at are only valid with --rolling" >&2
    exit 64
  fi
  release_tag="v${version}"
  asset_prefix="tunnelkit-${version}"
fi

set +e
changelog_body="$(bash "$extractor" "$version" "$changelog")"
extract_status="$?"
set -e
if [ "$extract_status" -ne 0 ]; then
  exit "$extract_status"
fi

download_base="${server_url}/${repo}/releases/download/${release_tag}"
windows_logo='data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTAgMGgxMS40djExLjRIMHptMTIuNiAwSDI0djExLjRIMTIuNnpNMCAxMi42aDExLjRWMjRIMHptMTIuNiAwSDI0VjI0SDEyLjZ6Ii8%2BPC9zdmc%2B'

badge() {
  local alt="$1"
  local badge_url="$2"
  local asset_suffix="$3"
  printf '[![%s](%s)](%s/%s-%s)' "$alt" "$badge_url" "$download_base" "$asset_prefix" "$asset_suffix"
}

win_x64="$(badge 'Windows Setup x64' "https://img.shields.io/badge/Setup-x64-0078D4?logo=${windows_logo}" 'windows-amd64-setup.exe')"
win_arm64="$(badge 'Windows Setup ARM64' "https://img.shields.io/badge/Setup-ARM64-4A90D9?logo=${windows_logo}" 'windows-arm64-setup.exe')"
mac_arm64="$(badge 'macOS DMG Apple Silicon' 'https://img.shields.io/badge/DMG-Apple%20Silicon-1D1D1F?logo=apple&logoColor=white' 'macos-arm64.dmg')"
mac_amd64="$(badge 'macOS DMG Intel' 'https://img.shields.io/badge/DMG-Intel-0071E3?logo=apple&logoColor=white' 'macos-amd64.dmg')"
deb_x64="$(badge 'Linux DEB x64' 'https://img.shields.io/badge/DEB-x64-A81D33?logo=debian&logoColor=white' 'linux-amd64.deb')"
deb_arm64="$(badge 'Linux DEB ARM64' 'https://img.shields.io/badge/DEB-ARM64-C94F65?logo=debian&logoColor=white' 'linux-arm64.deb')"
rpm_x64="$(badge 'Linux RPM x64' 'https://img.shields.io/badge/RPM-x64-B45309?logo=redhat&logoColor=white' 'linux-amd64.rpm')"
rpm_arm64="$(badge 'Linux RPM ARM64' 'https://img.shields.io/badge/RPM-ARM64-D97706?logo=redhat&logoColor=white' 'linux-arm64.rpm')"

if [ "$rolling" = true ]; then
  printf '%s\n%s\n\n' \
    '> [!WARNING]' \
    '> 此版本为自动构建的测试版，可能不稳定，请勿用于重要环境。'
fi

cat <<EOF
## 更新内容

${changelog_body}

## 下载安装

| 系统 | 安装包下载 |
| :--- | :--- |
| **Windows** | ${win_x64} ${win_arm64} |
| **macOS** | ${mac_arm64} ${mac_amd64} |
| **Linux** | ${deb_x64} ${deb_arm64}<br>${rpm_x64} ${rpm_arm64} |
EOF

if [ "$rolling" = true ]; then
  cat <<EOF

## 构建信息

- 预览版本：\`${version}\`
- 应用版本：\`${app_version}\`
- 提交：[\`${short_sha}\`](${server_url}/${repo}/commit/${source_sha})
- 构建时间：\`${built_at}\`
EOF
fi
