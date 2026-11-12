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

bash "$render_script" "v1.2.3" "$tmp_dir/SHA256SUMS.txt" "$tmp_dir" "2026-11-12"
cask_file="$tmp_dir/Casks/tunnelkit.rb"
scoop_file="$tmp_dir/bucket/tunnelkit.json"

for required_cask in \
  'cask "tunnelkit" do' \
  'arch arm: "arm64", intel: "amd64"' \
  'version "1.2.3"' \
  'arm64_linux:' \
  'x86_64_linux:' \
  'on_macos do' \
  'depends_on macos: :monterey' \
  'app "TunnelKit.app"' \
  'postflight_steps do' \
  'run "/usr/bin/xattr"' \
  'args:           ["-rd", "com.apple.quarantine", "{{appdir}}/TunnelKit.app"]' \
  'uninstall quit: "com.tunnelkit.app"' \
  '"/Library/PrivilegedHelperTools/com.tunnelkit.helper"' \
  'on_linux do' \
  'binary "tunnelkit"' \
  'binary "tunnelkit-helper"' \
  '"/usr/libexec/tunnelkit"' \
  '1111111111111111111111111111111111111111111111111111111111111111'; do
  if ! grep -Fq -- "$required_cask" "$cask_file"; then
    echo "Rendered Cask is missing required stanza: $required_cask" >&2
    exit 1
  fi
done
if grep -Eq '(["/])tunnelkit\.app"' "$cask_file"; then
  echo "Rendered Cask must reference TunnelKit.app matching package_macos_dmg.sh, not lowercase tunnelkit.app" >&2
  exit 1
fi

if command -v ruby >/dev/null 2>&1; then
  ruby -c "$cask_file" >/dev/null
fi

jq -e '
  .version == "1.2.3" and
  .architecture["64bit"].hash == "5555555555555555555555555555555555555555555555555555555555555555" and
  .architecture["arm64"].hash == "6666666666666666666666666666666666666666666666666666666666666666" and
  (.bin // null) == null and
  .shortcuts == [["tunnelkit.exe", "TunnelKit"]] and
  .checkver == "github" and
  .autoupdate.hash.url == "$baseurl/SHA256SUMS.txt" and
  any(.pre_uninstall[]; contains("$cmd -eq '\''uninstall'\''") and contains("TunnelKitHelper"))
' "$scoop_file" >/dev/null

diff -u "$scoop_file" <(jq --indent 4 . "$scoop_file")

winget_out="$tmp_dir/winget/manifests/m/maybk/TunnelKit/1.2.3"
for wf in \
  "$winget_out/maybk.TunnelKit.yaml" \
  "$winget_out/maybk.TunnelKit.installer.yaml" \
  "$winget_out/maybk.TunnelKit.locale.en-US.yaml" \
  "$winget_out/maybk.TunnelKit.locale.zh-CN.yaml"; do
  test -s "$wf"
  if command -v ruby >/dev/null 2>&1; then
    ruby -rpsych -rdate -e 'Psych.safe_load_file(ARGV[0], permitted_classes: [Date])' "$wf"
  fi
done
grep -Fq 'InstallerType: nullsoft' "$winget_out/maybk.TunnelKit.installer.yaml"
grep -Fq 'ElevationRequirement: elevatesSelf' "$winget_out/maybk.TunnelKit.installer.yaml"
grep -Fq 'ReleaseDate: 2026-11-12' "$winget_out/maybk.TunnelKit.installer.yaml"
grep -Fq "DefaultInstallLocation: '%ProgramFiles%\\maybk\\TunnelKit'" "$winget_out/maybk.TunnelKit.installer.yaml"
grep -Fq '7777777777777777777777777777777777777777777777777777777777777777' "$winget_out/maybk.TunnelKit.installer.yaml"
grep -Fq -- '- fortinet' "$winget_out/maybk.TunnelKit.locale.en-US.yaml"
grep -Fq -- '- 网络' "$winget_out/maybk.TunnelKit.locale.zh-CN.yaml"

bash "$render_script" "1.2.3" "$tmp_dir/SHA256SUMS.txt" "$tmp_dir/no-date"
if grep -Fq 'ReleaseDate:' "$tmp_dir/no-date/winget/manifests/m/maybk/TunnelKit/1.2.3/maybk.TunnelKit.installer.yaml"; then
  echo "Unexpected ReleaseDate when release-date argument is omitted" >&2
  exit 1
fi
