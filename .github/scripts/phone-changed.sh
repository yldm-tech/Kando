#!/usr/bin/env bash
# phone-changed.sh <commit>
#
# Prints "true" when commits since the last vX.Y.Z tag before <commit> changed the phone app or the workspace packages it imports (apps/mobile, packages/core, packages/remotefs, packages/study, packages/sync) with a releasable Conventional Commit (feat, fix, perf, revert, or breaking), and "false" otherwise. Release uses this to decide whether to upload the new vX.Y.Z build to TestFlight and Google Play, while still building the Android APK for every GitHub Release.
set -euo pipefail

sha=$(git rev-parse "$1^{commit}")
previous=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --merged "$sha" --sort=-v:refname | awk 'NR == 1' || true)
range=$sha
if [ -n "$previous" ]; then
  range="$previous..$sha"
fi

phone_paths=(
  apps/mobile
  packages/core
  packages/remotefs
  packages/study
  packages/sync
)

type_re='^([a-z]+)(\([^)]*\))?(!?):'
while read -r commit; do
  subject=$(git log -1 --format=%s "$commit")
  body=$(git log -1 --format=%b "$commit")
  if [[ $subject =~ $type_re ]]; then
    type=${BASH_REMATCH[1]}
    breaking=${BASH_REMATCH[3]}
  else
    continue
  fi
  if [ "$breaking" = "!" ] || grep -qE '^BREAKING[ -]CHANGE:' <<<"$body"; then
    echo "true"
    exit 0
  fi
  case $type in
    feat | fix | perf | revert)
      echo "true"
      exit 0
      ;;
  esac
done < <(git rev-list --no-merges "$range" -- "${phone_paths[@]}")

echo "false"
