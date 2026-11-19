#!/usr/bin/env bash
# official-plugins.sh [plugin folder ...]
#
# Signs Kando's official script plugins and makes the .kandoplugin files Kando installs (docs/plugin-system.md 15, P3c), on the owner's computer: never in CI, which holds no plugin key. For each plugin folder, every folder of apps/kando/plugins/official with an nsgo.yaml when none is named, it compiles and signs plugin.tsx with go tool nsgo publish, under the official release key in NSGO_RELEASE_KEY and its certificate, the file KANDO_OFFICIAL_CERT names (kando-official.cert, which the plugin root certified for ai.yldm.kando.plugin.kando.* and which is kept outside the repository); checks that the package chains to Kando's plugin root and is the plugin its kando-plugin.json says; and writes <id>-<version>.kandoplugin into apps/site/public/plugin/packs (or KANDO_PLUGIN_PACKS), next to the other packs `kando plugin index` lists. A file already there is never replaced: publish a new version instead, as a Kando that cached the index checks the file it lists by its SHA-256.
#
#   NSGO_RELEASE_KEY=<release secret> KANDO_OFFICIAL_CERT=~/keys/kando-official.cert .github/scripts/official-plugins.sh
set -euo pipefail

fail() {
  echo "official-plugins: $1" >&2
  exit 1
}

root=$(git rev-parse --show-toplevel)
[ -n "${NSGO_RELEASE_KEY:-}" ] || fail "NSGO_RELEASE_KEY is not set: give the official release key's secret (go tool nsgo keygen) in it"
[ -n "${KANDO_OFFICIAL_CERT:-}" ] || fail "KANDO_OFFICIAL_CERT is not set: name the release key's certificate file, kando-official.cert"
[ -f "$KANDO_OFFICIAL_CERT" ] || fail "$KANDO_OFFICIAL_CERT is not a file"
cert=$(cd "$(dirname "$KANDO_OFFICIAL_CERT")" && pwd)/$(basename "$KANDO_OFFICIAL_CERT")
case $cert in
  "$root"/*) fail "the certificate is inside the repository; keep it outside, with the keys" ;;
esac
packs=${KANDO_PLUGIN_PACKS:-$root/apps/site/public/plugin/packs}

folders=("$@")
if [ ${#folders[@]} -eq 0 ]; then
  for f in "$root"/apps/kando/plugins/official/*/nsgo.yaml; do
    folders+=("$(dirname "$f")")
  done
fi
[ ${#folders[@]} -gt 0 ] || fail "no plugin folder to sign"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cd "$root"
for dir in "${folders[@]}"; do
  dir=$(cd "$dir" && pwd)
  [ -f "$dir/nsgo.yaml" ] && [ -f "$dir/kando-plugin.json" ] && [ -f "$dir/plugin.tsx" ] || fail "$dir is not an official plugin's folder (nsgo.yaml, kando-plugin.json, plugin.tsx)"
  name=$(basename "$dir")
  # The package's file name is what kando-plugin.json's script names in the .kandoplugin.
  package=$work/$name.nsgopkg
  go tool nsgo publish -cert "$cert" -entry plugin.tsx -out "$package" "$dir"
  go run ./apps/kando/plugins/official/cmd/official wrap -src "$dir" -package "$package" -dir "$packs"
done
echo "official-plugins: done. Check each file with kando plugin validate, list them with kando plugin index, and commit them."
