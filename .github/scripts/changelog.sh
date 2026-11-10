#!/usr/bin/env bash
# changelog.sh add <vX.Y.Z> [commit]
# changelog.sh notes <vX.Y.Z>
#
# Kando's release notes live in apps/kando/CHANGELOG.md, beside mygo.json, a "## X.Y.Z - date" section per version, newest first. mygo build puts the section of the version it builds into the update manifests it signs (updates.changelog in MyGo's docs/updates.md), which the app's update window shows, and fails when the file has no section for that version. The Release workflow writes the section with add when it bumps the version, and makes the same section the GitHub release's notes with notes.
#
# add writes the section of a version from the Conventional Commits from the version tag before the commit (HEAD by default) to the commit, the range next-version.sh reads: breaking changes (a type ending in "!" or a "BREAKING CHANGE:" footer), features (feat), fixes (fix and revert) and performance (perf), each under a heading of its own, as their subjects without the type. Merge commits are skipped, as the commits they bring are listed themselves, and so are the types that release nothing (docs, test, ci, chore, refactor, style, build). A version with nothing to list gets a line that says so, as mygo build fails on an empty section. A version that has a section already keeps it, so notes written by hand before the release are the ones that ship.
#
# notes prints the section of a version without its heading, as mygo build reads it.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
file=$root/apps/kando/CHANGELOG.md
new=$(mktemp)
trap 'rm -f "$new" "$file.tmp"' EXIT

usage() {
  echo "usage: changelog.sh add <vX.Y.Z> [commit] | changelog.sh notes <vX.Y.Z>" >&2
  exit 2
}

# version <tag> prints the tag's version without the v, and stops when it is not one.
version() {
  local v=${1#v}
  if [[ ! $v =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "not a version: $1" >&2
    exit 1
  fi
  echo "$v"
}

# section <version> prints the lines of the version's section, without its heading and the blank lines around them, the way MyGo finds it (update.ReleaseNotes): from the first "## " heading whose first word is the version, with or without a v and in brackets or not, to the next "## " heading. It fails when the file has no such heading.
section() {
  if [ ! -f "$file" ]; then
    return 1
  fi
  awk -v want="$1" '
    /^## / {
      if (on) exit
      v = $2
      if (v ~ /^\[/) { v = substr(v, 2); sub(/\].*/, "", v) }
      sub(/^v/, "", v)
      if (v == want) { on = 1; found = 1 }
      next
    }
    on {
      lines[++n] = $0
      if ($0 ~ /[^ \t]/) { if (!first) first = n; last = n }
    }
    END {
      if (!found) exit 1
      for (i = first; first && i <= last; i++) print lines[i]
    }
  ' "$file"
}

# entries <range> prints a "<group>\t<line>" for each commit of the range worth listing, oldest first.
entries() {
  local type_re='^([a-z]+)(\([^)]*\))?(!?): (.+)$'
  local commit subject body type breaking text group
  while read -r commit; do
    subject=$(git log -1 --format=%s "$commit")
    body=$(git log -1 --format=%b "$commit")
    if [[ ! $subject =~ $type_re ]]; then
      continue
    fi
    type=${BASH_REMATCH[1]}
    breaking=${BASH_REMATCH[3]}
    text=${BASH_REMATCH[4]}
    if [ "$breaking" = "!" ] || grep -qE '^BREAKING[ -]CHANGE:' <<<"$body"; then
      group=breaking
    else
      case $type in
        feat) group=feat ;;
        fix | revert) group=fix ;;
        perf) group=perf ;;
        *) continue ;;
      esac
    fi
    # A revert's subject is the subject of the commit it reverts, listed without that one's type, as a "Revert" line.
    if [ "$type" = revert ]; then
      if [[ $text =~ $type_re ]]; then
        text=${BASH_REMATCH[4]}
      fi
      text="Revert \"$text\""
    fi
    # The subject starts with a capital, as a line of notes does; tr keeps to ASCII, which Conventional Commit subjects start with.
    printf '%s\t%s%s\n' "$group" "$(printf %s "${text:0:1}" | tr '[:lower:]' '[:upper:]')" "${text:1}"
  done < <(git rev-list --no-merges --reverse "$1")
}

# write <version> <commit> prints the section of the version, heading included.
write() {
  local version=$1 commit=$2 sha previous range list group title
  sha=$(git rev-parse "$commit^{commit}")
  # The version's own tag is skipped, so that a section can be written again for a version already tagged.
  previous=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --merged "$sha" --sort=-v:refname | grep -vxF "v$version" | awk 'NR == 1' || true)
  range=$sha
  if [ -n "$previous" ]; then
    range="$previous..$sha"
  fi
  list=$(entries "$range")

  echo "## $version - $(git log -1 --format=%cs "$sha")"
  echo
  if [ -z "$list" ]; then
    echo "No changes to the app worth listing: this version only carries maintenance."
    return
  fi
  local printed=0
  for group in breaking feat fix perf; do
    case $group in
      breaking) title="Breaking changes" ;;
      feat) title="Features" ;;
      fix) title="Fixes" ;;
      perf) title="Performance" ;;
    esac
    # A subject that two commits share, such as a change made again after a revert of it, is listed once.
    local lines
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
}

# add <version> <commit> puts the version's section above the newest one, or under the file's introduction when it has none.
add() {
  local version=$1 commit=$2 notes
  if notes=$(section "$version"); then
    if [ -z "$notes" ]; then
      echo "apps/kando/CHANGELOG.md has an empty section for $version, which mygo build fails on: write its notes or remove its heading" >&2
      exit 1
    fi
    echo "apps/kando/CHANGELOG.md has notes for $version already; keeping them" >&2
    return
  fi
  if [ ! -f "$file" ]; then
    printf '# Changelog\n\nWhat changed in each version of Kando, newest first. The app shows the section of a new version when it offers the update.\n' >"$file"
  fi
  write "$version" "$commit" >"$new"
  awk -v new="$new" '
    function put() { while ((getline line < new) > 0) print line; print ""; done = 1 }
    !done && /^## / { put() }
    { print }
    END {
      if (!done) {
        print ""
        put()
      }
    }
  ' "$file" >"$file.tmp"
  # The file ends with one newline, however many blank lines the last section left.
  awk '{ lines[NR] = $0 } END { n = NR; while (n > 0 && lines[n] == "") n--; for (i = 1; i <= n; i++) print lines[i] }' "$file.tmp" >"$file"
  rm -f "$file.tmp"
}

case ${1:-} in
  add)
    [ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
    v=$(version "$2")
    add "$v" "${3:-HEAD}"
    ;;
  notes)
    [ "$#" -eq 2 ] || usage
    v=$(version "$2")
    if ! notes=$(section "$v") || [ -z "$notes" ]; then
      echo "apps/kando/CHANGELOG.md has no notes for $v" >&2
      exit 1
    fi
    echo "$notes"
    ;;
  *) usage ;;
esac
