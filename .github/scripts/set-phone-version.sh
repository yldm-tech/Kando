#!/usr/bin/env bash
# set-phone-version.sh <mobile-vX.Y.Z>
#
# Writes a phone release's version into apps/mobile/nsgo.yaml, from which nsgo build names the iOS and Android packages. Fails if the file does not hold its version line exactly once.
set -euo pipefail

tag=$1
version=${tag#mobile-}
version=${version#v}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "not a version: $tag" >&2
  exit 1
fi

root=$(git rev-parse --show-toplevel)
file=$root/apps/mobile/nsgo.yaml
pattern='^version: [0-9.]+$'
count=$(grep -cE "$pattern" "$file" || true)
if [ "$count" -ne 1 ]; then
  echo "apps/mobile/nsgo.yaml: expected one line matching $pattern, found $count" >&2
  exit 1
fi
sed -E "s/$pattern/version: $version/" "$file" >"$file.tmp"
mv "$file.tmp" "$file"
