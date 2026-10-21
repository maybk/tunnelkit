#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo "Usage: $0 <app_bundle_path> <output_dmg_path> [volume_name]" >&2
  exit 1
fi

bundle_path="$1"
dmg_path="$2"
volume_name="${3:-TunnelKit}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
settings_file="$repo_root/build/darwin/dmg/settings.py"
assets_dir="$repo_root/build/darwin/dmg"
volume_icon="$repo_root/build/darwin/icons.icns"

if [ ! -d "$bundle_path" ]; then
  echo "App bundle does not exist: $bundle_path" >&2
  exit 1
fi

dmg_python="python3"
if ! "$dmg_python" -c "import dmgbuild" >/dev/null 2>&1; then
  venv_dir="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/tunnelkit-dmgbuild-venv"
  if [ ! -x "$venv_dir/bin/python3" ] || ! "$venv_dir/bin/python3" -c "import dmgbuild" >/dev/null 2>&1; then
    python3 -m venv "$venv_dir"
    "$venv_dir/bin/pip" install --disable-pip-version-check --quiet "dmgbuild==1.6.7"
  fi
  dmg_python="$venv_dir/bin/python3"
fi

mkdir -p "$(dirname "$dmg_path")"
rm -f "$dmg_path"

"$dmg_python" -m dmgbuild \
  -s "$settings_file" \
  -D "app_path=$bundle_path" \
  -D "app_name=TunnelKit.app" \
  -D "assets_dir=$assets_dir" \
  -D "volume_icon=$volume_icon" \
  --detach-retries 15 \
  "$volume_name" \
  "$dmg_path"

test -s "$dmg_path"
hdiutil verify "$dmg_path" >/dev/null
