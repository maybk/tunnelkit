#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ci_workflow="$repo_root/.github/workflows/ci.yml"
release_workflow="$repo_root/.github/workflows/release.yml"
build_workflow="$repo_root/.github/workflows/release-assets.yml"
rolling_workflow="$repo_root/.github/workflows/rolling.yml"
pending_version_extractor="$repo_root/scripts/extract_pending_version.sh"

for workflow in "$build_workflow" "$rolling_workflow"; do
  if [ ! -f "$workflow" ]; then
    echo "missing workflow: ${workflow#"$repo_root/"}"
    exit 1
  fi
done

for required in \
  'schedule:' \
  "- cron: '17 18 * * *'" \
  'workflow_dispatch:' \
  'force:' \
  "github.event_name == 'schedule'" \
  "github.event_name == 'workflow_dispatch'" \
  'actions: read' \
  'group: rolling' \
  'cancel-in-progress: true' \
  'refs/remotes/origin/main' \
  'git checkout --detach --quiet "$source_sha"' \
  'git merge-base --is-ancestor' \
  'v[0-9]+\.[0-9]+\.[0-9]+' \
  '正式发布提交，跳过 Rolling' \
  'published_sha="$(git rev-parse --verify --quiet "refs/tags/${rolling_tag}^{commit}" || true)"' \
  'releases_json="$(gh api "repos/$GH_REPO/releases?per_page=100")"' \
  'all(.state == "uploaded")' \
  '当前 main 提交已发布完整 Rolling' \
  'actions/workflows/ci.yml/runs?head_sha=$source_sha' \
  'needs_ci: ${{ steps.source.outputs.needs_ci }}' \
  "needs.prepare.outputs.needs_ci == 'true'" \
  'uses: ./.github/workflows/ci.yml' \
  'uses: ./.github/workflows/release-assets.yml' \
  'retention_days: 1' \
  'source_sha: ${{ needs.prepare.outputs.source_sha }}' \
  'version: ${{ needs.prepare.outputs.build_version }}' \
  'app_version: ${{ steps.source.outputs.app_version }}' \
  'APP_VERSION: ${{ needs.prepare.outputs.app_version }}' \
  'echo "app_version=$app_version" >> "$GITHUB_OUTPUT"' \
  'ROLLING_VERSION: ${{ needs.prepare.outputs.rolling_version }}' \
  'bash scripts/extract_pending_version.sh CHANGELOG.md' \
  'bash scripts/extract_release_notes.sh "$rolling_version" CHANGELOG.md' \
  'if [ "$notes_status" -eq 2 ]; then' \
  '待发布版本暂无内容，跳过 Rolling' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-macos-universal.dmg' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-macos-arm64.dmg' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-macos-amd64.dmg' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-windows-amd64-setup.exe' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-windows-amd64.zip' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-windows-arm64-setup.exe' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-windows-arm64.zip' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-amd64.tar.gz' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-amd64.deb' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-amd64.rpm' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-arm64.tar.gz' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-arm64.deb' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-arm64.rpm' \
  'SHA256SUMS.txt' \
  '--draft' \
  '--prerelease' \
  'app_version="${rolling_version}-pre"' \
  'build_version="${rolling_version}-pre"' \
  'rolling_tag="v${ROLLING_VERSION}-pre"' \
  'git tag -a --force "$rolling_tag" -m "TunnelKit $ROLLING_VERSION Rolling ($SHORT_SHA)" "$SOURCE_SHA"' \
  '--title "TunnelKit $ROLLING_VERSION Rolling"' \
  'if [ "$asset_count" -ne 14 ]; then'; do
  if ! grep -Fq -- "$required" "$rolling_workflow"; then
    echo "Rolling workflow is missing required contract: $required"
    exit 1
  fi
done

if [ ! -f "$pending_version_extractor" ]; then
  echo "Rolling workflow must use a tested pending changelog version extractor"
  exit 1
fi

if grep -Fq 'workflow_run:' "$rolling_workflow" || grep -Fq 'github.event.workflow_run' "$rolling_workflow"; then
  echo "Rolling workflow must run on daily schedule and manual dispatch rather than every push workflow_run"
  exit 1
fi

if grep -Fq 'GITHUB_SHA' "$rolling_workflow"; then
  echo "Rolling workflow must not use GITHUB_SHA directly as the source commit"
  exit 1
fi

if grep -Fq 'app_go_version' "$rolling_workflow"; then
  echo "Rolling workflow must read application version solely from build/config.yml"
  exit 1
fi

if ! grep -Fq "github.event_name == 'workflow_dispatch'" "$rolling_workflow" ||
  ! grep -Fq "github.ref == 'refs/heads/main'" "$rolling_workflow"; then
  echo "Rolling runs must enforce CI gating and only accept the main branch"
  exit 1
fi

if ! grep -Fq 'source_sha:' "$ci_workflow" ||
  ! grep -Fq 'ref: ${{ inputs.source_sha }}' "$ci_workflow"; then
  echo "reusable CI workflow must accept an optional source_sha input for exact Rolling commit verification"
  exit 1
fi

for required in \
  'workflow_call:' \
  'source_sha:' \
  'version:' \
  'retention_days:' \
  'ref: ${{ inputs.source_sha }}' \
  'retention-days: ${{ inputs.retention_days }}' \
  'wails3 task darwin:package:arm64' \
  'wails3 task darwin:package:amd64' \
  'wails3 task darwin:package:universal' \
  'wails3 task windows:build:amd64' \
  'wails3 task windows:build:arm64' \
  'wails3 task windows:generate:nsis:installer ARCH=amd64' \
  'wails3 task windows:generate:nsis:installer ARCH=arm64' \
  'wails3 task linux:build' \
  'wails3 task linux:create:deb' \
  'wails3 task linux:generate:rpm ARCH=amd64' \
  'wails3 task linux:generate:rpm ARCH=arm64' \
  'name: release-macos-arm64' \
  'name: release-macos-amd64' \
  'name: release-macos-universal' \
  'name: release-windows-amd64' \
  'name: release-windows-arm64' \
  'name: release-linux-amd64' \
  'name: release-linux-arm64'; do
  if ! grep -Fq -- "$required" "$build_workflow"; then
    echo "reusable release build is missing required contract: $required"
    exit 1
  fi
done

for caller in "$release_workflow" "$rolling_workflow"; do
  if ! grep -Fq 'uses: ./.github/workflows/release-assets.yml' "$caller"; then
    echo "formal and Rolling releases must share the reusable build workflow: ${caller#"$repo_root/"}"
    exit 1
  fi
done

if ! grep -Fq 'source_sha: ${{ needs.validate.outputs.source_sha }}' "$release_workflow" ||
  ! grep -Fq 'retention_days: 7' "$release_workflow"; then
  echo "formal releases must build the validated tag commit and retain artifacts for seven days"
  exit 1
fi

for required in \
  'pending_version="$(bash scripts/extract_pending_version.sh CHANGELOG.md)"' \
  '^## \[$version\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$'; do
  if ! grep -Fq -- "$required" "$release_workflow"; then
    echo "formal releases must validate the pending section and dated release section: $required"
    exit 1
  fi
done

cleanup_job="$(awk '/^  cleanup-rolling:/,0' "$release_workflow")"
for required in \
  'needs: publish' \
  'contents: write' \
  'git merge-base --is-ancestor "$rolling_commit" "$release_commit"' \
  'gh release delete "$tag_name" --yes' \
  'refs/tags/v*-pre' \
  'Rolling 不存在，无需清理' \
  'Rolling 指向正式版本之后的提交，保留'; do
  if ! grep -Fq -- "$required" <<<"$cleanup_job"; then
    echo "formal release cleanup is missing required contract: $required"
    exit 1
  fi
done

rolling_publish_job="$(awk '/^  publish:/,0' "$rolling_workflow")"
rolling_notes_step="$(awk \
  '/^      - name: Prepare Rolling notes$/,/^      - name: Create draft, upload, verify, and publish Rolling$/' \
  "$rolling_workflow")"
for required in \
  'pending_notes_file="$RUNNER_TEMP/rolling-changelog.md"' \
  'bash scripts/extract_release_notes.sh "$ROLLING_VERSION" CHANGELOG.md > "$pending_notes_file"' \
  'cat "$pending_notes_file"' \
  '## 更新内容' \
  '## 构建信息' \
  '预览版本' \
  '应用版本' \
  '提交：' \
  '构建时间'; do
  if ! grep -Fq -- "$required" <<<"$rolling_notes_step"; then
    echo "Rolling notes must include pending CHANGELOG content and concise build metadata: $required"
    exit 1
  fi
done

for required in \
  'always()' \
  '!cancelled()' \
  "needs.prepare.result == 'success'" \
  "needs.prepare.outputs.should_publish == 'true'" \
  "needs.build.result == 'success'"; do
  if ! grep -Fq -- "$required" <<<"$rolling_publish_job"; then
    echo "Rolling publish must override skipped dependency propagation safely: $required"
    exit 1
  fi
done

for mutation_job in "$rolling_publish_job" "$cleanup_job"; do
  if ! grep -Fq 'group: rolling-release-mutation' <<<"$mutation_job" ||
    ! grep -Fq 'cancel-in-progress: false' <<<"$mutation_job"; then
    echo "Rolling publish and formal cleanup must share a non-canceling mutation concurrency group"
    exit 1
  fi
done

if ! grep -Fq 'bash scripts/test_rolling_release_config.sh' "$ci_workflow"; then
  echo "ordinary CI must run the Rolling release workflow contract test"
  exit 1
fi

if ! grep -Fq 'bash scripts/test_extract_pending_version.sh' "$ci_workflow"; then
  echo "ordinary CI must test pending changelog version extraction"
  exit 1
fi

# Behavioral state-machine tests against the extracted Rolling prepare script
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

prepare_script="$tmp_dir/prepare.sh"
awk '
  /^      - name: Validate source commit$/ { in_step=1; next }
  in_step && /^        run: \|$/ { in_run=1; next }
  in_run {
    if ($0 ~ /^  [a-zA-Z0-9_-]+:/ || $0 ~ /^      - name:/) { exit }
    sub(/^          /, "")
    print
  }
' "$rolling_workflow" > "$prepare_script"
test -s "$prepare_script"
bash -n "$prepare_script"

mock_bin="$tmp_dir/bin"
mkdir -p "$mock_bin"
cat > "$mock_bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "api" ]; then
  endpoint="${2:-}"
  case "$endpoint" in
    repos/*/releases\?per_page=100)
      if [ "${MOCK_RELEASES_API_FAIL:-false}" = "true" ]; then
        echo "HTTP 502 Bad Gateway" >&2
        exit 1
      fi
      cat "$MOCK_RELEASES_JSON_FILE"
      exit 0
      ;;
    repos/*/actions/workflows/ci.yml/runs*)
      if [ "${MOCK_CI_API_FAIL:-false}" = "true" ]; then
        exit 1
      fi
      cat "$MOCK_CI_JSON_FILE"
      exit 0
      ;;
  esac
fi
echo "unexpected gh command: $*" >&2
exit 1
EOF
chmod +x "$mock_bin/gh"

releases_json_file="$tmp_dir/releases.json"
ci_json_file="$tmp_dir/ci_runs.json"
printf '[]\n' > "$releases_json_file"
printf '{"workflow_runs":[]}\n' > "$ci_json_file"

write_complete_release_json() {
  local ver="$1"
  local ssha="$2"
  local draft="${3:-false}"
  local asset_state="${4:-uploaded}"
  local omit_last="${5:-false}"
  jq -n \
    --arg tag "v${ver}-pre" \
    --arg ver "$ver" \
    --arg ssha "$ssha" \
    --argjson draft "$draft" \
    --arg state "$asset_state" \
    --argjson omit "$omit_last" \
    '
      (
        ["SHA256SUMS.txt"] + (
          [
            "macos-universal.dmg",
            "macos-arm64.dmg",
            "macos-amd64.dmg",
            "windows-amd64-setup.exe",
            "windows-amd64.zip",
            "windows-arm64-setup.exe",
            "windows-arm64.zip",
            "linux-amd64.tar.gz",
            "linux-amd64.deb",
            "linux-amd64.rpm",
            "linux-arm64.tar.gz",
            "linux-arm64.deb",
            "linux-arm64.rpm"
          ] | map("tunnelkit-\($ver)-rolling-\($ssha)-" + .)
        )
      ) as $names |
      [
        {
          tag_name: $tag,
          draft: $draft,
          prerelease: true,
          assets: (
            (if $omit then $names[0:13] else $names end)
            | map({ name: ., state: $state })
          )
        }
      ]
    ' > "$releases_json_file"
}

origin_repo="$tmp_dir/origin.git"
work_repo="$tmp_dir/work"
git init --bare --quiet "$origin_repo"
git init --quiet "$work_repo"
git -C "$work_repo" checkout -q -b main
git -C "$work_repo" config user.name "Test Bot"
git -C "$work_repo" config user.email "test@example.com"
git -C "$work_repo" remote add origin "$origin_repo"

mkdir -p "$work_repo/build" "$work_repo/scripts"
cp "$repo_root/scripts/extract_pending_version.sh" "$work_repo/scripts/"
cp "$repo_root/scripts/extract_release_notes.sh" "$work_repo/scripts/"
cat > "$work_repo/build/config.yml" <<'EOF'
info:
  version: "1.0.1"
EOF
cat > "$work_repo/CHANGELOG.md" <<'EOF'
# 更新日志

## [1.0.2] - 未发布

### 修复

- 修复测试问题。

## [1.0.1] - 2026-11-11

### 修复

- 初始正式版。
EOF
git -C "$work_repo" add .
git -C "$work_repo" commit -q -m "feat: pending change"
git -C "$work_repo" push -q origin main

run_prepare() {
  local out_file="$tmp_dir/github_output.txt"
  : > "$out_file"
  (
    cd "$work_repo"
    PATH="$mock_bin:$PATH" \
      RUNNER_TEMP="$tmp_dir" \
      GITHUB_OUTPUT="$out_file" \
      GH_TOKEN="dummy" \
      GH_REPO="maybk/tunnelkit" \
      MOCK_RELEASES_JSON_FILE="$releases_json_file" \
      MOCK_CI_JSON_FILE="$ci_json_file" \
      EVENT_NAME="${1:-schedule}" \
      EVENT_REF="${2:-refs/heads/main}" \
      FORCE_PUBLISH="${3:-false}" \
      bash "$prepare_script" >/dev/null
  ) && cat "$out_file"
}

# 1. No -pre tag online, pending changelog non-empty, no prior CI success -> publish + run CI
printf '[]\n' > "$releases_json_file"
printf '{"workflow_runs":[]}\n' > "$ci_json_file"
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=true" <<<"$out"
grep -Fq "needs_ci=true" <<<"$out"
grep -Fq "rolling_version=1.0.2" <<<"$out"

# 2. Matching -pre tag online + complete public prerelease (14 uploaded assets with matching short_sha) -> skip
head_sha="$(git -C "$work_repo" rev-parse HEAD)"
head_short_sha="${head_sha:0:12}"
git -C "$work_repo" tag -a -f "v1.0.2-pre" -m "Rolling" "$head_sha"
git -C "$work_repo" push -q --force origin "refs/tags/v1.0.2-pre"
write_complete_release_json "1.0.2" "$head_short_sha" false uploaded false
printf '{"workflow_runs":[{"event":"push","head_branch":"main","conclusion":"success"}]}\n' > "$ci_json_file"
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=false" <<<"$out"
grep -Fq "needs_ci=false" <<<"$out"

# 3. Same published commit, but manual dispatch with force=true and passing latest push CI -> rebuild without rerunning CI
out="$(run_prepare workflow_dispatch refs/heads/main true)"
grep -Fq "should_publish=true" <<<"$out"
grep -Fq "needs_ci=false" <<<"$out"

# 3b. Latest push CI rerun failed even if an older run succeeded -> must require CI gate
printf '{"workflow_runs":[{"event":"push","head_branch":"main","conclusion":"failure"},{"event":"push","head_branch":"main","conclusion":"success"}]}\n' > "$ci_json_file"
out="$(run_prepare workflow_dispatch refs/heads/main true)"
grep -Fq "should_publish=true" <<<"$out"
grep -Fq "needs_ci=true" <<<"$out"
printf '{"workflow_runs":[{"event":"push","head_branch":"main","conclusion":"success"}]}\n' > "$ci_json_file"

# 4a. Orphan tag (no matching GitHub Release) -> self-heal and rebuild
printf '[]\n' > "$releases_json_file"
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=true" <<<"$out"

# 4b. Incomplete Rolling release (draft=true, <14 assets, starter state, or mismatched asset short_sha) -> self-heal
write_complete_release_json "1.0.2" "$head_short_sha" true uploaded false
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=true" <<<"$out"

write_complete_release_json "1.0.2" "$head_short_sha" false uploaded true
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=true" <<<"$out"

write_complete_release_json "1.0.2" "$head_short_sha" false starter false
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=true" <<<"$out"

write_complete_release_json "1.0.2" "deadbeefcafe" false uploaded false
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=true" <<<"$out"

# 4c. Transient GitHub Releases API failure (5xx) -> fast-fail rather than falsely rebuilding
if MOCK_RELEASES_API_FAIL=true run_prepare schedule refs/heads/main false >/dev/null 2>&1; then
  echo "prepare must fail fast on GitHub Releases API errors rather than triggering a spurious rebuild"
  exit 1
fi

# 5. Formal release commit tagged vX.Y.Z -> skip Rolling even without -pre tag
git -C "$work_repo" push -q origin ":refs/tags/v1.0.2-pre"
git -C "$work_repo" tag -d "v1.0.2-pre" >/dev/null
git -C "$work_repo" tag -a "v1.0.2" -m "TunnelKit v1.0.2" "$head_sha"
git -C "$work_repo" push -q origin "refs/tags/v1.0.2"
printf '[]\n' > "$releases_json_file"
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=false" <<<"$out"

# 6. Empty pending changelog section after release (on a new untagged commit) -> skip Rolling
git -C "$work_repo" checkout -q main
cat > "$work_repo/CHANGELOG.md" <<'EOF'
# 更新日志

## [1.0.3] - 未发布

## [1.0.2] - 2026-11-13

### 修复

- 修复测试问题。
EOF
git -C "$work_repo" add CHANGELOG.md
git -C "$work_repo" commit -q -m "chore: empty pending changelog"
git -C "$work_repo" push -q origin main
empty_head_sha="$(git -C "$work_repo" rev-parse refs/heads/main)"
[ "$empty_head_sha" != "$head_sha" ]
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=false" <<<"$out"

# 7. New commit with non-empty pending changelog advances past older -pre tag -> publish new Rolling
git -C "$work_repo" checkout -q main
git -C "$work_repo" tag -a -f "v1.0.3-pre" -m "Old Rolling" "$empty_head_sha"
git -C "$work_repo" push -q --force origin "refs/tags/v1.0.3-pre"
write_complete_release_json "1.0.3" "${empty_head_sha:0:12}" false uploaded false
cat > "$work_repo/CHANGELOG.md" <<'EOF'
# 更新日志

## [1.0.3] - 未发布

### 新增

- 新增每日定时 Rolling 构建。

## [1.0.2] - 2026-11-13

### 修复

- 修复测试问题。
EOF
git -C "$work_repo" add CHANGELOG.md
git -C "$work_repo" commit -q -m "feat: add new feature for 1.0.3"
git -C "$work_repo" push -q origin main
new_head_sha="$(git -C "$work_repo" rev-parse refs/heads/main)"
out="$(run_prepare schedule refs/heads/main false)"
grep -Fq "should_publish=true" <<<"$out"
grep -Fq "needs_ci=false" <<<"$out"
grep -Fq "rolling_version=1.0.3" <<<"$out"
grep -Fq "source_sha=$new_head_sha" <<<"$out"

# 8. Manual dispatch from non-main branch -> must fail
if (
  cd "$work_repo"
  PATH="$mock_bin:$PATH" \
    RUNNER_TEMP="$tmp_dir" \
    GITHUB_OUTPUT="$tmp_dir/github_output_err.txt" \
    GH_TOKEN="dummy" \
    GH_REPO="maybk/tunnelkit" \
    MOCK_RELEASES_JSON_FILE="$releases_json_file" \
    MOCK_CI_JSON_FILE="$ci_json_file" \
    EVENT_NAME="workflow_dispatch" \
    EVENT_REF="refs/heads/feature" \
    FORCE_PUBLISH="false" \
    bash "$prepare_script" >/dev/null 2>&1
); then
  echo "manual Rolling dispatch from a non-main branch must be rejected"
  exit 1
fi
