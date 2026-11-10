#!/usr/bin/env bash
# changelog-test.sh
#
# Checks changelog.sh on a repository of its own, made in a temporary folder: the section add writes from the commits since the previous version tag (pull requests by their titles, scopes that are not the app left out, reverts settled), that it puts a section above those of older versions and keeps one already there, the line it writes for a version with nothing to list, and that notes prints a section the way mygo build reads it.
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

# merge <branch> <message...> merges the branch into the current one in a merge commit with the date commit gives, and the message's paragraphs, as GitHub writes a pull request's merge with its title in the body.
merge() {
  local branch=$1 args=() m
  shift
  for m in "$@"; do
    args+=(-m "$m")
  done
  GIT_AUTHOR_DATE=2026-01-02T00:00:00Z GIT_COMMITTER_DATE=2026-01-02T00:00:00Z git merge -q --no-ff "${args[@]}" "$branch"
}

# branch <name> <subject...> makes a branch off the current commit with a commit for each subject, and goes back.
branch() {
  local name=$1 subject
  shift
  git checkout -q -b "$name"
  for subject in "$@"; do
    commit "$subject"
  done
  git checkout -q -
}

commit "feat: first"
commit "feat(films): add a filter"
git tag -a v0.1.0 -m v0.1.0
commit "fix(player): stop on the last frame"
commit "docs: explain the queue"
commit "feat(films): add a sort menu"
commit "refactor: split the view"
commit "perf: draw fewer frames"
commit "feat(shelf)!: move the library"
commit "fix: keep the cue" "BREAKING CHANGE: cues are stored apart"
commit "revert: feat(films): add a sort menu"
commit "revert: feat(films): add a filter"
commit "not a conventional commit"
commit "fix: stop on the last frame"
commit "feat(site): redesign the landing page"
commit "fix(deps): bump a library"
commit "fix(dev): keep the window across reloads"
branch side "fix: from a merged branch"
merge side "Merge branch side"
branch volume "feat(player): add volume control" "fix(player): address review findings" "feat(media)!: drop the old cache"
merge volume "Merge pull request #2 from example/volume" "feat(player): add volume control and shortcuts"
branch icon "feat(site): rework the page" "feat(brand): replace the app icon"
merge icon "Merge pull request #3 from example/icon" "feat(site): rework the page and the app icon"

"$script" add v0.2.0
want=$(
  cat <<'END'
# Changelog

What changed in each version of Kando, newest first. The app shows the section of a new version when it offers the update.

## 0.2.0 - 2026-01-02

### Breaking changes

- Move the library
- Keep the cue
- Drop the old cache

### Features

- Add volume control and shortcuts
- Replace the app icon

### Fixes

- Stop on the last frame
- Revert "add a filter"
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

# A section written ahead for a later version stays above the one add writes.
awk '/^## / && !done { print "## 0.4.0"; print ""; print "Written ahead."; print ""; done = 1 } { print }' apps/kando/CHANGELOG.md >changelog.tmp
mv changelog.tmp apps/kando/CHANGELOG.md
commit "fix: keep the order"
"$script" add v0.2.2
expect "older versions below" "## 0.4.0 ## 0.2.2 - 2026-01-02 ## 0.2.1 - 2026-01-02 ## 0.2.0 - 2026-01-02" "$(grep '^## ' apps/kando/CHANGELOG.md | paste -sd ' ' -)"
expect "section between" "### Fixes

- Keep the order" "$("$script" notes v0.2.2)"

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
