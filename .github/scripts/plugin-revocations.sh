#!/usr/bin/env bash
# plugin-revocations.sh [-days N] [-keys <key ID,...>] [-packages <sha256 or .nsgopkg,...>]
#
# Signs the next revocation list of Kando's plugin root with go tool nsgo revoke, on the owner's computer, with the root's secret in NSGO_ROOT_KEY (never in CI), and writes it to apps/kando/plugins/revocations/kando-plugin-revocations.json, which the owner commits; every release then attaches that file as kando-plugin-revocations.json, where Kando fetches it once a day (script.RevocationsURL, docs/plugin-system.md 6.4, 15). The new list takes the committed one's sequence plus one (1 for the first) and revokes again everything the committed one did, as Kando keeps only a root's newest list, plus the release keys (-keys, by key ID) and packages (-packages, by SHA-256 or file) given. It expires in -days days, 180 unless given: sign a new one, with nothing to add, before then, as a release refuses an expired list and Kando, holding one, pauses the plugins of other publishers that reach the network or a credential.
#
#   NSGO_ROOT_KEY=<root secret> .github/scripts/plugin-revocations.sh -keys 0123456789abcdef
set -euo pipefail

fail() {
  echo "plugin-revocations: $1" >&2
  exit 1
}

days=180
add_keys=""
add_packages=""
while [ $# -gt 0 ]; do
  case $1 in
    -days) days=$2; shift 2 ;;
    -keys) add_keys=$2; shift 2 ;;
    -packages) add_packages=$2; shift 2 ;;
    *) fail "unknown argument $1; usage: plugin-revocations.sh [-days N] [-keys <key ID,...>] [-packages <sha256 or .nsgopkg,...>]" ;;
  esac
done
[[ $days =~ ^[1-9][0-9]*$ ]] || fail "-days is a number of days, 1 or more"
[ -n "${NSGO_ROOT_KEY:-}" ] || fail "NSGO_ROOT_KEY is not set: give the plugin root's secret in it, on the offline computer that keeps it"

# Packages named by file, relative to where the script was run from, made absolute before it moves to the repository's top.
if [ -n "$add_packages" ]; then
  IFS=, read -r -a named <<<"$add_packages"
  add_packages=""
  for p in "${named[@]}"; do
    if [ -f "$p" ]; then
      p=$(cd "$(dirname "$p")" && pwd)/$(basename "$p")
    fi
    add_packages=${add_packages:+$add_packages,}$p
  done
fi

root=$(git rev-parse --show-toplevel)
cd "$root"
list=apps/kando/plugins/revocations/kando-plugin-revocations.json
tool=(go run ./apps/kando/plugins/official/cmd/official)

# The committed list's next sequence and what it revokes, a line each.
next=$("${tool[@]}" revocations -list "$list" -next) || fail "could not read the committed list $list (see above)"
{
  read -r sequence
  read -r keys
  read -r packages
} <<<"$next"
[[ $sequence =~ ^[1-9][0-9]*$ ]] || fail "could not read the committed list's sequence"

join() {
  local a=$1 b=$2
  if [ -n "$a" ] && [ -n "$b" ]; then
    echo "$a,$b"
  else
    echo "$a$b"
  fi
}
keys=$(join "$keys" "$add_keys")
packages=$(join "$packages" "$add_packages")

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
go tool nsgo revoke -sequence "$sequence" -days "$days" -keys "$keys" -packages "$packages" -out "$work/list.json"
mkdir -p "$(dirname "$list")"
previous=()
if [ -f "$list" ]; then
  cp "$list" "$work/previous.json"
  previous=(-previous "$work/previous.json")
fi
# Checked as the release will, against the committed list it replaces, before it replaces it.
"${tool[@]}" revocations -list "$work/list.json" ${previous[@]+"${previous[@]}"}
mv "$work/list.json" "$list"
echo "plugin-revocations: wrote $list (list $sequence); commit it, and the next release publishes it."
