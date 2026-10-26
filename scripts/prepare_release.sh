#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target_version=""
release_date="$(date '+%Y-%m-%d')"
skip_wails=false

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root)
      repo_root="$2"
      shift 2
      ;;
    --date)
      release_date="$2"
      shift 2
      ;;
    --skip-wails)
      skip_wails=true
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [version] [--date YYYY-MM-DD] [--root <dir>] [--skip-wails]"
      exit 0
      ;;
    *)
      if [ -z "$target_version" ]; then
        target_version="${1#v}"
        shift
      else
        echo "Unexpected argument: $1" >&2
        exit 1
      fi
      ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
config_file="$repo_root/build/config.yml"
changelog_file="$repo_root/CHANGELOG.md"

pending_version="$(bash "$script_dir/extract_pending_version.sh" "$changelog_file")"
if [ -z "$target_version" ]; then
  target_version="$pending_version"
elif [ "$target_version" != "$pending_version" ]; then
  echo "Target version $target_version does not match pending CHANGELOG version $pending_version" >&2
  exit 1
fi

if [[ ! "$target_version" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "Invalid semantic version: $target_version" >&2
  exit 1
fi
major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
patch="${BASH_REMATCH[3]}"
next_version="${major}.${minor}.$((patch + 1))"

if [[ ! "$release_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  echo "Invalid release date (expected YYYY-MM-DD): $release_date" >&2
  exit 1
fi

bash "$script_dir/extract_release_notes.sh" "$target_version" "$changelog_file" >/dev/null

tmp_config="$(mktemp)"
awk -v ver="$target_version" '
  BEGIN { in_info = 0; updated = 0 }
  /^[^[:space:]#]/ {
    in_info = ($1 == "info:")
  }
  in_info && !updated && /^[[:space:]]+version:[[:space:]]*/ {
    sub(/version:[[:space:]]*"[^"]*"/, "version: \"" ver "\"")
    updated = 1
  }
  { print }
  END {
    if (!updated) exit 1
  }
' "$config_file" > "$tmp_config"
mv "$tmp_config" "$config_file"

tmp_changelog="$(mktemp)"
awk -v cur="$target_version" -v next_ver="$next_version" -v rdate="$release_date" '
  BEGIN { replaced = 0 }
  !replaced && $0 == "## [" cur "] - 未发布" {
    print "## [" next_ver "] - 未发布"
    print ""
    print "## [" cur "] - " rdate
    replaced = 1
    next
  }
  { print }
  END {
    if (!replaced) exit 1
  }
' "$changelog_file" > "$tmp_changelog"
mv "$tmp_changelog" "$changelog_file"

bash "$script_dir/extract_pending_version.sh" "$changelog_file" >/dev/null
bash "$script_dir/extract_release_notes.sh" "$target_version" "$changelog_file" >/dev/null

cat <<EOF
Prepared release v${target_version} (${release_date}), next pending version: ${next_version}

Next steps:
  git commit -am "chore(release): v${target_version}"
  git tag -a "v${target_version}" -m "TunnelKit v${target_version}"
  git push --atomic origin main "v${target_version}"
EOF
