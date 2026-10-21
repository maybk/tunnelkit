#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -gt 1 ]; then
  echo "Usage: bash scripts/extract_pending_version.sh [CHANGELOG]" >&2
  exit 64
fi

changelog="${1:-CHANGELOG.md}"
if [ ! -f "$changelog" ]; then
  echo "Changelog was not found: $changelog" >&2
  exit 1
fi

first_version_heading="$(sed -n '/^## \[/ { p; q; }' "$changelog")"
if [[ ! "$first_version_heading" =~ ^##[[:space:]]\[([0-9]+\.[0-9]+\.[0-9]+)\][[:space:]]-[[:space:]]未发布$ ]]; then
  echo "The first changelog version section must be: ## [MAJOR.MINOR.PATCH] - 未发布" >&2
  exit 1
fi
version="${BASH_REMATCH[1]}"

pending_count="$(grep -Ec '^## \[[0-9]+\.[0-9]+\.[0-9]+\] - 未发布$' "$changelog" || true)"
if [ "$pending_count" -ne 1 ]; then
  echo "Expected exactly one pending changelog version; found $pending_count" >&2
  exit 1
fi

printf '%s\n' "$version"
