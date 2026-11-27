#!/usr/bin/env bash
# phone-release-test.sh
#
# Checks next-phone-version.sh, set-phone-version.sh and phone-notes.sh on a temporary git repository: path filtering (desktop-only commits do not trigger a phone release), Conventional Commit bump rules, 0.x breaking-change behavior, desktop tags not interfering with mobile-v* tags, forced bumps, nsgo.yaml updating and release notes generation.
set -euo pipefail

scripts_dir=$(cd "$(dirname "$0")" && pwd)
next_ver=$scripts_dir/next-phone-version.sh
set_ver=$scripts_dir/set-phone-version.sh
notes_sh=$scripts_dir/phone-notes.sh

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

git init -q
git config user.name test
git config user.email test@example.com
git config commit.gpgsign false
git config tag.gpgsign false

mkdir -p apps/kando apps/mobile packages/sync

cat >apps/mobile/nsgo.yaml <<'EOF'
name: Kando
id: ai.yldm.kando.mobile
version: 0.2.0
EOF

fail() {
  echo "phone-release-test: $1" >&2
  exit 1
}

expect() {
  if [ "$2" != "$3" ]; then
    printf 'phone-release-test: %s\n--- want\n%s\n--- got\n%s\n' "$1" "$2" "$3" >&2
    exit 1
  fi
}

commit_file() {
  local path=$1 subject=$2 body=${3:-}
  printf '%s\n' "$subject" >>"$path"
  git add "$path"
  git commit -q -m "$subject" ${body:+-m "$body"}
}

# Initial commit with no mobile-v* tag yet: base version comes from apps/mobile/nsgo.yaml (0.2.0).
git add apps/mobile/nsgo.yaml
git commit -q -m "chore: init"
expect "no releasable commits yet" "" "$("$next_ver" HEAD)"

commit_file apps/mobile/app.go "feat(mobile): initial player"
expect "first feat bumps from nsgo.yaml version" "mobile-v0.3.0" "$("$next_ver" HEAD)"

git tag -a mobile-v0.2.0 -m mobile-v0.2.0
expect "already tagged commit releases nothing" "" "$("$next_ver" HEAD)"

# Desktop-only commits and desktop tags do not trigger a phone release.
commit_file apps/kando/main.go "feat(films): desktop-only feature"
git tag -a v0.75.0 -m v0.75.0
expect "desktop-only feat releases nothing for phone" "" "$("$next_ver" HEAD)"

# Non-releasable commits on mobile paths do not trigger a phone release.
commit_file apps/mobile/app.go "docs(mobile): clarify comment"
commit_file apps/mobile/app.go "chore(mobile): tidy"
expect "chore/docs on mobile release nothing" "" "$("$next_ver" HEAD)"

# Forced bump works even when no releasable mobile commits exist.
expect "forced patch bump" "mobile-v0.2.1" "$("$next_ver" HEAD patch)"
expect "forced minor bump" "mobile-v0.3.0" "$("$next_ver" HEAD minor)"
expect "forced major bump" "mobile-v1.0.0" "$("$next_ver" HEAD major)"

# A fix on a shared mobile package triggers a patch bump.
commit_file packages/sync/client.go "fix(sync): retry dropped connection"
expect "fix on packages/sync triggers patch" "mobile-v0.2.1" "$("$next_ver" HEAD)"

# A feat on apps/mobile promotes the bump to minor.
commit_file apps/mobile/app.go "feat(mobile): unlock encrypted libraries"
expect "feat on apps/mobile triggers minor" "mobile-v0.3.0" "$("$next_ver" HEAD)"

# Notes list only mobile-path changes and settle reverts.
commit_file apps/mobile/app.go "feat(mobile): temporary banner"
commit_file apps/mobile/app.go "revert: feat(mobile): temporary banner"
commit_file apps/mobile/app.go "perf(mobile): cache library icons"
want_notes=$(
  cat <<'END'
### Features

- Unlock encrypted libraries

### Fixes

- Retry dropped connection

### Performance

- Cache library icons
END
)
expect "phone notes" "$want_notes" "$("$notes_sh" mobile-v0.3.0 HEAD)"

# Breaking change on 0.x stays minor; on 1.x becomes major.
commit_file apps/mobile/app.go "feat(mobile)!: change storage format"
expect "breaking on 0.x bumps minor" "mobile-v0.3.0" "$("$next_ver" HEAD)"

git tag -a mobile-v1.0.0 -m mobile-v1.0.0
commit_file apps/mobile/app.go "fix(mobile): keep session" "BREAKING CHANGE: drops v1 sync"
expect "breaking on 1.x bumps major" "mobile-v2.0.0" "$("$next_ver" HEAD)"

# An older commit before the newest mobile-v* tag releases nothing.
expect "commit before newest tag" "" "$("$next_ver" HEAD~2)"

# set-phone-version.sh updates apps/mobile/nsgo.yaml and rejects non-versions.
"$set_ver" mobile-v2.0.0
expect "nsgo.yaml updated" "version: 2.0.0" "$(grep '^version:' apps/mobile/nsgo.yaml)"
if "$set_ver" mobile-v2.0 2>/dev/null; then
  fail "set-phone-version.sh accepted 2.0"
fi

# Maintenance-only notes fallback.
git tag -a mobile-v2.0.0 -m mobile-v2.0.0
commit_file apps/mobile/app.go "chore(mobile): bump build"
expect "maintenance notes" "No changes to the phone app worth listing: this version only carries maintenance." "$("$notes_sh" mobile-v2.0.1 HEAD)"

echo "phone-release-test: ok"
