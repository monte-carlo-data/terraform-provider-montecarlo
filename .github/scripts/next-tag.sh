#!/usr/bin/env bash
# Prints the tag to release HEAD as: v<base>.<n>, where <base> is the major.minor in VERSION
# and <n> is one past the highest patch already tagged on that base. Prints nothing when HEAD
# already carries a version tag, so a rerun tags nothing. Reads tags from the local clone.
set -euo pipefail

base=$(tr -d '[:space:]' < VERSION)
# A major of 2 or above is a new provider major, which is more than a VERSION edit.
if ! [[ $base =~ ^[01]\.(0|[1-9][0-9]*)$ ]]; then
  echo "VERSION must be <major>.<minor> with a major of 0 or 1, got '$base'" >&2
  exit 1
fi

if git tag --points-at HEAD | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  exit 0
fi

last=$(git tag --list "v$base.*" | { grep -E "^v${base//./\\.}\.[0-9]+$" || true; } |
  sed "s/^v$base\.//" | sort -n | tail -1)
echo "v$base.$((${last:--1} + 1))"
