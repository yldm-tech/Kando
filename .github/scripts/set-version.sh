#!/usr/bin/env bash
# set-version.sh <vX.Y.Z>
#
# Writes a release's version into the files that carry one: the app's mygo.json, from which mygo build names the app's version and its packages. The file must hold its version exactly once, so that a moved or renamed line fails here rather than going unbumped.
set -euo pipefail

tag=$1
version=${tag#v}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "not a version: $tag" >&2
  exit 1
fi

root=$(git rev-parse --show-toplevel)

# bump <file> <pattern of the line> <the line with the new version>
bump() {
  local file=$root/$1 pattern=$2 line=$3 count
  count=$(grep -cE "$pattern" "$file" || true)
  if [ "$count" -ne 1 ]; then
    echo "$1: expected one line matching $pattern, found $count" >&2
    exit 1
  fi
  sed -E "s/$pattern/$line/" "$file" >"$file.tmp"
  mv "$file.tmp" "$file"
}

bump apps/kando/mygo.json '^  "version": "[^"]*",$' "  \"version\": \"$version\","
