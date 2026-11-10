#!/usr/bin/env bash
# changelog-test.sh
#
# Checks changelog.sh on a repository of its own, made in a temporary folder: the section add writes from the commits since the previous version tag, that it puts the newest section first and keeps one already there, the line it writes for a version with nothing to list, and that notes prints a section the way mygo build reads it.
set -euo pipefail

script=$(cd "$(dirname "$0")" && pwd)/changelog.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

git init -q
git config user.name test
git config user.email test@example.com
git config commit.gpgsign false
git config tag.gpgsign false
mkdir -p apps/kando

# commit <subject> [body] makes an empty commit dated 2026-01-02, the date the sections take from the commit they are written at.
commit() {
  GIT_AUTHOR_DATE=2026-01-02T00:00:00Z GIT_COMMITTER_DATE=2026-01-02T00:00:00Z git commit -q --allow-empty -m "$1" ${2:+-m "$2"}
}

fail() {
  echo "changelog-test: $1" >&2
  exit 1
}

# expect <name> <want> <got>
expect() {
  if [ "$2" != "$3" ]; then
    printf 'changelog-test: %s\n--- want\n%s\n--- got\n%s\n' "$1" "$2" "$3" >&2
    exit 1
  fi
}

commit "feat: first"
git tag -a v0.1.0 -m v0.1.0
commit "fix(player): stop on the last frame"
commit "docs: explain the queue"
commit "feat(films): add a sort menu"
commit "refactor: split the view"
commit "perf: draw fewer frames"
commit "feat(shelf)!: move the library"
commit "fix: keep the cue" "BREAKING CHANGE: cues are stored apart"
commit "revert: feat(films): add a sort menu"
commit "not a conventional commit"
commit "fix: stop on the last frame"
git checkout -q -b side
commit "fix: from a merged branch"
git checkout -q -
GIT_COMMITTER_DATE=2026-01-02T00:00:00Z git merge -q --no-ff -m "Merge branch side" side

"$script" add v0.2.0
want=$(
  cat <<'END'
# Changelog

What changed in each version of Kando, newest first. The app shows the section of a new version when it offers the update.

## 0.2.0 - 2026-01-02

### Breaking changes

- Move the library
- Keep the cue

### Features

- Add a sort menu

### Fixes

- Stop on the last frame
- Revert "add a sort menu"
- From a merged branch

### Performance

- Draw fewer frames
END
)
expect "first section" "$want" "$(cat apps/kando/CHANGELOG.md)"

git tag -a v0.2.0 -m v0.2.0
commit "chore: tidy"
"$script" add v0.2.1
expect "newest first" "## 0.2.1 - 2026-01-02 ## 0.2.0 - 2026-01-02" "$(grep '^## ' apps/kando/CHANGELOG.md | paste -sd ' ' -)"
expect "nothing to list" "No changes to the app worth listing: this version only carries maintenance." "$("$script" notes v0.2.1)"

before=$(cat apps/kando/CHANGELOG.md)
"$script" add v0.2.0 2>/dev/null
expect "a section already there is kept" "$before" "$(cat apps/kando/CHANGELOG.md)"

# A section may be written by hand, with the heading Keep a Changelog uses, and is what notes prints.
printf '# Changelog\n\n## [0.3.0] - 2026-02-03\n\n\nWritten by hand.\n\n## 0.2.1\n\nOlder.\n' >apps/kando/CHANGELOG.md
"$script" add v0.3.0 2>/dev/null
expect "hand-written notes" "Written by hand." "$("$script" notes 0.3.0)"

printf '# Changelog\n\n## 0.3.0\n\n## 0.2.1\n\nOlder.\n' >apps/kando/CHANGELOG.md
if "$script" add v0.3.0 2>/dev/null; then
  fail "add accepted an empty section, which mygo build fails on"
fi
if "$script" notes v0.3.0 2>/dev/null; then
  fail "notes printed an empty section"
fi
if "$script" notes v9.0.0 2>/dev/null; then
  fail "notes printed a version that has no section"
fi
if "$script" add 1.2 2>/dev/null; then
  fail "add accepted 1.2 as a version"
fi

echo "changelog-test: ok"
