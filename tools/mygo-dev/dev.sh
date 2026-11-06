#!/bin/sh
# Runs `mygo dev` with reloads that keep the running window until the new build shows its own window, so a reload does not look like the app quitting. Stock `mygo dev` quits the running build first (egoist/mygo#22, for the single-instance lock, web view profiles and app files, none of which Kando shares between builds beyond its library files). Only macOS probes windows; elsewhere reloads keep the stock order.
# It copies MyGo's module at the version go.mod pins, applies dev.go.patch, adds the overlap files and builds the CLI into .mygo/bin. When that fails, as when the patch no longer applies after an upgrade, it warns and runs the stock CLI.
set -eu

root=$(cd "$(dirname "$0")/../.." && pwd)
here="$root/tools/mygo-dev"
work="$root/.mygo/mygo-dev"
bin="$root/.mygo/bin/mygo-dev"

cd "$root"

build() {
	version=$(go list -m github.com/egoist/mygo) || return 1
	stamp=$(cat "$here/dev.go.patch" "$here"/overlap*.go | shasum | cut -d' ' -f1)
	if [ -x "$bin" ] && [ "$(cat "$work/stamp" 2>/dev/null)" = "$version $stamp" ]; then
		return 0
	fi
	src=$(go list -m -f '{{.Dir}}' github.com/egoist/mygo) || return 1
	rm -rf "$work"
	mkdir -p "$work" "$(dirname "$bin")"
	cp -R "$src" "$work/src"
	chmod -R u+w "$work/src"
	patch --quiet --forward --no-backup-if-mismatch "$work/src/cmd/mygo/dev.go" "$here/dev.go.patch" || return 1
	cp "$here/overlap.go" "$work/src/cmd/mygo/kando_overlap.go"
	cp "$here/overlap_darwin.go" "$work/src/cmd/mygo/kando_overlap_darwin.go"
	cp "$here/overlap_other.go" "$work/src/cmd/mygo/kando_overlap_other.go"
	(cd "$work/src" && GOWORK=off go build -o "$bin" ./cmd/mygo) || return 1
	echo "$version $stamp" >"$work/stamp"
}

if build; then
	exec "$bin" dev "$@"
fi
echo "mygo-dev: could not build the patched CLI for $(go list -m github.com/egoist/mygo) (see above); running the stock mygo dev, which closes the window on every reload" >&2
exec go tool mygo dev "$@"
