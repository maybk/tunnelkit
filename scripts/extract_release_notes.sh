#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "Usage: bash scripts/extract_release_notes.sh VERSION [CHANGELOG]" >&2
  exit 64
fi

version="$1"
changelog="${2:-CHANGELOG.md}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Release version must match MAJOR.MINOR.PATCH: $version" >&2
  exit 1
fi

if [ ! -f "$changelog" ]; then
  echo "Changelog was not found: $changelog" >&2
  exit 1
fi

heading="## [$version]"
heading_count="$(
  awk -v heading="$heading" '
    $0 == heading || index($0, heading " - ") == 1 { count++ }
    END { print count + 0 }
  ' "$changelog"
)"

if [ "$heading_count" -ne 1 ]; then
  echo "Expected exactly one changelog section for $version; found $heading_count" >&2
  exit 1
fi

set +e
awk -v heading="$heading" '
  function is_target(line) {
    return line == heading || index(line, heading " - ") == 1
  }

  is_target($0) {
    found = 1
    next
  }

  found && /^## \[/ {
    exit
  }

  found {
    lines[++count] = $0
  }

  END {
    first = 1
    while (first <= count && lines[first] ~ /^[[:space:]]*$/) {
      first++
    }
    while (count >= first && lines[count] ~ /^[[:space:]]*$/) {
      count--
    }
    if (first > count) {
      exit 2
    }
    for (line_index = first; line_index <= count; line_index++) {
      print lines[line_index]
    }
  }
' "$changelog"
status="$?"
set -e

if [ "$status" -eq 2 ]; then
  echo "Changelog section for $version is empty" >&2
  exit 2
fi
if [ "$status" -ne 0 ]; then
  echo "Could not extract changelog section for $version" >&2
  exit "$status"
fi
