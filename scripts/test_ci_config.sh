#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ci_workflow="$repo_root/.github/workflows/ci.yml"
release_workflow="$repo_root/.github/workflows/release.yml"
release_assets_workflow="$repo_root/.github/workflows/release-assets.yml"
rolling_workflow="$repo_root/.github/workflows/rolling.yml"
dependabot_config="$repo_root/.github/dependabot.yml"
windows_wintun_setup="$repo_root/scripts/prepare_windows_wintun.ps1"

locked_npm_sources="$(
  sed -n 's/^[[:space:]]*"resolved":[[:space:]]*"\(https:\/\/[^/]*\)\/.*/\1/p' \
    "$repo_root/frontend/package-lock.json" | sort -u
)"
if [ "$locked_npm_sources" != "https://registry.npmjs.org" ]; then
  echo "frontend/package-lock.json must resolve exclusively against https://registry.npmjs.org; got: $locked_npm_sources"
  exit 1
fi

config_version="$(sed -n 's/^[[:space:]]*version:[[:space:]]*"\([^"]*\)".*/\1/p' "$repo_root/build/config.yml" | head -n 1)"
if [[ ! "$config_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "build/config.yml must define a valid semantic version (MAJOR.MINOR.PATCH); got: ${config_version:-<missing>}"
  exit 1
fi

if ! grep -Fq '//go:embed all:frontend/dist all:assets/icons build/config.yml' "$repo_root/main.go" ||
  ! grep -Fq 'loadVersion(assets)' "$repo_root/internal/app/app.go"; then
  echo "application runtime version must be read directly from embedded build/config.yml"
  exit 1
fi

if ! grep -Fq 'wails3 task common:update:build-assets' "$release_assets_workflow"; then
  echo "release asset builds must sync platform manifests from build/config.yml via wails3 task common:update:build-assets"
  exit 1
fi

for dmg_asset in \
  "$repo_root/scripts/package_macos_dmg.sh" \
  "$repo_root/build/darwin/dmg/settings.py" \
  "$repo_root/build/darwin/dmg/background.png" \
  "$repo_root/build/darwin/dmg/background@2x.png"; do
  if [ ! -f "$dmg_asset" ]; then
    echo "missing macOS DMG packaging asset: ${dmg_asset#"$repo_root/"}"
    exit 1
  fi
done

if ! grep -Fq 'bash scripts/package_macos_dmg.sh' "$release_assets_workflow" ||
  ! grep -Fq 'symlinks = {"Applications": "/Applications"}' "$repo_root/build/darwin/dmg/settings.py"; then
  echo "macOS DMG builds must use scripts/package_macos_dmg.sh with an Applications drop target"
  exit 1
fi

for workflow in "$ci_workflow" "$release_workflow" "$release_assets_workflow" "$rolling_workflow"; do
  if [ ! -f "$workflow" ]; then
    echo "missing workflow: ${workflow#"$repo_root/"}"
    exit 1
  fi

  if grep -E '^[[:space:]]*uses:' "$workflow" |
    grep -Ev 'uses:[[:space:]]+\./\.github/workflows/(ci|release-assets)\.yml$' |
    grep -Ev '@[0-9a-f]{40}([[:space:]]+#.*)?$' >/dev/null; then
    echo "all third-party actions must be pinned to a full commit SHA: ${workflow#"$repo_root/"}"
    exit 1
  fi
done

for artifact_action in upload-artifact download-artifact; do
  action_pin_count="$(
    grep -hE "uses: actions/$artifact_action@[0-9a-f]{40}" \
      "$ci_workflow" "$release_workflow" "$release_assets_workflow" "$rolling_workflow" |
      sed -E "s/.*actions\/$artifact_action@([0-9a-f]{40}).*/\1/" |
      sort -u |
      wc -l |
      tr -d '[:space:]'
  )"
  if [ "$action_pin_count" -ne 1 ]; then
    echo "CI and Release must use one consistent actions/$artifact_action version"
    exit 1
  fi
done

if ! grep -Fq 'artifact-roundtrip:' "$ci_workflow" ||
  ! grep -Fq 'actions/upload-artifact@' "$ci_workflow" ||
  ! grep -Fq 'actions/download-artifact@' "$ci_workflow" ||
  ! grep -Fq 'if-no-files-found: error' "$ci_workflow" ||
  ! grep -Fq 'cmp --silent' "$ci_workflow"; then
  echo "ordinary CI must verify an uploaded artifact can be downloaded unchanged"
  exit 1
fi

if ! grep -q "contents: read" "$ci_workflow" || grep -q "contents: write" "$ci_workflow"; then
  echo "ordinary CI must use read-only repository permissions"
  exit 1
fi

if ! grep -Fq 'workflow_call:' "$ci_workflow"; then
  echo "ordinary CI must be reusable as the release quality gate"
  exit 1
fi

release_ci_job="$(awk '/^  ci:/,/^  build:/' "$release_workflow")"
if ! grep -Fq 'uses: ./.github/workflows/ci.yml' <<<"$release_ci_job"; then
  echo "release workflow must reuse ordinary CI before building release assets"
  exit 1
fi

release_build_job="$(awk '/^  build:/,/^  publish:/' "$release_workflow")"
if ! grep -Fq -- '- validate' <<<"$release_build_job" ||
  ! grep -Fq -- '- ci' <<<"$release_build_job" ||
  ! grep -Fq 'uses: ./.github/workflows/release-assets.yml' <<<"$release_build_job" ||
  ! grep -Fq 'retention_days: 7' <<<"$release_build_job"; then
  echo "formal releases must call the shared build after tag validation and CI"
  exit 1
fi

for required in \
  "tags:" \
  "'v*'" \
  "contents: read" \
  "contents: write" \
  "GH_REPO" \
  "SHA256SUMS.txt" \
  "--verify-tag" \
  "--draft"; do
  if ! grep -Fq -- "$required" "$release_workflow"; then
    echo "release workflow is missing required contract: $required"
    exit 1
  fi
done

if [ ! -f "$windows_wintun_setup" ] ||
  ! grep -Fq 'scripts/prepare_windows_wintun.ps1' "$release_assets_workflow" ||
  ! grep -Fq '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51' "$windows_wintun_setup" ||
  ! grep -Fq 'Expand-Archive' "$windows_wintun_setup"; then
  echo "Windows releases must verify and extract Wintun DLLs via scripts/prepare_windows_wintun.ps1"
  exit 1
fi

if grep -Eq 'PULL_TOKEN|REPO_URL|@main' "$release_workflow" "$release_assets_workflow" "$rolling_workflow"; then
  echo "release workflow must not clone another repository or use floating main actions"
  exit 1
fi

publish_job="$(awk '/^  publish:/,/^  cleanup-rolling:/' "$release_workflow")"
for expected_asset in \
  '"tunnelkit-$APP_VERSION-macos-universal.dmg"' \
  '"tunnelkit-$APP_VERSION-macos-arm64.dmg"' \
  '"tunnelkit-$APP_VERSION-macos-amd64.dmg"' \
  '"tunnelkit-$APP_VERSION-windows-amd64.zip"' \
  '"tunnelkit-$APP_VERSION-windows-arm64.zip"' \
  '"tunnelkit-$APP_VERSION-linux-amd64.tar.gz"'; do
  if ! grep -Fq "$expected_asset" <<<"$publish_job"; then
    echo "published releases must verify asset: $expected_asset"
    exit 1
  fi
done

if ! grep -Fq 'if [ "$asset_count" -ne 7 ]; then' <<<"$publish_job" ||
  ! grep -Fq 'expected 7' <<<"$publish_job"; then
  echo "published releases must contain all six platform archives plus SHA256SUMS.txt (7 total)"
  exit 1
fi

if [ ! -f "$dependabot_config" ] ||
  ! grep -Fq 'package-ecosystem: "github-actions"' "$dependabot_config" ||
  ! grep -Fq 'interval: "weekly"' "$dependabot_config" ||
  ! grep -Fq 'groups:' "$dependabot_config" ||
  ! grep -Fq 'github-actions:' "$dependabot_config" ||
  ! grep -Fq -- '- "*"' "$dependabot_config"; then
  echo "Dependabot must group reviewed weekly updates for pinned GitHub Actions"
  exit 1
fi
