#!/usr/bin/env bash
# browser-extension.sh <out-dir>
#
# Checks Kando's browser extension (browser-extension-check.mjs) and zips it for each browser family into out-dir: kando-browser-extension-chromium-<version>.zip, whose manifest leaves out Firefox's settings and background scripts, and kando-browser-extension-firefox-<version>.zip, whose manifest leaves out Chromium's key and service worker. The folder itself, which Kando ships and both families load unpacked, keeps both. It needs Node.js, jq and zip.
set -euo pipefail

out=$(cd "$1" && pwd)
root=$(git rev-parse --show-toplevel)
src=$root/apps/kando/resources/browser-extension

node "$root/.github/scripts/browser-extension-check.mjs" "$src"
version=$(jq -r .version "$src/manifest.json")

for family in chromium firefox; do
  dir=$(mktemp -d)
  cp -R "$src/." "$dir/"
  case $family in
    chromium) filter='del(.browser_specific_settings) | .background |= del(.scripts)' ;;
    firefox) filter='del(.key) | .background |= del(.service_worker)' ;;
  esac
  jq "$filter" "$src/manifest.json" >"$dir/manifest.json"
  zip="$out/kando-browser-extension-$family-$version.zip"
  rm -f "$zip"
  (cd "$dir" && zip -qrX "$zip" .)
  rm -rf "$dir"
  echo "$zip"
done
