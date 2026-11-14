#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
extractor="$repo_root/scripts/extract_release_notes.sh"
renderer="$repo_root/scripts/render_release_notes.sh"
ci_workflow="$repo_root/.github/workflows/ci.yml"
release_workflow="$repo_root/.github/workflows/release.yml"

if ! grep -Eq '^## \[[0-9]+\.[0-9]+\.[0-9]+\] - 未发布$' "$repo_root/CHANGELOG.md"; then
  echo "CHANGELOG.md must keep a pending semantic version for incoming changes"
  exit 1
fi

if [ ! -f "$extractor" ] || [ ! -f "$renderer" ]; then
  echo "missing release notes extractor or renderer"
  exit 1
fi
bash -n "$extractor"
bash -n "$renderer"

temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

fixture="$temp_dir/CHANGELOG.md"
actual="$temp_dir/actual.md"
expected="$temp_dir/expected.md"

cat >"$fixture" <<'EOF'
# 更新日志

## [0.2.0] - 未发布

### 新增

- 尚未发布的变化。

## [0.1.0] - 2026-10-21

### 新增

- 新增正式功能。

### 修复

- 修复正式问题。

## [0.0.9] - 2026-09-01

### 修复

- 不应包含的旧版本内容。
EOF

cat >"$expected" <<'EOF'
### 新增

- 新增正式功能。

### 修复

- 修复正式问题。
EOF

bash "$extractor" "0.1.0" "$fixture" >"$actual"
diff -u "$expected" "$actual"

if bash "$extractor" "3.0.0" "$fixture" >/dev/null 2>&1; then
  echo "missing release version must fail"
  exit 1
fi

cat >"$fixture" <<'EOF'
# 更新日志

## [0.2.0] - 2026-10-21

## [0.1.0] - 2026-09-01

- 旧版本内容。
EOF

set +e
bash "$extractor" "0.2.0" "$fixture" >/dev/null 2>&1
empty_status="$?"
set -e
if [ "$empty_status" -ne 2 ]; then
  echo "empty release notes must return status 2; got $empty_status"
  exit 1
fi

if bash "$extractor" "not-a-version" "$fixture" >/dev/null 2>&1; then
  echo "invalid release version must fail"
  exit 1
fi

cat >"$fixture" <<'EOF'
# 更新日志

## [0.1.0] - 2026-10-21

- 第一段内容。

## [0.1.0] - 2026-10-22

- 重复版本内容。
EOF

if bash "$extractor" "0.1.0" "$fixture" >/dev/null 2>&1; then
  echo "duplicate release versions must fail"
  exit 1
fi

bash "$extractor" "0.1.0" "$repo_root/CHANGELOG.md" >"$actual"
if ! grep -Fq '支持 Fortinet SSL-VPN 连接' "$actual"; then
  echo "repository release notes for 0.1.0 must contain the expected section"
  exit 1
fi

rendered_formal="$temp_dir/rendered-formal.md"
cat >"$fixture" <<'EOF'
# 更新日志

## [0.2.0] - 未发布

### 变更

- 预览版本调整。

## [0.1.0] - 2026-10-21

### 新增

- 新增正式功能。

### 修复

- 修复正式问题。
EOF

bash "$renderer" \
  --version "0.1.0" \
  --repo "maybk/tunnelkit" \
  --server-url "https://github.com///" \
  --changelog "$fixture" >"$rendered_formal"

for required_formal in \
  '## 更新内容' \
  '- 新增正式功能。' \
  '- 修复正式问题。' \
  '## 下载安装' \
  '| 系统 | 安装包下载 |' \
  '![Windows Setup x64](' \
  '![Windows Setup ARM64](' \
  '![macOS DMG Apple Silicon](' \
  '![macOS DMG Intel](' \
  '![Linux DEB x64](' \
  '![Linux DEB ARM64](' \
  '![Linux RPM x64](' \
  '![Linux RPM ARM64]('; do
  if ! grep -Fq -- "$required_formal" "$rendered_formal"; then
    echo "rendered formal release notes missing expected fragment: $required_formal"
    exit 1
  fi
done

if grep -Fq '> [!WARNING]' "$rendered_formal" || grep -Fq '## 构建信息' "$rendered_formal"; then
  echo "formal release notes must not include Rolling prerelease warning or metadata"
  exit 1
fi

for curated_suffix in \
  'windows-amd64-setup.exe' \
  'windows-arm64-setup.exe' \
  'macos-arm64.dmg' \
  'macos-amd64.dmg' \
  'linux-amd64.deb' \
  'linux-arm64.deb' \
  'linux-amd64.rpm' \
  'linux-arm64.rpm'; do
  expected_url="https://github.com/maybk/tunnelkit/releases/download/v0.1.0/tunnelkit-0.1.0-${curated_suffix}"
  if ! grep -Fq "$expected_url" "$rendered_formal"; then
    echo "rendered formal release notes missing curated download URL: $expected_url"
    exit 1
  fi
  if ! grep -Fq "\"tunnelkit-\$APP_VERSION-${curated_suffix}\"" "$release_workflow"; then
    echo "curated download suffix does not match verified assets in release.yml: $curated_suffix"
    exit 1
  fi
done

for excluded_suffix in \
  'macos-universal.dmg' \
  'windows-amd64.zip' \
  'windows-arm64.zip' \
  'linux-amd64.tar.gz' \
  'linux-arm64.tar.gz' \
  'SHA256SUMS.txt'; do
  if grep -Fq "$excluded_suffix" "$rendered_formal"; then
    echo "curated download table must omit secondary/portable asset: $excluded_suffix"
    exit 1
  fi
done

if grep -Eq 'logo=data:image/svg[^)]*\+' "$rendered_formal"; then
  echo "inline SVG badge logo query parameter must percent-encode '+' as '%2B'"
  exit 1
fi

win_row_line="$(grep -nF '| **Windows** |' "$rendered_formal" | head -n 1 | cut -d: -f1)"
mac_row_line="$(grep -nF '| **macOS** |' "$rendered_formal" | head -n 1 | cut -d: -f1)"
linux_row_line="$(grep -nF '| **Linux** |' "$rendered_formal" | head -n 1 | cut -d: -f1)"
if [ -z "$win_row_line" ] || [ -z "$mac_row_line" ] || [ -z "$linux_row_line" ] ||
  [ "$win_row_line" -ge "$mac_row_line" ] || [ "$mac_row_line" -ge "$linux_row_line" ]; then
  echo "download table must list OS rows in Windows -> macOS -> Linux order"
  exit 1
fi

rendered_rolling="$temp_dir/rendered-rolling.md"
sample_sha="0123456789abcdef0123456789abcdef01234567"
bash "$renderer" \
  --rolling \
  --version "0.2.0" \
  --app-version "0.2.0-pre" \
  --source-sha "$sample_sha" \
  --built-at "2026-11-15 00:30:00 UTC" \
  --repo "maybk/tunnelkit" \
  --server-url "https://github.com" \
  --changelog "$fixture" >"$rendered_rolling"

for required_rolling in \
  '> [!WARNING]' \
  '## 更新内容' \
  '- 预览版本调整。' \
  '## 下载安装' \
  '## 构建信息' \
  '- 预览版本：`0.2.0`' \
  '- 应用版本：`0.2.0-pre`' \
  '- 提交：[`0123456789ab`](https://github.com/maybk/tunnelkit/commit/0123456789abcdef0123456789abcdef01234567)' \
  '- 构建时间：`2026-11-15 00:30:00 UTC`'; do
  if ! grep -Fq -- "$required_rolling" "$rendered_rolling"; then
    echo "rendered Rolling release notes missing expected fragment: $required_rolling"
    exit 1
  fi
done

for curated_suffix in \
  'windows-amd64-setup.exe' \
  'windows-arm64-setup.exe' \
  'macos-arm64.dmg' \
  'macos-amd64.dmg' \
  'linux-amd64.deb' \
  'linux-arm64.deb' \
  'linux-amd64.rpm' \
  'linux-arm64.rpm'; do
  expected_rolling_url="https://github.com/maybk/tunnelkit/releases/download/v0.2.0-pre/tunnelkit-0.2.0-rolling-0123456789ab-${curated_suffix}"
  if ! grep -Fq "$expected_rolling_url" "$rendered_rolling"; then
    echo "rendered Rolling release notes missing curated download URL: $expected_rolling_url"
    exit 1
  fi
done

cat >"$fixture" <<'EOF'
# 更新日志

## [0.2.0] - 未发布

## [0.1.0] - 2026-10-21

- 已有内容。
EOF

set +e
bash "$renderer" --version "0.2.0" --changelog "$fixture" >/dev/null 2>&1
render_empty_status="$?"
set -e
if [ "$render_empty_status" -ne 2 ]; then
  echo "renderer must preserve exit status 2 on empty changelog section; got $render_empty_status"
  exit 1
fi

if bash "$renderer" --rolling --version "0.1.0" --app-version "0.1.0-pre" --source-sha "shortsha" --changelog "$fixture" >/dev/null 2>&1; then
  echo "renderer must reject non-40-hex Rolling source SHA"
  exit 1
fi

if bash "$renderer" --rolling --version "0.1.0" --app-version "0.1.0" --source-sha "$sample_sha" --changelog "$fixture" >/dev/null 2>&1; then
  echo "renderer must reject Rolling app-version without prerelease suffix"
  exit 1
fi

if bash "$renderer" --version "0.1.0" --source-sha "$sample_sha" --changelog "$fixture" >/dev/null 2>&1; then
  echo "renderer must reject Rolling-only flags in formal mode"
  exit 1
fi

if ! grep -Fq 'bash scripts/render_release_notes.sh \' "$release_workflow" ||
  ! grep -Fq -- '--version "$APP_VERSION"' "$release_workflow" ||
  ! grep -Fq -- '--notes-file "$release_notes_file"' "$release_workflow" ||
  grep -Fq -- '--generate-notes' "$release_workflow"; then
  echo "release workflow must render release notes with curated download badges from CHANGELOG.md"
  exit 1
fi

validate_job="$(awk '/^  validate:/,/^  ci:/' "$release_workflow")"
if ! grep -Fq 'name: Validate release notes' <<<"$validate_job" ||
  ! grep -Fq 'bash scripts/extract_release_notes.sh "$version" CHANGELOG.md' <<<"$validate_job" ||
  ! grep -Fq 'steps.version.outputs.version' <<<"$validate_job"; then
  echo "release workflow must validate release notes before platform builds"
  exit 1
fi

if ! grep -Fq 'bash scripts/test_extract_release_notes.sh' "$ci_workflow"; then
  echo "ordinary CI must run the release notes contract test"
  exit 1
fi
