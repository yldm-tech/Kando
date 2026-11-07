#!/usr/bin/env bash
# label-conflicts.sh [pr-number...]
#
# Gives the open pull requests named, or every open one, the "conflicts" label while they conflict with their base branch, with one comment when they start to, and takes the label away once they merge cleanly. GitHub works out whether a pull request merges cleanly in the background after a push, so its state is asked again a few times while it reads UNKNOWN.
set -euo pipefail

label=conflicts

if [ "$#" -gt 0 ]; then
  prs=("$@")
else
  mapfile -t prs < <(gh pr list --state open --limit 200 --json number --jq '.[].number')
fi

for pr in "${prs[@]}"; do
  state=UNKNOWN
  for _ in 1 2 3 4 5 6; do
    state=$(gh pr view "$pr" --json mergeable --jq .mergeable)
    if [ "$state" != UNKNOWN ]; then
      break
    fi
    sleep 10
  done
  labelled=$(gh pr view "$pr" --json labels --jq "[.labels[].name] | index(\"$label\") != null")
  case $state in
    CONFLICTING)
      if [ "$labelled" = false ]; then
        gh pr edit "$pr" --add-label "$label"
        base=$(gh pr view "$pr" --json baseRefName --jq .baseRefName)
        gh pr comment "$pr" --body "This pull request conflicts with \`$base\`. Merge \`$base\` into it or rebase it onto \`$base\`, then push; the \`$label\` label goes away once it merges cleanly."
      fi
      echo "#$pr conflicts"
      ;;
    MERGEABLE)
      if [ "$labelled" = true ]; then
        gh pr edit "$pr" --remove-label "$label"
      fi
      echo "#$pr merges cleanly"
      ;;
    *)
      echo "#$pr: GitHub has not worked out whether it merges cleanly yet"
      ;;
  esac
done
