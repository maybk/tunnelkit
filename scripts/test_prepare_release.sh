#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
prepare_script="$repo_root/scripts/prepare_release.sh"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p "$tmp_dir/build"
cat > "$tmp_dir/build/config.yml" <<'EOF'
info:
  companyName: "maybk"
  productName: "TunnelKit"
  version: "0.2.1"
EOF

cat > "$tmp_dir/CHANGELOG.md" <<'EOF'
# 更新日志

## [0.2.2] - 未发布

### 新增

- 支持应用内一键更新并自动重启。

## [0.2.1] - 2026-10-23

### 新增

- 初始发布说明。
EOF

bash "$prepare_script" --root "$tmp_dir" --date "2026-10-26" --skip-wails >/dev/null

grep -Fq 'version: "0.2.2"' "$tmp_dir/build/config.yml"
grep -Fq '## [0.2.3] - 未发布' "$tmp_dir/CHANGELOG.md"
grep -Fq '## [0.2.2] - 2026-10-26' "$tmp_dir/CHANGELOG.md"

pending="$(bash "$repo_root/scripts/extract_pending_version.sh" "$tmp_dir/CHANGELOG.md")"
[ "$pending" = "0.2.3" ]
notes="$(bash "$repo_root/scripts/extract_release_notes.sh" "0.2.2" "$tmp_dir/CHANGELOG.md")"
grep -Fq '支持应用内一键更新并自动重启' <<<"$notes"
