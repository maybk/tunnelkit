#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ci_workflow="$repo_root/.github/workflows/ci.yml"
release_workflow="$repo_root/.github/workflows/release.yml"
release_assets_workflow="$repo_root/.github/workflows/release-assets.yml"
rolling_workflow="$repo_root/.github/workflows/rolling.yml"
dependabot_config="$repo_root/.github/dependabot.yml"
windows_wintun_setup="$repo_root/scripts/prepare_windows_wintun.ps1"
windows_helper_smoke="$repo_root/scripts/smoke_windows_helper.ps1"

if [ ! -f "$repo_root/frontend/pnpm-lock.yaml" ] || [ -f "$repo_root/frontend/package-lock.json" ]; then
  echo "frontend must use pnpm-lock.yaml exclusively"
  exit 1
fi
custom_tarball_sources="$(
  sed -n 's/.*tarball:[[:space:]]*\(https:\/\/[^/}]*\)\/.*/\1/p' \
    "$repo_root/frontend/pnpm-lock.yaml" | grep -v '^https://registry\.npmjs\.org$' | sort -u || true
)"
if [ -n "$custom_tarball_sources" ]; then
  echo "frontend/pnpm-lock.yaml must not contain non-official tarball URLs; got: $custom_tarball_sources"
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

if [ ! -f "$windows_helper_smoke" ] ||
  ! grep -Fq 'scripts/smoke_windows_helper.ps1 -InstallerPath "dist/tunnelkit-$env:APP_VERSION-windows-amd64-setup.exe"' "$release_assets_workflow" ||
  ! grep -Fq 'scripts/smoke_windows_helper.ps1 -InstallerPath "dist/tunnelkit-$env:APP_VERSION-windows-arm64-setup.exe"' "$release_assets_workflow" ||
  ! grep -Fq 'TestSmokeInstalledWindowsHelperIsCurrent' "$windows_helper_smoke" ||
  ! grep -Fq 'TestSmokeLaunchInstalledWindowsHelper' "$windows_helper_smoke"; then
  echo "Windows releases must smoke-test both amd64 and arm64 NSIS installers and helper SCM IPC via scripts/smoke_windows_helper.ps1"
  exit 1
fi

windows_taskfile="$repo_root/build/windows/Taskfile.yml"
if [ ! -f "$windows_taskfile" ] ||
  [ "$(grep -Fc 'makensis -INPUTCHARSET UTF8' "$windows_taskfile")" -ne 2 ] ||
  grep -E '^[[:space:]]*makensis([[:space:]]|$)' "$windows_taskfile" | grep -Fv 'makensis -INPUTCHARSET UTF8' >/dev/null; then
  echo "build/windows/Taskfile.yml must invoke makensis with -INPUTCHARSET UTF8 on all host platforms"
  exit 1
fi

nsis_script="$repo_root/build/windows/nsis/project.nsi"
if [ ! -f "$nsis_script" ] ||
  ! grep -Fq 'Unicode true' "$nsis_script" ||
  ! grep -Fq '!include "MUI2.nsh"' "$nsis_script" ||
  ! grep -Fq '!insertmacro MUI_UNPAGE_CONFIRM' "$nsis_script" ||
  ! grep -Fq '!define MUI_FINISHPAGE_RUN_FUNCTION "LaunchAsDesktopUser"' "$nsis_script" ||
  ! grep -Fq 'ShowInstDetails hide' "$nsis_script" ||
  ! grep -Fq 'ShowUnInstDetails hide' "$nsis_script" ||
  ! grep -Fq '!define WAILS_WIN10_REQUIRED "$(MSG_WIN10_REQUIRED)"' "$nsis_script" ||
  ! grep -Fq '!define WAILS_ARCHITECTURE_NOT_SUPPORTED "$(MSG_ARCH_NOT_SUPPORTED)"' "$nsis_script" ||
  ! grep -Fq '!define WAILS_INSTALL_WEBVIEW_DETAILPRINT "$(MSG_INSTALL_WEBVIEW2)"' "$nsis_script" ||
  grep -Fq 'MUI_FINISHPAGE_NOAUTOCLOSE' "$nsis_script" ||
  grep -Fq 'ShowInstDetails show' "$nsis_script" ||
  grep -Fq 'nsExec::ExecToLog' "$nsis_script"; then
  echo "Windows NSIS installer must use Unicode MUI2, localized Wails messages, clean hidden details without ExecToLog noise, uninstall confirmation, and non-elevated Explorer launch"
  exit 1
fi

nsis_en_line="$(grep -nF '!insertmacro MUI_LANGUAGE "English"' "$nsis_script" | head -n 1 | cut -d: -f1)"
nsis_zh_line="$(grep -nF '!insertmacro MUI_LANGUAGE "SimpChinese"' "$nsis_script" | head -n 1 | cut -d: -f1)"
if [ -z "$nsis_en_line" ] || [ -z "$nsis_zh_line" ] || [ "$nsis_en_line" -ge "$nsis_zh_line" ]; then
  echo "Windows NSIS installer must declare English before SimpChinese so non-Chinese systems default to English"
  exit 1
fi

LC_ALL=C awk '
  /^[[:space:]]*[#;]/ { next }
  /^[[:space:]]*LangString[[:space:]]+/ {
    id = $2
    lang = $3
    ids[id] = 1
    if (lang != "${LANG_ENGLISH}" && lang != "${LANG_SIMPCHINESE}") {
      printf("invalid LangString language token on line %d: %s\n", NR, lang) > "/dev/stderr"
      err = 1
    }
    if ($0 !~ /^[[:space:]]*LangString[[:space:]]+[A-Z0-9_]+[[:space:]]+\$\{LANG_(ENGLISH|SIMPCHINESE)\}[[:space:]]+"[^"]+"[[:space:]]*$/) {
      printf("malformed LangString syntax or unescaped quote on line %d: %s\n", NR, $0) > "/dev/stderr"
      err = 1
    }
    if (lang == "${LANG_ENGLISH}") {
      en[id]++
      if ($0 ~ /[^[:print:][:space:]]/) {
        printf("English LangString must be pure ASCII on line %d: %s\n", NR, id) > "/dev/stderr"
        err = 1
      }
    }
    if (lang == "${LANG_SIMPCHINESE}") {
      zh[id]++
    }
    next
  }
  {
    if ($0 ~ /[^[:print:][:space:]]/) {
      printf("hardcoded non-ASCII text outside LangString on line %d: %s\n", NR, $0) > "/dev/stderr"
      err = 1
    }
    line = $0
    while (match(line, /\$\([A-Za-z0-9_]+\)/)) {
      ref = substr(line, RSTART + 2, RLENGTH - 3)
      refs[ref]++
      line = substr(line, RSTART + RLENGTH)
    }
  }
  END {
    count = 0
    for (id in ids) {
      count++
      if (en[id] != 1 || zh[id] != 1) {
        printf("LangString %s must define exactly one English and one SimpChinese entry (got en=%d, zh=%d)\n", id, en[id], zh[id]) > "/dev/stderr"
        err = 1
      }
      if (!refs[id]) {
        printf("unused LangString: %s\n", id) > "/dev/stderr"
        err = 1
      }
    }
    for (ref in refs) {
      if (!ids[ref]) {
        printf("undefined LangString reference: $(%s)\n", ref) > "/dev/stderr"
        err = 1
      }
    }
    if (count < 12) {
      printf("expected at least 12 localized NSIS LangStrings, got %d\n", count) > "/dev/stderr"
      err = 1
    }
    exit err
  }
' "$nsis_script"

if grep -Eq 'PULL_TOKEN|REPO_URL|@main' "$release_workflow" "$release_assets_workflow" "$rolling_workflow"; then
  echo "release workflow must not clone another repository or use floating main actions"
  exit 1
fi

publish_job="$(awk '/^  publish:/,/^  cleanup-rolling:/' "$release_workflow")"
for expected_asset in \
  '"tunnelkit-$APP_VERSION-macos-universal.dmg"' \
  '"tunnelkit-$APP_VERSION-macos-arm64.dmg"' \
  '"tunnelkit-$APP_VERSION-macos-amd64.dmg"' \
  '"tunnelkit-$APP_VERSION-windows-amd64-setup.exe"' \
  '"tunnelkit-$APP_VERSION-windows-amd64.zip"' \
  '"tunnelkit-$APP_VERSION-windows-arm64-setup.exe"' \
  '"tunnelkit-$APP_VERSION-windows-arm64.zip"' \
  '"tunnelkit-$APP_VERSION-linux-amd64.tar.gz"' \
  '"tunnelkit-$APP_VERSION-linux-amd64.deb"' \
  '"tunnelkit-$APP_VERSION-linux-amd64.rpm"' \
  '"tunnelkit-$APP_VERSION-linux-arm64.tar.gz"' \
  '"tunnelkit-$APP_VERSION-linux-arm64.deb"' \
  '"tunnelkit-$APP_VERSION-linux-arm64.rpm"'; do
  if ! grep -Fq "$expected_asset" <<<"$publish_job"; then
    echo "published releases must verify asset: $expected_asset"
    exit 1
  fi
done

if ! grep -Fq 'if [ "$asset_count" -ne 14 ]; then' <<<"$publish_job" ||
  ! grep -Fq 'expected 14' <<<"$publish_job" ||
  ! grep -Fq 'all(.state == "uploaded")' <<<"$publish_job" ||
  ! grep -Fq 'bash scripts/render_package_manifests.sh "$APP_VERSION" release-files/SHA256SUMS.txt "$RUNNER_TEMP/manifests" "$release_date"' <<<"$publish_job" ||
  ! grep -Fq 'Psych.safe_load_file' <<<"$publish_job"; then
  echo "published releases must verify 14 uploaded draft assets and render/validate dated package manifests before publishing"
  exit 1
fi

nfpm_config="$repo_root/build/linux/nfpm/nfpm.yaml"
for required_nfpm in \
  '/usr/local/bin/tunnelkit' \
  '/usr/local/bin/tunnelkit-helper' \
  'libgtk-4-1' \
  'libwebkitgtk-6.0-4' \
  'iproute2' \
  'iproute' \
  'postinstall: "./build/linux/nfpm/scripts/postinstall.sh"' \
  'preremove: "./build/linux/nfpm/scripts/preremove.sh"'; do
  if ! grep -Fq "$required_nfpm" "$nfpm_config"; then
    echo "build/linux/nfpm/nfpm.yaml is missing required deb/rpm metadata: $required_nfpm"
    exit 1
  fi
done

for nfpm_script in \
  "$repo_root/build/linux/nfpm/scripts/postinstall.sh" \
  "$repo_root/build/linux/nfpm/scripts/preremove.sh"; do
  sh -n "$nfpm_script"
done

for required_runner in \
  'runs-on: macos-latest' \
  'runs-on: windows-latest' \
  'runs-on: windows-11-arm' \
  'runs-on: ubuntu-24.04' \
  'runs-on: ubuntu-24.04-arm' \
  'xvfb-run' \
  'go test ./...'; do
  if ! grep -Fq "$required_runner" "$release_assets_workflow"; then
    echo "release-assets.yml is missing required parallel runner or test step: $required_runner"
    exit 1
  fi
done

if grep -Fq 'pnpm --dir frontend install' "$release_assets_workflow" ||
  ! grep -Fq 'pnpm install --frozen-lockfile --registry=https://registry.npmjs.org' "$repo_root/build/Taskfile.yml"; then
  echo "release-assets.yml must delegate frontend dependency installation to build/Taskfile.yml to prevent concurrent pnpm install races"
  exit 1
fi

macos_arm64_job="$(awk '/^  macos-arm64:/,/^  macos-amd64:/' "$release_assets_workflow")"
macos_arm64_build_line="$(grep -nF 'Build and verify macOS arm64 bundle and DMG' <<<"$macos_arm64_job" | cut -d: -f1)"
macos_arm64_test_line="$(grep -nF 'Run native macOS arm64 analysis and test suite' <<<"$macos_arm64_job" | cut -d: -f1)"
if [ -z "$macos_arm64_build_line" ] || [ -z "$macos_arm64_test_line" ] || [ "$macos_arm64_build_line" -ge "$macos_arm64_test_line" ]; then
  echo "macos-arm64 must build frontend/dist and bundle before running go vet / go test"
  exit 1
fi

win_amd64_job="$(awk '/^  windows-amd64:/,/^  windows-arm64:/' "$release_assets_workflow")"
win_amd64_build_line="$(grep -nF 'Build Windows amd64 executables' <<<"$win_amd64_job" | cut -d: -f1)"
win_amd64_test_line="$(grep -nF 'Run native Windows amd64 analysis and test suite' <<<"$win_amd64_job" | cut -d: -f1)"
if [ -z "$win_amd64_build_line" ] || [ -z "$win_amd64_test_line" ] || [ "$win_amd64_build_line" -ge "$win_amd64_test_line" ]; then
  echo "windows-amd64 must build frontend/dist and executables before running go vet / go test"
  exit 1
fi

win_arm64_job="$(awk '/^  windows-arm64:/,/^  linux-amd64:/' "$release_assets_workflow")"
win_arm64_build_line="$(grep -nF 'Build Windows arm64 executables' <<<"$win_arm64_job" | cut -d: -f1)"
win_arm64_test_line="$(grep -nF 'Run native Windows arm64 analysis and test suite' <<<"$win_arm64_job" | cut -d: -f1)"
if [ -z "$win_arm64_build_line" ] || [ -z "$win_arm64_test_line" ] || [ "$win_arm64_build_line" -ge "$win_arm64_test_line" ]; then
  echo "windows-arm64 must build frontend/dist and executables before running go vet / go test"
  exit 1
fi

linux_amd64_job="$(awk '/^  linux-amd64:/,/^  linux-arm64:/' "$release_assets_workflow")"
amd64_build_line="$(grep -nF 'Build Linux amd64 tarball and deb package' <<<"$linux_amd64_job" | cut -d: -f1)"
amd64_test_line="$(grep -nF 'Run native Linux amd64 analysis and test suite' <<<"$linux_amd64_job" | cut -d: -f1)"
if [ -z "$amd64_build_line" ] || [ -z "$amd64_test_line" ] || [ "$amd64_build_line" -ge "$amd64_test_line" ]; then
  echo "linux-amd64 must build frontend/dist and packages before running go vet / go test"
  exit 1
fi

linux_arm64_job="$(awk '/^  linux-arm64:/,/^  merge:/' "$release_assets_workflow")"
build_line="$(grep -nF 'Build Linux arm64 tarball and deb package' <<<"$linux_arm64_job" | cut -d: -f1)"
test_line="$(grep -nF 'Run native Linux arm64 analysis and test suite' <<<"$linux_arm64_job" | cut -d: -f1)"
if [ -z "$build_line" ] || [ -z "$test_line" ] || [ "$build_line" -ge "$test_line" ]; then
  echo "linux-arm64 must build frontend/dist and packages before running go vet / go test"
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
