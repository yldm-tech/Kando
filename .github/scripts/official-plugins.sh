#!/usr/bin/env bash
# official-plugins.sh [plugin folder ...]
#
# Signs Kando's official script plugins and makes the .kandoplugin files Kando installs (docs/plugin-system.md 15, P3c), on the owner's computer: never in CI, which holds no plugin key. For each plugin folder, every folder of apps/kando/plugins/official with an nsgo.yaml when none is named, it compiles and signs plugin.tsx with go tool nsgo publish, under the official release key in NSGO_RELEASE_KEY and its certificate, the file KANDO_OFFICIAL_CERT names (kando-official.cert, which the plugin root certified for ai.yldm.kando.plugin.kando.* and which is kept outside the repository); checks that the package chains to Kando's plugin root and is the plugin its kando-plugin.json says; and lays it out as the community plugin repository, yldm-tech/kando-plugins, takes a script plugin: scripts/<id>/<id>-<version>.kandoplugin and the source it was signed from in scripts/<id>/src/ (the folder's files but its Go tests), which that repository's CI checks against each other. The layout goes into KANDO_PLUGINS_OUT, a new temporary folder when it is not set; KANDO_PLUGINS_OUT=<kando-plugins checkout> writes it into the checkout directly. A file already there is never replaced: publish a new version instead, as a Kando that cached the index checks the file it lists by its SHA-256. Publishing is then a pull request to kando-plugins with scripts/<id>/, which its Publish workflow signs into the index on merge.
#
#   NSGO_RELEASE_KEY=<release secret> KANDO_OFFICIAL_CERT=~/keys/kando-official.cert KANDO_PLUGINS_OUT=~/src/kando-plugins .github/scripts/official-plugins.sh
set -euo pipefail

fail() {
  echo "official-plugins: $1" >&2
  exit 1
}

root=$(git rev-parse --show-toplevel)
root=$(cd "$root" && pwd -P)

# outside fails when the folder $1 is in this repository: the packs are published from kando-plugins.
outside() {
  case $1/ in
    "$root"/*) fail "$1 is inside this repository; the packs are published from kando-plugins, so give a folder outside it" ;;
  esac
}
[ -n "${NSGO_RELEASE_KEY:-}" ] || fail "NSGO_RELEASE_KEY is not set: give the official release key's secret (go tool nsgo keygen) in it"
[ -n "${KANDO_OFFICIAL_CERT:-}" ] || fail "KANDO_OFFICIAL_CERT is not set: name the release key's certificate file, kando-official.cert"
[ -f "$KANDO_OFFICIAL_CERT" ] || fail "$KANDO_OFFICIAL_CERT is not a file"
cert=$(cd "$(dirname "$KANDO_OFFICIAL_CERT")" && pwd -P)/$(basename "$KANDO_OFFICIAL_CERT")
case $cert in
  "$root"/*) fail "the certificate is inside the repository; keep it outside, with the keys" ;;
esac
if [ -n "${KANDO_PLUGINS_OUT:-}" ]; then
  # Paths given relative to where the script was run from, made absolute before it moves to the repository's top.
  case $KANDO_PLUGINS_OUT in
    /*) out=$KANDO_PLUGINS_OUT ;;
    *) out=$PWD/$KANDO_PLUGINS_OUT ;;
  esac
  # The folder is checked with .. and symbolic links resolved, which the path as given may hide: before it is made, as far as it exists, and once more after.
  existing=$out
  rest=
  while [ ! -d "$existing" ]; do
    rest=/$(basename "$existing")$rest
    existing=$(dirname "$existing")
  done
  outside "$(cd "$existing" && pwd -P)$rest"
  mkdir -p "$out"
  out=$(cd "$out" && pwd -P)
  outside "$out"
else
  tmp=${TMPDIR:-/tmp}
  out=$(mktemp -d "${tmp%/}/kando-official-plugins.XXXXXX")
fi

folders=()
for dir in "$@"; do
  [ -d "$dir" ] || fail "$dir is not a folder"
  folders+=("$(cd "$dir" && pwd)")
done
if [ ${#folders[@]} -eq 0 ]; then
  for f in "$root"/apps/kando/plugins/official/*/nsgo.yaml; do
    folders+=("$(dirname "$f")")
  done
fi
[ ${#folders[@]} -gt 0 ] || fail "no plugin folder to sign"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cd "$root"
written=()
for dir in "${folders[@]}"; do
  [ -f "$dir/nsgo.yaml" ] && [ -f "$dir/kando-plugin.json" ] && [ -f "$dir/plugin.tsx" ] || fail "$dir is not an official plugin's folder (nsgo.yaml, kando-plugin.json, plugin.tsx)"
  name=$(basename "$dir")
  # The package's file name is what kando-plugin.json's script names in the .kandoplugin.
  package=$work/$name.nsgopkg
  # TODO(nsgo#924): nsgo publish cannot sign pkg.Contents.Extra, so the package does not sign its manifest and plugin.tsx must say it (kandoManifest); once it can, sign signature.SignedManifest for a manifest signature.SignsManifest takes, as script.PackFolder will, and keep kandoManifest in plugin.tsx until the manifest's oldest Kando compares the signed entry (signature.ManifestEntrySince). https://github.com/nsgo-dev/nsgo/issues/924
  go tool nsgo publish -cert "$cert" -entry plugin.tsx -out "$package" "$dir"
  # wrap writes into a folder of its own and prints the file's path; the file is <id>-<version>.kandoplugin, and wrap has checked the id against the signed package.
  file=$(go run ./apps/kando/plugins/official/cmd/official wrap -src "$dir" -package "$package" -dir "$work/$name.wrapped")
  file_name=$(basename "$file")
  id=$(awk -F'"' '$2 == "id" { print $4; exit }' "$dir/kando-plugin.json")
  case $file_name in
    "$id"-*.kandoplugin) ;;
    *) fail "$dir/kando-plugin.json's first id, \"$id\", is not the plugin $file_name holds" ;;
  esac
  dest=$out/scripts/$id
  [ ! -e "$dest/$file_name" ] || fail "$dest/$file_name is there already: publish a new version of the plugin rather than replacing it"
  mkdir -p "$dest"
  cp "$file" "$dest/$file_name"
  # src/ is the source of this version alone, so a file the plugin no longer has leaves it too.
  rm -rf "$dest/src"
  (cd "$dir" && find . -type f ! -name '*.go' ! -name '.*') | while IFS= read -r f; do
    mkdir -p "$dest/src/$(dirname "$f")"
    cp "$dir/$f" "$dest/src/$f"
  done
  written+=("$dest/$file_name")
  for other in "$dest"/*.kandoplugin; do
    [ "$other" = "$dest/$file_name" ] || echo "official-plugins: remove $other in the same pull request: kando-plugins lists the one .kandoplugin in scripts/$id/, and its Publish keeps serving the older file for a week" >&2
  done
done
echo "official-plugins: wrote, in $out:"
for f in "${written[@]}"; do
  echo "  ${f#"$out"/} (and its src/)"
done
echo "official-plugins: check each with kando plugin validate (source verified), copy scripts/<id>/ into a kando-plugins checkout unless $out is one, and open a pull request there; its Publish signs the index on merge."
