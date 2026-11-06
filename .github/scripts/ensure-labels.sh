#!/usr/bin/env bash
# ensure-labels.sh
#
# Creates the labels of .github/labels.tsv that the repository does not have yet, so that the workflows can give them; labels that exist keep their colors and descriptions.
set -euo pipefail

here=$(dirname "$0")
existing=$(gh label list --limit 500 --json name --jq '.[].name')

while IFS=$'\t' read -r name color description; do
  case $name in
    '' | '#'*) continue ;;
  esac
  if ! grep -qxF -- "$name" <<<"$existing"; then
    gh label create "$name" --color "$color" --description "$description"
  fi
done <"$here/../labels.tsv"
