#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
render_script="$repo_root/scripts/render_package_manifests.sh"

if [ ! -f "$render_script" ]; then
  echo "missing package manifest renderer: ${render_script#"$repo_root/"}" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

cat > "$tmp_dir/SHA256SUMS.txt" <<'EOF'
1111111111111111111111111111111111111111111111111111111111111111  tunnelkit-1.2.3-macos-arm64.dmg
2222222222222222222222222222222222222222222222222222222222222222  tunnelkit-1.2.3-macos-amd64.dmg
3333333333333333333333333333333333333333333333333333333333333333  tunnelkit-1.2.3-linux-arm64.tar.gz
4444444444444444444444444444444444444444444444444444444444444444  tunnelkit-1.2.3-linux-amd64.tar.gz
5555555555555555555555555555555555555555555555555555555555555555  tunnelkit-1.2.3-windows-amd64.zip
6666666666666666666666666666666666666666666666666666666666666666  tunnelkit-1.2.3-windows-arm64.zip
7777777777777777777777777777777777777777777777777777777777777777  tunnelkit-1.2.3-windows-amd64-setup.exe
8888888888888888888888888888888888888888888888888888888888888888  tunnelkit-1.2.3-windows-arm64-setup.exe
EOF

bash "$render_script" "v1.2.3" "$tmp_dir/SHA256SUMS.txt" "$tmp_dir"
cask_file="$tmp_dir/Casks/tunnelkit.rb"
scoop_file="$tmp_dir/bucket/tunnelkit.json"

for required_cask in \
  'cask "tunnelkit" do' \
  'arch arm: "arm64", intel: "amd64"' \
  'version "1.2.3"' \
  'arm64_linux:' \
  'x86_64_linux:' \
  'on_macos do' \
  'app "tunnelkit.app"' \
  'postflight_steps do' \
  'run "/usr/bin/xattr"' \
  'args:           ["-rd", "com.apple.quarantine", "{{appdir}}/tunnelkit.app"]' \
  'on_linux do' \
  'binary "tunnelkit"' \
  'binary "tunnelkit-helper"' \
  '1111111111111111111111111111111111111111111111111111111111111111'; do
  if ! grep -Fq -- "$required_cask" "$cask_file"; then
    echo "Rendered Cask is missing required stanza: $required_cask" >&2
    exit 1
  fi
done

if command -v ruby >/dev/null 2>&1; then
  ruby -c "$cask_file" >/dev/null
fi

for required_scoop in \
  '"version": "1.2.3"' \
  '"64bit"' \
  '"arm64"' \
  '"bin": "tunnelkit.exe"' \
  '"TunnelKit"' \
  '"autoupdate"' \
  'SHA256SUMS.txt' \
  '5555555555555555555555555555555555555555555555555555555555555555'; do
  if ! grep -Fq -- "$required_scoop" "$scoop_file"; then
    echo "Rendered Scoop manifest is missing required field: $required_scoop" >&2
    exit 1
  fi
done

if command -v python3 >/dev/null 2>&1; then
  python3 -m json.tool "$scoop_file" >/dev/null
fi

winget_out="$tmp_dir/winget/manifests/m/maybk/TunnelKit/1.2.3"
for wf in \
  "$winget_out/maybk.TunnelKit.yaml" \
  "$winget_out/maybk.TunnelKit.installer.yaml" \
  "$winget_out/maybk.TunnelKit.locale.en-US.yaml" \
  "$winget_out/maybk.TunnelKit.locale.zh-CN.yaml"; do
  test -s "$wf"
done
grep -Fq 'InstallerType: nullsoft' "$winget_out/maybk.TunnelKit.installer.yaml"
grep -Fq '7777777777777777777777777777777777777777777777777777777777777777' "$winget_out/maybk.TunnelKit.installer.yaml"
