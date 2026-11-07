#!/usr/bin/env bash
# label-pr.sh <pr-number>
#
# Gives a pull request the type label its Conventional Commit title calls for (feat is feature, fix is bug, a type ending in "!" is also breaking, ...) and the size label of its changed lines, taking away the type and size labels it no longer fits. A title that is not a Conventional Commit fails, and so does a commit whose subject is not one (merge commits aside): the Release workflow reads versions from the commits that reach main, which are the pull request's own commits when it is merged, and its title or its one commit's subject when it is squashed.
set -euo pipefail

pr=$1
title=$(gh pr view "$pr" --json title --jq .title)

type_re='^([a-z]+)(\([^)]*\))?(!?): .+'
if [[ ! $title =~ $type_re ]]; then
  echo "::error::The title \"$title\" is not a Conventional Commit, such as \"feat(films): add a sort menu\" or \"fix: ...\"."
  exit 1
fi
type=${BASH_REMATCH[1]}
breaking=${BASH_REMATCH[3]}

case $type in
  feat) want=feature ;;
  fix) want=bug ;;
  perf) want=performance ;;
  refactor) want=refactor ;;
  docs) want=documentation ;;
  test) want=tests ;;
  ci) want=ci ;;
  build | chore | style | revert) want=chore ;;
  *)
    echo "::error::\"$type\" is not a Conventional Commit type: use feat, fix, perf, refactor, docs, test, ci, build, chore, style or revert."
    exit 1
    ;;
esac

changed=$(gh pr view "$pr" --json additions,deletions --jq '.additions + .deletions')
if [ "$changed" -lt 10 ]; then
  size=size/XS
elif [ "$changed" -lt 100 ]; then
  size=size/S
elif [ "$changed" -lt 500 ]; then
  size=size/M
elif [ "$changed" -lt 1000 ]; then
  size=size/L
else
  size=size/XL
fi

wanted=("$want" "$size")
if [ "$breaking" = "!" ]; then
  wanted+=(breaking)
fi

managed=(feature bug performance refactor documentation tests ci chore breaking size/XS size/S size/M size/L size/XL)
mapfile -t have < <(gh pr view "$pr" --json labels --jq '.labels[].name')

# contains <word> <list...> reports whether the list holds the word.
contains() {
  local word=$1
  shift
  for item in "$@"; do
    if [ "$item" = "$word" ]; then
      return 0
    fi
  done
  return 1
}

args=()
for l in "${wanted[@]}"; do
  if ! contains "$l" "${have[@]}"; then
    args+=(--add-label "$l")
  fi
done
for l in "${managed[@]}"; do
  if contains "$l" "${have[@]}" && ! contains "$l" "${wanted[@]}"; then
    args+=(--remove-label "$l")
  fi
done
if [ "${#args[@]}" -gt 0 ]; then
  gh pr edit "$pr" "${args[@]}"
fi
echo "#$pr: ${wanted[*]}"

types='feat|fix|perf|refactor|docs|test|ci|build|chore|style|revert'
mapfile -t bad < <(gh api --paginate "repos/{owner}/{repo}/pulls/$pr/commits" --jq '.[] | select((.parents | length) == 1) | .commit.message | split("\n")[0]' | grep -vE "^($types)(\([^)]*\))?!?: .+" || true)
if [ "${#bad[@]}" -gt 0 ]; then
  echo "::error::These commits are not Conventional Commits, which the Release workflow reads versions from once they reach main: reword them, for example with git rebase -i, or squash-merge the pull request."
  printf '  %s\n' "${bad[@]}"
  exit 1
fi
