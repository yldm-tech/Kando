#!/usr/bin/env bash
# phone-notes.sh <mobile-vX.Y.Z> [commit]
#
# Prints Markdown release notes for a phone release from the Conventional Commits since the previous mobile-v* tag that changed the phone app or the workspace packages it imports (apps/mobile, packages/core, packages/remotefs, packages/study, packages/sync).
set -euo pipefail

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "usage: phone-notes.sh <mobile-vX.Y.Z> [commit]" >&2
  exit 2
fi

version=${1#mobile-}
version=${version#v}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "not a version: $1" >&2
  exit 1
fi

sha=$(git rev-parse "${2:-HEAD}^{commit}")
previous=$(git tag --list 'mobile-v[0-9]*.[0-9]*.[0-9]*' --merged "$sha" --sort=-v:refname | grep -vxF "mobile-v$version" | awk 'NR == 1' || true)
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

skip_scopes=" site deps dev "

classify() {
  local type_re='^([a-z]+)(\(([^)]*)\))?(!?): (.+)$'
  local type scope breaking text group
  if [[ ! $1 =~ $type_re ]]; then
    return 1
  fi
  type=${BASH_REMATCH[1]}
  scope=${BASH_REMATCH[3]}
  breaking=${BASH_REMATCH[4]}
  text=${BASH_REMATCH[5]}
  if [ "$breaking" = "!" ] || grep -qE '^BREAKING[ -]CHANGE:' <<<"$2"; then
    group=breaking
  else
    if [ -n "$scope" ] && [[ $skip_scopes == *" $scope "* ]]; then
      return 1
    fi
    case $type in
      feat) group=feat ;;
      fix) group=fix ;;
      revert) group=revert ;;
      perf) group=perf ;;
      *) return 1 ;;
    esac
  fi
  if [ "$type" = revert ] && [[ $text =~ $type_re ]]; then
    text=${BASH_REMATCH[5]}
  fi
  printf '%s\t%s\n' "$group" "$text"
}

entries() {
  local commit
  while read -r commit; do
    classify "$(git log -1 --format=%s "$commit")" "$(git log -1 --format=%b "$commit")" || true
  done < <(git rev-list --no-merges --reverse "$range" -- "${phone_paths[@]}")
}

settle() {
  awk -F '\t' '
    { g[NR] = $1; t[NR] = $2; if ($1 == "revert") r[$2]++; else n[$2]++ }
    END {
      for (i = 1; i <= NR; i++) {
        if (g[i] == "revert") {
          if (r[t[i]] > n[t[i]]) print "fix\tRevert \"" t[i] "\""
        } else if (n[t[i]] > r[t[i]]) {
          print g[i] "\t" toupper(substr(t[i], 1, 1)) substr(t[i], 2)
        }
      }
    }
  '
}

list=$(entries | settle)
if [ -z "$list" ]; then
  echo "No changes to the phone app worth listing: this version only carries maintenance."
  exit 0
fi

printed=0
for group in breaking feat fix perf; do
  case $group in
    breaking) title="Breaking changes" ;;
    feat) title="Features" ;;
    fix) title="Fixes" ;;
    perf) title="Performance" ;;
  esac
  lines=$(awk -F '\t' -v g="$group" '$1 == g && !seen[$2]++ { print "- " $2 }' <<<"$list")
  if [ -z "$lines" ]; then
    continue
  fi
  if [ "$printed" -eq 1 ]; then
    echo
  fi
  echo "### $title"
  echo
  echo "$lines"
  printed=1
done
