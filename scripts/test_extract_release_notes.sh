#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
extractor="$repo_root/scripts/extract_release_notes.sh"
ci_workflow="$repo_root/.github/workflows/ci.yml"
release_workflow="$repo_root/.github/workflows/release.yml"

if ! grep -Eq '^## \[[0-9]+\.[0-9]+\.[0-9]+\] - 未发布$' "$repo_root/CHANGELOG.md"; then
  echo "CHANGELOG.md must keep a pending semantic version for incoming changes"
  exit 1
fi

if [ ! -f "$extractor" ]; then
  echo "missing release notes extractor"
  exit 1
fi

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

if ! grep -Fq 'bash scripts/extract_release_notes.sh "$APP_VERSION" CHANGELOG.md' "$release_workflow" ||
  ! grep -Fq -- '--notes-file "$release_notes_file"' "$release_workflow" ||
  grep -Fq -- '--generate-notes' "$release_workflow"; then
  echo "release workflow must publish the matching CHANGELOG section"
  exit 1
fi

validate_job="$(awk '/^  validate:/,/^  ci:/' "$release_workflow")"
if ! grep -Fq 'name: Validate release notes' <<<"$validate_job" ||
  ! grep -Fq 'steps.version.outputs.version' <<<"$validate_job"; then
  echo "release workflow must validate release notes before platform builds"
  exit 1
fi

if ! grep -Fq 'bash scripts/test_extract_release_notes.sh' "$ci_workflow"; then
  echo "ordinary CI must run the release notes contract test"
  exit 1
fi
