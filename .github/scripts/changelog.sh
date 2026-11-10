#!/usr/bin/env bash
# changelog.sh add <vX.Y.Z> [commit]
# changelog.sh notes <vX.Y.Z>
#
# Kando's release notes live in apps/kando/CHANGELOG.md, beside mygo.json, a "## X.Y.Z - date" section per version, newest first. mygo build puts the section of the version it builds into the update manifests it signs (updates.changelog in MyGo's docs/updates.md), which the app's update window shows, and fails when the file has no section for that version. The Release workflow writes the section with add when it bumps the version, and makes the same section the GitHub release's notes with notes.
#
# add writes the section of a version from the Conventional Commits from the version tag before the commit (HEAD by default) to the commit, the range next-version.sh reads: breaking changes (a type ending in "!" or a "BREAKING CHANGE:" footer), features (feat), fixes (fix and revert) and performance (perf), each under a heading of its own, as their subjects without the type. A pull request's merge commit stands for the pull request with its title, so the commits that made it, review fixes among them, are not listed one by one (see entries). The types that release nothing (docs, test, ci, chore, refactor, style, build) are left out, and so are the scopes that are not the app (site, deps, dev), so a release of nothing else gets a line that says so, as mygo build fails on an empty section. A change and a revert of it in the same range are both left out. A version that has a section already keeps it, so notes written by hand before the release are the ones that ship, and the way to leave a change out of them; write one only for the version next-version.sh gives the commits, as it is kept as it is when that version comes. The new section goes above the sections of older versions.
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

# Scopes of changes that are not the app's, left out of its notes: the landing page, dependency bumps and the mygo dev loop.
skip_scopes=" site deps dev "

# classify <subject> <body> prints the "<group>\t<text>" line of a Conventional Commit subject worth listing, and fails for one that is not: another type, a skipped scope or no Conventional Commit at all. The group is breaking, feat, fix, perf or revert, and the text is the subject without its type; a revert's is the subject it reverts, without that one's type.
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

# entries <range> prints a "<group>\t<text>" line for each change of the range worth listing, oldest first. It walks main's own history (the first parents): a commit made on main is listed by its subject, and a pull request's merge commit by the pull request's title, the first line of its message's body, which describes the change as a whole rather than the commits that made it, review fixes among them. A merge whose title is not worth listing, such as one about the landing page that also changed the app's icon, or one without a title, as git merge writes, lists the commits it brings instead, and one whose title is listed still lists the breaking changes it brings under a title that does not say so.
entries() {
  local commit subject body title rest line c
  while read -r commit; do
    if [ -z "$(git rev-parse -q --verify "$commit^2" 2>/dev/null)" ]; then
      classify "$(git log -1 --format=%s "$commit")" "$(git log -1 --format=%b "$commit")" || true
      continue
    fi
    body=$(git log -1 --format=%b "$commit")
    title=$(awk 'NF { print; exit }' <<<"$body")
    rest=$(awk 'NF && !t { t = 1; next } t' <<<"$body")
    if [ -n "$title" ] && line=$(classify "$title" "$rest"); then
      echo "$line"
      if [ "${line%%$'\t'*}" = breaking ]; then
        continue
      fi
      while read -r c; do
        subject=$(git log -1 --format=%s "$c")
        line=$(classify "$subject" "$(git log -1 --format=%b "$c")") || continue
        if [ "${line%%$'\t'*}" = breaking ]; then
          echo "$line"
        fi
      done < <(git rev-list --no-merges --reverse "$commit^1..$commit")
      continue
    fi
    while read -r c; do
      classify "$(git log -1 --format=%s "$c")" "$(git log -1 --format=%b "$c")" || true
    done < <(git rev-list --no-merges --reverse "$commit^1..$commit")
  done < <(git rev-list --first-parent --reverse "$1")
}

# settle prints the entries with the reverts settled: a change that a revert in the same range undoes is not listed, nor is that revert, as neither ships; a revert of a change from an earlier version is a fix, written as Revert "<text>". A change made again after its revert is listed once. Each text then starts with a capital, as a line of notes does (toupper keeps to ASCII, which Conventional Commit subjects start with).
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
  list=$(entries "$range" | settle)

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
    # A text that two changes share is listed once.
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

# add <version> <commit> puts the version's section above the first section of an older version, or at the end of the file when it has none, which leaves sections of newer versions, and headings that are no version, such as "## Unreleased", above it.
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
  awk -v new="$new" -v want="$version" '
    function put() { while ((getline line < new) > 0) print line; print ""; done = 1 }
    # older(a, b) tells whether version a comes before version b.
    function older(a, b,   x, y, i) {
      split(a, x, "."); split(b, y, ".")
      for (i = 1; i <= 3; i++) if (x[i] + 0 != y[i] + 0) return x[i] + 0 < y[i] + 0
      return 0
    }
    !done && /^## / {
      v = $2
      if (v ~ /^\[/) { v = substr(v, 2); sub(/\].*/, "", v) }
      sub(/^v/, "", v)
      if (v ~ /^[0-9]+\.[0-9]+\.[0-9]+$/ && older(v, want)) put()
    }
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
