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

read_info_quoted_field() {
  local field="$1"
  local raw_line re
  raw_line="$(awk -v field="$field" '
    { sub(/\015$/, "") }
    /^[^[:space:]#]/ { in_info = ($1 == "info:") }
    in_info && $0 ~ ("^[[:space:]]+" field ":[[:space:]]*") { print; exit }
  ' "$repo_root/build/config.yml")"
  re="^[[:space:]]+${field}:[[:space:]]+\"([^\"]+)\"([[:space:]]+#.*)?[[:space:]]*$"
  if [[ ! "$raw_line" =~ $re ]]; then
    echo "info.${field} in build/config.yml must be a double-quoted scalar: ${raw_line:-<missing>}" >&2
    return 1
  fi
  printf '%s' "${BASH_REMATCH[1]}"
}

config_version="$(read_info_quoted_field version)"
if [[ ! "$config_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "build/config.yml must define a valid semantic version (MAJOR.MINOR.PATCH); got: ${config_version:-<missing>}" >&2
  exit 1
fi
config_product="$(read_info_quoted_field productName)"
if [ "$config_product" != "TunnelKit" ]; then
  echo "Invalid info.productName in build/config.yml: ${config_product:-<missing>}" >&2
  exit 1
fi
config_bundle_id="$(read_info_quoted_field productIdentifier)"
if [ "$config_bundle_id" != "com.tunnelkit.app" ]; then
  echo "Invalid info.productIdentifier in build/config.yml: ${config_bundle_id:-<missing>}" >&2
  exit 1
fi
config_desc="$(read_info_quoted_field description)"
if [ -z "$config_desc" ] || [ "${#config_desc}" -gt 80 ]; then
  echo "Invalid info.description length in build/config.yml (${#config_desc}): ${config_desc:-<missing>}" >&2
  exit 1
fi
case "$config_desc" in
  " "*|*" "|*.|*\"*|*\\*|*": "*|*" #"*)
    echo "info.description in build/config.yml must be a plain scalar without leading/trailing space, trailing period, or unsafe chars: $config_desc" >&2
    exit 1
    ;;
esac

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

if ! grep -Fq 'artifact-upload:' "$ci_workflow" ||
  ! grep -Fq 'artifact-roundtrip:' "$ci_workflow" ||
  ! grep -Fq 'needs: artifact-upload' "$ci_workflow" ||
  ! grep -Fq 'actions/upload-artifact@' "$ci_workflow" ||
  ! grep -Fq 'actions/download-artifact@' "$ci_workflow" ||
  ! grep -Fq 'if-no-files-found: error' "$ci_workflow" ||
  ! grep -Fq 'overwrite: true' "$ci_workflow" ||
  ! grep -Fq "steps.download.outcome == 'failure'" "$ci_workflow" ||
  ! grep -Fq 'cmp --silent' "$ci_workflow"; then
  echo "ordinary CI must verify cross-job artifact upload and download unchanged with overwrite and failure retry"
  exit 1
fi

for upload_workflow in "$ci_workflow" "$release_assets_workflow"; do
  upload_count="$(grep -Fc 'uses: actions/upload-artifact@' "$upload_workflow")"
  overwrite_count="$(grep -Fc 'overwrite: true' "$upload_workflow")"
  if [ "$upload_count" -eq 0 ] || [ "$upload_count" -ne "$overwrite_count" ]; then
    echo "every actions/upload-artifact step must set overwrite: true in ${upload_workflow#"$repo_root/"}" >&2
    exit 1
  fi
  checkout_count="$(grep -Fc 'uses: actions/checkout@' "$upload_workflow")"
  persist_false_count="$(grep -Fc 'persist-credentials: false' "$upload_workflow")"
  if [ "$checkout_count" -eq 0 ] || [ "$checkout_count" -ne "$persist_false_count" ]; then
    echo "every read-only checkout must set persist-credentials: false in ${upload_workflow#"$repo_root/"}" >&2
    exit 1
  fi
done

if ! grep -q "contents: read" "$ci_workflow" || grep -q "contents: write" "$ci_workflow"; then
  echo "ordinary CI must use read-only repository permissions"
  exit 1
fi

if ! grep -Fq 'workflow_call:' "$ci_workflow"; then
  echo "ordinary CI must be reusable as the release quality gate"
  exit 1
fi

release_ci_job="$(awk '/^  ci:/,/^  build:/' "$release_workflow")"
if ! grep -Fq 'needs: validate' <<<"$release_ci_job" ||
  ! grep -Fq 'uses: ./.github/workflows/ci.yml' <<<"$release_ci_job" ||
  ! grep -Fq 'source_sha: ${{ needs.validate.outputs.source_sha }}' <<<"$release_ci_job"; then
  echo "release workflow must validate tag first and reuse ordinary CI at the validated source_sha"
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
  ! grep -Fq 'test "$(wc -l < SHA256SUMS.txt' <<<"$publish_job" ||
  ! grep -Fq 'sha256sum -c SHA256SUMS.txt' <<<"$publish_job" ||
  ! grep -Fq 'all(.state == "uploaded")' <<<"$publish_job" ||
  ! grep -Fq 'ref: ${{ needs.validate.outputs.source_sha }}' <<<"$publish_job" ||
  ! grep -Fq 'persist-credentials: false' <<<"$publish_job" ||
  ! grep -Fq "steps.download_assets.outcome == 'failure'" <<<"$publish_job"; then
  echo "published releases must pin validated source_sha, retry transient artifact downloads, and verify checksums and all 14 uploaded draft assets before publishing"
  exit 1
fi

nfpm_config="$repo_root/build/linux/nfpm/nfpm.yaml"
for required_nfpm in \
  '/usr/bin/tunnelkit' \
  '/usr/bin/tunnelkit-helper' \
  'maintainer: "maybk <https://github.com/maybk/tunnelkit>"' \
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
if grep -Fq '/usr/local/bin/' "$nfpm_config" || grep -Fq '/usr/local/bin/' "$repo_root/build/linux/nfpm/scripts/postinstall.sh"; then
  echo "Linux deb/rpm packaging must install to FHS /usr/bin rather than /usr/local/bin" >&2
  exit 1
fi

for nfpm_script in \
  "$repo_root/build/linux/nfpm/scripts/postinstall.sh" \
  "$repo_root/build/linux/nfpm/scripts/preremove.sh"; do
  sh -n "$nfpm_script"
done
test ! -e "$repo_root/build/linux/nfpm/scripts/preinstall.sh"
test ! -e "$repo_root/build/linux/nfpm/scripts/postremove.sh"
bash -n "$repo_root/build/linux/appimage/build.sh"
if grep -Fq 'mv "${APP_NAME}*.AppImage"' "$repo_root/build/linux/appimage/build.sh"; then
  echo "build/linux/appimage/build.sh must not quote the glob wildcard when renaming AppImage" >&2
  exit 1
fi

if ! grep -Fq 'Exec=tunnelkit %u' "$repo_root/build/linux/desktop" ||
  ! grep -Fq 'StartupWMClass=com.tunnelkit.app' "$repo_root/build/linux/desktop" ||
  ! grep -Fq "STARTUP_WM_CLASS: 'com.tunnelkit.app'" "$repo_root/build/linux/Taskfile.yml" ||
  ! grep -Fq 'printf '\''StartupWMClass=%s\n'\'' "{{.STARTUP_WM_CLASS}}" >> "{{.OUTPUTFILE}}"' "$repo_root/build/linux/Taskfile.yml"; then
  echo "Linux .desktop configuration must set Exec=tunnelkit and StartupWMClass via STARTUP_WM_CLASS (com.tunnelkit.app)" >&2
  exit 1
fi

if ! grep -Fq 'for target in darwin/arm64 windows/amd64 windows/arm64; do' "$ci_workflow" ||
  ! grep -Fq 'GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go vet' "$ci_workflow"; then
  echo "ordinary CI must cross-vet macOS and Windows pure-Go packages" >&2
  exit 1
fi

if ! grep -Fq 'test-platform:' "$ci_workflow" ||
  ! grep -Fq "if: github.workflow == 'CI'" "$ci_workflow" ||
  ! grep -Fq 'os: [macos-latest, windows-latest]' "$ci_workflow" ||
  ! grep -Fq 'go test -race ./fortivpn/... ./internal/... ./cmd/...' "$ci_workflow" ||
  ! grep -Fq 'golangci/golangci-lint-action@' "$ci_workflow" ||
  ! grep -Fq 'version: v2.15.0' "$ci_workflow" ||
  ! grep -Fq 'go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...' "$ci_workflow"; then
  echo "ordinary CI must run golangci-lint v2, govulncheck, and native macOS/Windows Go race tests on direct CI runs while skipping duplicate runs in Release/Rolling callers" >&2
  exit 1
fi

if [ ! -f "$repo_root/.golangci.yml" ] ||
  ! grep -Fq 'version: "2"' "$repo_root/.golangci.yml" ||
  ! grep -Fq 'GOLANGCI_LINT_VERSION: v2.15.0' "$repo_root/Taskfile.yml" ||
  ! grep -Fq 'GOVULNCHECK_VERSION: v1.10.0' "$repo_root/Taskfile.yml"; then
  echo "local .golangci.yml v2 and Taskfile.yml lint fallback versions must stay aligned with ci.yml" >&2
  exit 1
fi

task_block() {
  local name="$1"
  awk -v target="  ${name}:" '
    { sub(/\015$/, "") }
    /^[^[:space:]#]/ {
      if (flag) exit
      in_tasks = ($1 == "tasks:")
      next
    }
    in_tasks && $0 == target { flag = 1; next }
    flag && /^  [^[:space:]#]/ { exit }
    flag { print }
  ' "$repo_root/Taskfile.yml"
}

for required_task in fmt lint test check check:frontend check:go release:prepare; do
  if [ -z "$(task_block "$required_task")" ]; then
    echo "Taskfile.yml is missing required local task: $required_task" >&2
    exit 1
  fi
done

check_task_block="$(task_block check)"
check_frontend_block="$(task_block check:frontend)"
check_go_block="$(task_block check:go)"
frontend_dist_ensure_block="$(task_block frontend:dist:ensure)"
lint_go_block="$(task_block lint:go)"
test_go_block="$(task_block test:go)"

if ! grep -Fq 'task: check:frontend' <<<"$check_task_block" ||
  ! grep -Fq 'task: check:go' <<<"$check_task_block" ||
  grep -Fq 'internal: true' <<<"$check_frontend_block" ||
  grep -Fq 'internal: true' <<<"$check_go_block" ||
  ! grep -Fq 'desc:' <<<"$check_frontend_block" ||
  ! grep -Fq 'desc:' <<<"$check_go_block" ||
  ! grep -Fq 'internal: true' <<<"$frontend_dist_ensure_block" ||
  ! grep -Fq 'task: frontend:dist:ensure' <<<"$lint_go_block" ||
  ! grep -Fq 'task: frontend:dist:ensure' <<<"$test_go_block"; then
  echo "Taskfile.yml must expose check:frontend and check:go, compose check from both, and ensure frontend/dist via an internal helper before standalone Go checks" >&2
  exit 1
fi

for versioned_workflow in "$release_workflow" "$rolling_workflow"; do
  if ! grep -Fq 'in_info=($1=="info:")' "$versioned_workflow"; then
    echo "workflow must parse info.version scoped to the info section of build/config.yml: ${versioned_workflow#"$repo_root/"}" >&2
    exit 1
  fi
done

for required_runner in \
  'runs-on: macos-latest' \
  'runs-on: windows-latest' \
  'runs-on: windows-11-arm' \
  'runs-on: ubuntu-24.04' \
  'runs-on: ubuntu-24.04-arm' \
  'xvfb-run' \
  'go test -race ./...' \
  'go test ./...' \
  '--version' \
  'rpm -qp --requires' \
  'rpm -qlp' \
  'TestSmokeLaunchInstalledUnixHelper'; do
  if ! grep -Fq -- "$required_runner" "$release_assets_workflow"; then
    echo "release-assets.yml is missing required parallel runner or verification step: $required_runner"
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

for test_script in "$repo_root"/scripts/test_*.sh; do
  test -f "$test_script"
  rel_script="${test_script#"$repo_root/"}"
  if ! grep -Fq "bash $rel_script" "$ci_workflow"; then
    echo "ci.yml is missing contract test: $rel_script" >&2
    exit 1
  fi
  if ! grep -Fq "bash $rel_script" "$repo_root/Taskfile.yml"; then
    echo "Taskfile.yml check task is missing contract test: $rel_script" >&2
    exit 1
  fi
done

test ! -e "$repo_root/scripts/render_package_manifests.sh"
test ! -e "$repo_root/scripts/test_package_manifests.sh"

notify_tap_job="$(awk '/^  notify-tap:/,0' "$release_workflow")"
for required_notify in \
  '- validate' \
  '- publish' \
  'permissions: {}' \
  'TAP_REPO: maybk/homebrew-tap' \
  'TAP_WORKFLOW: sync.yml' \
  'TAP_APP: tunnelkit' \
  'actions/create-github-app-token@' \
  'repositories: homebrew-tap' \
  'permission-actions: write' \
  'TAP_DISPATCH_APP_CLIENT_ID' \
  'TAP_DISPATCH_APP_PRIVATE_KEY' \
  'TAP_DISPATCH_TOKEN' \
  '/actions/workflows/${TAP_WORKFLOW}/dispatches' \
  'return_run_details: true'; do
  if ! grep -Fq -- "$required_notify" <<<"$notify_tap_job"; then
    echo "release.yml notify-tap job missing required contract: $required_notify" >&2
    exit 1
  fi
done
if grep -Eq 'permission-contents:|/repos/\$\{TAP_REPO\}/dispatches|repository_dispatch' <<<"$notify_tap_job"; then
  echo "release.yml notify-tap must use workflow_dispatch with Actions: write only (never Contents: write or repository_dispatch)" >&2
  exit 1
fi

if command -v python3 >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 && command -v curl >/dev/null 2>&1; then
  tap_tmp="$(mktemp -d)"
  tap_mock_pid=""
  cleanup_tap_test() {
    if [ -n "$tap_mock_pid" ]; then
      kill "$tap_mock_pid" >/dev/null 2>&1 || true
      wait "$tap_mock_pid" >/dev/null 2>&1 || true
    fi
    rm -rf "$tap_tmp"
  }
  trap cleanup_tap_test EXIT

  extract_step_script() {
    local step_name="$1"
    local out_file="$2"
    awk -v step="      - name: ${step_name}" '
      $0 == step { in_step=1; next }
      in_step && /^        run: \|$/ { in_run=1; next }
      in_run {
        if ($0 ~ /^  [a-zA-Z0-9_-]+:/ || $0 ~ /^      - name:/) { exit }
        sub(/^          /, "")
        print
      }
    ' "$release_workflow" > "$out_file"
    test -s "$out_file"
    bash -n "$out_file"
  }

  auth_mode_script="$tap_tmp/auth_mode.sh"
  dispatch_script="$tap_tmp/dispatch.sh"
  extract_step_script "Detect tap dispatch authentication mode" "$auth_mode_script"
  extract_step_script "Dispatch tap sync workflow" "$dispatch_script"

  # Auth mode detection matrix: App priority, partial-App warning + PAT fallback, CRLF-only PAT, none
  run_auth_mode() {
    local out_file="$tap_tmp/auth_out.txt"
    local log_file="$tap_tmp/auth_log.txt"
    : > "$out_file"
    APP_CLIENT_ID="$1" APP_PRIVATE_KEY="$2" PAT_TOKEN="$3" GITHUB_OUTPUT="$out_file" \
      bash "$auth_mode_script" > "$log_file"
    cat "$out_file"
  }
  [ "$(run_auth_mode "Iv23test" "pem-key" "pat-tok")" = "mode=app" ]
  [ "$(run_auth_mode "Iv23test" "" "pat-tok")" = "mode=pat" ]
  grep -Fq "::warning::Incomplete GitHub App configuration" "$tap_tmp/auth_log.txt"
  [ "$(run_auth_mode "" "pem-key" "")" = "mode=none" ]
  grep -Fq "::warning::Incomplete GitHub App configuration" "$tap_tmp/auth_log.txt"
  [ "$(run_auth_mode "" "" $'\r\n')" = "mode=none" ]
  [ "$(run_auth_mode "" "" "")" = "mode=none" ]

  tap_state="$tap_tmp/state.json"
  tap_port_file="$tap_tmp/port.txt"
  printf '{"mode":"ok_200","calls":0,"last_body":null,"last_auth":""}\n' > "$tap_state"

  python3 - "$tap_state" "$tap_port_file" <<'PY' &
import http.server, json, pathlib, sys

state_path = pathlib.Path(sys.argv[1])
port_path = pathlib.Path(sys.argv[2])

class DispatchHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        pass

    def do_POST(self):
        st = json.loads(state_path.read_text())
        n = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(n).decode()) if n > 0 else {}
        st["calls"] += 1
        st["last_body"] = body
        st["last_auth"] = self.headers.get("Authorization", "")
        st["last_path"] = self.path
        state_path.write_text(json.dumps(st))

        mode = st["mode"]
        if mode == "ok_200":
            resp = json.dumps({
                "workflow_run_id": 42,
                "html_url": "https://github.com/maybk/homebrew-tap/actions/runs/42",
            }).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(resp)))
            self.end_headers()
            self.wfile.write(resp)
            return
        if mode == "ok_204":
            self.send_response(204)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if mode == "retry_503_then_200":
            if st["calls"] == 1:
                resp = json.dumps({"message": "Service Unavailable"}).encode()
                self.send_response(503)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(resp)))
                self.end_headers()
                self.wfile.write(resp)
                return
            resp = json.dumps({
                "workflow_run_id": 99,
                "html_url": "https://github.com/maybk/homebrew-tap/actions/runs/99",
            }).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(resp)))
            self.end_headers()
            self.wfile.write(resp)
            return
        resp = json.dumps({"message": "Resource not accessible by personal access token"}).encode()
        self.send_response(403)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(resp)))
        self.end_headers()
        self.wfile.write(resp)

srv = http.server.HTTPServer(("127.0.0.1", 0), DispatchHandler)
port_path.write_text(str(srv.server_address[1]))
srv.serve_forever()
PY
  tap_mock_pid="$!"
  for _ in $(seq 1 30); do
    [ -s "$tap_port_file" ] && break
    sleep 0.05
  done
  if [ ! -s "$tap_port_file" ]; then
    echo "Mock tap dispatch server failed to start" >&2
    exit 1
  fi
  tap_api="http://127.0.0.1:$(cat "$tap_port_file")"
  tap_summary="$tap_tmp/summary.md"

  # 1. Unconfigured credentials skip cleanly with manual fallback notice and step summary
  : > "$tap_summary"
  TAP_REPO="maybk/homebrew-tap" TAP_WORKFLOW="sync.yml" TAP_REF="main" TAP_APP="tunnelkit" APP_VERSION="1.1.4" \
    AUTH_MODE="none" APP_TOKEN="" PAT_TOKEN="" GITHUB_API_URL="$tap_api" GITHUB_STEP_SUMMARY="$tap_summary" \
    bash "$dispatch_script" | grep -Fq "Skipping automatic maybk/homebrew-tap sync"
  grep -Fq "Status: skipped" "$tap_summary"

  # 2. App token dispatches workflow_dispatch (HTTP 200 with run URL and step summary)
  : > "$tap_summary"
  TAP_REPO="maybk/homebrew-tap" TAP_WORKFLOW="sync.yml" TAP_REF="main" TAP_APP="tunnelkit" APP_VERSION="1.1.4" \
    AUTH_MODE="app" APP_TOKEN="app-tok" PAT_TOKEN="pat-tok" GITHUB_API_URL="$tap_api" \
    GITHUB_STEP_SUMMARY="$tap_summary" TAP_DISPATCH_RETRY_SLEEP_SEC="0.05" \
    bash "$dispatch_script" | grep -Fq "https://github.com/maybk/homebrew-tap/actions/runs/42"
  grep -Fq "https://github.com/maybk/homebrew-tap/actions/runs/42" "$tap_summary"
  jq -e '
    .last_path == "/repos/maybk/homebrew-tap/actions/workflows/sync.yml/dispatches" and
    .last_auth == "Bearer app-tok" and
    .last_body.ref == "main" and
    .last_body.inputs.app == "tunnelkit" and
    .last_body.inputs.expected_version == "1.1.4" and
    .last_body.return_run_details == true
  ' "$tap_state" >/dev/null

  # 3. HTTP 204 backward-compatible response and trailing CRLF in PAT token succeed cleanly
  printf '{"mode":"ok_204","calls":0,"last_body":null,"last_auth":""}\n' > "$tap_state"
  TAP_REPO="maybk/homebrew-tap" TAP_WORKFLOW="sync.yml" TAP_REF="main" TAP_APP="tunnelkit" APP_VERSION="1.1.4" \
    AUTH_MODE="pat" APP_TOKEN="" PAT_TOKEN=$'pat-crlf\r\n' GITHUB_API_URL="$tap_api" TAP_DISPATCH_RETRY_SLEEP_SEC="0.05" \
    bash "$dispatch_script" | grep -Fq "HTTP 204"
  jq -e '.calls == 1 and .last_auth == "Bearer pat-crlf"' "$tap_state" >/dev/null

  # 4. Quote or control character in dispatch token is rejected before invoking curl
  if TAP_REPO="maybk/homebrew-tap" TAP_WORKFLOW="sync.yml" TAP_REF="main" TAP_APP="tunnelkit" APP_VERSION="1.1.4" \
      AUTH_MODE="pat" APP_TOKEN="" PAT_TOKEN='bad"token' GITHUB_API_URL="$tap_api" \
      bash "$dispatch_script" >/dev/null 2>&1; then
    echo "Expected notify-tap dispatch script to reject quote characters in token" >&2
    exit 1
  fi
  jq -e '.calls == 1' "$tap_state" >/dev/null

  # 5. Transient 503 retries and succeeds
  printf '{"mode":"retry_503_then_200","calls":0,"last_body":null,"last_auth":""}\n' > "$tap_state"
  TAP_REPO="maybk/homebrew-tap" TAP_WORKFLOW="sync.yml" TAP_REF="main" TAP_APP="tunnelkit" APP_VERSION="1.1.4" \
    AUTH_MODE="pat" APP_TOKEN="" PAT_TOKEN="pat-tok" GITHUB_API_URL="$tap_api" TAP_DISPATCH_RETRY_SLEEP_SEC="0.05" \
    bash "$dispatch_script" | grep -Fq "https://github.com/maybk/homebrew-tap/actions/runs/99"
  jq -e '.calls == 2 and .last_auth == "Bearer pat-tok"' "$tap_state" >/dev/null

  # 6. 403 permission error fails fast without silent swallow
  printf '{"mode":"err_403","calls":0,"last_body":null,"last_auth":""}\n' > "$tap_state"
  if TAP_REPO="maybk/homebrew-tap" TAP_WORKFLOW="sync.yml" TAP_REF="main" TAP_APP="tunnelkit" APP_VERSION="1.1.4" \
      AUTH_MODE="pat" APP_TOKEN="" PAT_TOKEN="pat-tok" GITHUB_API_URL="$tap_api" TAP_DISPATCH_RETRY_SLEEP_SEC="0.05" \
      bash "$dispatch_script" >/dev/null 2>&1; then
    echo "Expected notify-tap dispatch script to fail visibly on HTTP 403" >&2
    exit 1
  fi
  jq -e '.calls == 1' "$tap_state" >/dev/null

  # 7. Non-HTTP curl transport failure reports HTTP 000 cleanly (never empty status)
  kill "$tap_mock_pid" >/dev/null 2>&1 || true
  wait "$tap_mock_pid" >/dev/null 2>&1 || true
  tap_mock_pid=""
  set +e
  conn_err_out="$(
    TAP_REPO="maybk/homebrew-tap" TAP_WORKFLOW="sync.yml" TAP_REF="main" TAP_APP="tunnelkit" APP_VERSION="1.1.4" \
      AUTH_MODE="pat" APP_TOKEN="" PAT_TOKEN="pat-tok" GITHUB_API_URL="$tap_api" TAP_DISPATCH_RETRY_SLEEP_SEC="0.01" \
      bash "$dispatch_script" 2>&1
  )"
  conn_err_rc="$?"
  set -e
  [ "$conn_err_rc" -ne 0 ]
  grep -Fq "(HTTP 000)" <<<"$conn_err_out"
fi
