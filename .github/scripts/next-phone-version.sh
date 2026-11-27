#!/usr/bin/env bash
# next-phone-version.sh <commit> [major|minor|patch]
#
# Prints the phone release tag (mobile-vX.Y.Z) the commit should get, or nothing when it should get none. Without a bump given, the bump follows the Conventional Commits since the last mobile-v* tag that changed the phone app or the workspace packages it imports (apps/mobile, packages/core, packages/remotefs, packages/study, packages/sync): a breaking change (a type ending in "!" or a "BREAKING CHANGE:" footer) is a major bump, feat a minor one, fix, perf and revert a patch one; other types (docs, test, ci, chore, refactor, style, build) and commits outside those paths release nothing. While the major version is 0, a breaking change bumps the minor version, as SemVer leaves 0.x free to break.
#
# Nothing is printed when the commit already has a mobile-v* tag, or when it is not after the newest mobile-v* tag (CI on an older push of main that finished late).
set -euo pipefail

sha=$(git rev-parse "$1^{commit}")
forced=${2:-}

if [ -n "$(git tag --points-at "$sha" --list 'mobile-v[0-9]*')" ]; then
  exit 0
fi

newest=$(git tag --list 'mobile-v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | awk 'NR == 1')
if [ -n "$newest" ] && ! git merge-base --is-ancestor "$newest" "$sha"; then
  exit 0
fi

if [ -n "$newest" ]; then
  last_version=${newest#mobile-v}
  range="$newest..$sha"
else
  last_version=$(git show "$sha:apps/mobile/nsgo.yaml" 2>/dev/null | sed -nE 's/^version: *"?([0-9]+\.[0-9]+\.[0-9]+)"? *$/\1/p' | awk 'NR == 1' || true)
  last_version=${last_version:-0.0.0}
  range=$sha
fi

phone_paths=(
  apps/mobile
  packages/core
  packages/remotefs
  packages/study
  packages/sync
)

bump=$forced
if [ -z "$bump" ]; then
  bump=none
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
      bump=major
      break
    fi
    case $type in
      feat) bump=minor ;;
      fix | perf | revert)
        if [ "$bump" = none ]; then
          bump="patch"
        fi
        ;;
    esac
  done < <(git rev-list --no-merges "$range" -- "${phone_paths[@]}")
fi

if [ "$bump" = none ]; then
  exit 0
fi

IFS=. read -r major minor patch <<<"$last_version"
if [ "$bump" = major ] && [ "$major" -eq 0 ] && [ -z "$forced" ]; then
  bump=minor
fi
case $bump in
  major) echo "mobile-v$((major + 1)).0.0" ;;
  minor) echo "mobile-v$major.$((minor + 1)).0" ;;
  patch) echo "mobile-v$major.$minor.$((patch + 1))" ;;
  *)
    echo "unknown bump: $bump" >&2
    exit 1
    ;;
esac
