#!/usr/bin/env bash
# mygo-bridge-test.sh
#
# Checks mygo-bridge.go on packages of its own, made in a temporary folder as nsgo build lays them out: make writes a signed archive and manifest per target that verify accepts, the macOS archive holds the app bundle alone with its relative links, the Windows one only the installed files, verify refuses a manifest under another key, and make refuses a private key that is not the public key's and an app that links outside itself.
set -euo pipefail

script=$(cd "$(dirname "$0")" && pwd)/mygo-bridge.go
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
export GOWORK=off

fail() {
  echo "mygo-bridge-test: $1" >&2
  exit 1
}

# keygen prints a fresh Ed25519 key pair, base64: the 64-byte private key, then the public key.
mkdir "$work/keygen"
cat > "$work/keygen/main.go" <<'EOF'
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

func main() {
	pub, priv, _ := ed25519.GenerateKey(nil)
	fmt.Println(base64.StdEncoding.EncodeToString(priv))
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
}
EOF
keys() { (cd "$work/keygen" && go run main.go); }
read -r -d '' private public < <(keys; printf '\0') || true
read -r -d '' _ other < <(keys; printf '\0') || true

packages=$work/packages
app=$packages/Kando-1.2.3-macos_arm64/Kando.app
mkdir -p "$packages/releases" "$app/Contents/MacOS" "$app/Contents/Frameworks/Sparkle.framework/Versions/B"
printf 'binary' > "$app/Contents/MacOS/Kando"
chmod 755 "$app/Contents/MacOS/Kando"
ln -s B "$app/Contents/Frameworks/Sparkle.framework/Versions/Current"
win=$packages/Kando-1.2.3-windows_x64
mkdir -p "$win"
for file in Kando.exe WinSparkle.dll icon.png Kando.nsi Kando-setup.ico; do
  printf '%s' "$file" > "$win/$file"
done
printf 'Notes' > "$work/notes.md"

make_bridge() {
  go run "$script" make -version 1.2.3 -repo owner/app -packages "$packages" -targets macos_arm64,windows_x64 -notes "$work/notes.md" -public "$1" -date 2026-01-02T00:00:00Z
}

MYGO_UPDATER_PRIVATE_KEY=$private make_bridge "$public" > /dev/null
go run "$script" verify -dir "$packages/releases" -public "$public" > /dev/null || fail "verify refused what make wrote"

releases=$packages/releases
[ -f "$releases/update-darwin-arm64.json" ] && [ -f "$releases/update-windows-amd64.json" ] || fail "make wrote no manifest per target"
grep -q '"url": "https://github.com/owner/app/releases/download/v1.2.3/kando-1.2.3-darwin-arm64.tar.gz"' "$releases/update-darwin-arm64.json" || fail "the manifest names another archive URL"
grep -q '"notes": "Notes"' "$releases/update-darwin-arm64.json" || fail "the manifest lacks the notes"

listing=$(tar -tzvf "$releases/kando-1.2.3-darwin-arm64.tar.gz")
[ "$(tar -tzf "$releases/kando-1.2.3-darwin-arm64.tar.gz" | cut -d/ -f1 | sort -u)" = Kando.app ] || fail "the macOS archive holds more than Kando.app"
grep -q 'Versions/Current -> B' <<<"$listing" || fail "the macOS archive lost the framework's relative link"
grep -q '^-rwxr-xr-x.*Kando.app/Contents/MacOS/Kando$' <<<"$listing" || fail "the macOS archive lost the executable's mode"
[ "$(tar -tzf "$releases/kando-1.2.3-windows-amd64.tar.gz" | tr '\n' ' ')" = "Kando.exe WinSparkle.dll icon.png " ] || fail "the Windows archive holds other files than the installed ones"

if go run "$script" verify -dir "$releases" -public "$other" > /dev/null 2>&1; then
  fail "verify accepted a manifest under another key"
fi
if MYGO_UPDATER_PRIVATE_KEY=$private make_bridge "$other" > /dev/null 2>&1; then
  fail "make took a private key that is not the public key's"
fi
ln -s /etc "$app/Contents/Resources"
if MYGO_UPDATER_PRIVATE_KEY=$private make_bridge "$public" > /dev/null 2>&1; then
  fail "make archived a link outside the app"
fi
echo "mygo-bridge-test: ok"
