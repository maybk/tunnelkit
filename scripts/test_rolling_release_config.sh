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
  'workflow_run:' \
  'workflows: [CI]' \
  'types: [completed]' \
  'branches: [main]' \
  "github.event.workflow_run.conclusion == 'success'" \
  "github.event.workflow_run.event == 'push'" \
  "github.event.workflow_run.head_branch == 'main'" \
  'github.event.workflow_run.head_sha' \
  "github.event_name == 'workflow_run' && github.event.workflow_run.head_sha || github.sha" \
  'workflow_dispatch:' \
  'group: rolling' \
  'cancel-in-progress: true' \
  'refs/remotes/origin/main' \
  'git merge-base --is-ancestor' \
  'v[0-9]+\.[0-9]+\.[0-9]+' \
  '正式发布提交，跳过 Rolling' \
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
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-windows-amd64.zip' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-windows-arm64.zip' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-amd64.tar.gz' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-amd64.deb' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-arm64.tar.gz' \
  'tunnelkit-$ROLLING_VERSION-rolling-$SHORT_SHA-linux-arm64.deb' \
  'SHA256SUMS.txt' \
  '--draft' \
  '--prerelease' \
  'git tag -a --force rolling -m "TunnelKit $ROLLING_VERSION Rolling ($SHORT_SHA)" "$SOURCE_SHA"' \
  '--title "TunnelKit $ROLLING_VERSION Rolling"' \
  'if [ "$asset_count" -ne 10 ]; then'; do
  if ! grep -Fq -- "$required" "$rolling_workflow"; then
    echo "Rolling workflow is missing required contract: $required"
    exit 1
  fi
done

if [ ! -f "$pending_version_extractor" ]; then
  echo "Rolling workflow must use a tested pending changelog version extractor"
  exit 1
fi

if grep -Fq 'GITHUB_SHA' "$rolling_workflow"; then
  echo "Rolling workflow must not use its workflow_run GITHUB_SHA as the source commit"
  exit 1
fi

if grep -Fq 'app_go_version' "$rolling_workflow"; then
  echo "Rolling workflow must read application version solely from build/config.yml"
  exit 1
fi

if ! grep -Fq "github.event_name == 'workflow_dispatch'" "$rolling_workflow" ||
  ! grep -Fq "github.ref == 'refs/heads/main'" "$rolling_workflow"; then
  echo "manual Rolling runs must execute CI and only accept the current main branch"
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
  'wails3 task linux:build' \
  'wails3 task linux:create:deb' \
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
  'gh release delete rolling' \
  ':refs/tags/rolling' \
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
