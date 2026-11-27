#!/usr/bin/env bash
# phone-changed-test.sh
#
# Checks phone-changed.sh and set-version.sh on a temporary git repository: desktop-only changes and non-releasable mobile commits report false, releasable commits on apps/mobile or its imported workspace packages report true, and set-version.sh keeps apps/kando/nsgo.yaml, manifest.json and apps/mobile/nsgo.yaml in sync.
set -euo pipefail

scripts_dir=$(cd "$(dirname "$0")" && pwd)
phone_changed=$scripts_dir/phone-changed.sh
set_ver=$scripts_dir/set-version.sh

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

git init -q
git config user.name test
git config user.email test@example.com
git config commit.gpgsign false
git config tag.gpgsign false

mkdir -p apps/kando/resources/browser-extension apps/mobile packages/sync

cat >apps/kando/nsgo.yaml <<'EOF'
name: Kando
version: 0.75.0
EOF

cat >apps/kando/resources/browser-extension/manifest.json <<'EOF'
{
  "name": "Kando",
  "version": "0.75.0",
  "manifest_version": 3
}
EOF

cat >apps/mobile/nsgo.yaml <<'EOF'
name: Kando
id: ai.yldm.kando.mobile
version: 0.2.0
EOF

expect() {
  if [ "$2" != "$3" ]; then
    printf 'phone-changed-test: %s\n--- want\n%s\n--- got\n%s\n' "$1" "$2" "$3" >&2
    exit 1
  fi
}

commit_file() {
  local path=$1 subject=$2 body=${3:-}
  printf '%s\n' "$subject" >>"$path"
  git add "$path"
  git commit -q -m "$subject" ${body:+-m "$body"}
}

git add apps/kando/nsgo.yaml apps/kando/resources/browser-extension/manifest.json apps/mobile/nsgo.yaml
git commit -q -m "chore: init"
git tag -a v0.75.0 -m v0.75.0

expect "nothing since tag" "false" "$("$phone_changed" HEAD)"

commit_file apps/kando/main.go "feat(films): desktop-only change"
expect "desktop-only feat" "false" "$("$phone_changed" HEAD)"

commit_file apps/mobile/app.go "docs(mobile): clarify comment"
commit_file apps/mobile/app.go "chore(mobile): tidy"
expect "non-releasable mobile commits" "false" "$("$phone_changed" HEAD)"

commit_file packages/sync/client.go "fix(sync): retry range request"
expect "fix on packages/sync" "true" "$("$phone_changed" HEAD)"

git tag -a v0.75.1 -m v0.75.1
expect "clean after new tag" "false" "$("$phone_changed" HEAD)"

commit_file apps/mobile/app.go "feat(mobile): stream computer videos"
expect "feat on apps/mobile" "true" "$("$phone_changed" HEAD)"

"$set_ver" v0.76.0
expect "desktop nsgo.yaml" "version: 0.76.0" "$(grep '^version:' apps/kando/nsgo.yaml)"
expect "extension manifest" '  "version": "0.76.0",' "$(grep '"version":' apps/kando/resources/browser-extension/manifest.json)"
expect "mobile nsgo.yaml" "version: 0.76.0" "$(grep '^version:' apps/mobile/nsgo.yaml)"

echo "phone-changed-test: ok"
