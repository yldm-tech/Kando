.PHONY: test dev build official-plugins plugin-revocations

# The NSGo target of this machine, such as macos_arm64 or linux_x64, which make build builds; TARGET=windows_x64 or a list (TARGET=macos_arm64,macos_x64) builds others.
TARGET ?= $(subst darwin,macos,$(shell go env GOOS))_$(subst amd64,x64,$(shell go env GOARCH))

test:
	go test ./packages/core/...
	go test ./packages/media/...
	go test ./packages/remotefs/...
	go test ./packages/study/...
	go test ./packages/sync/...
	go test ./packages/track/...
	go test ./apps/kando/...
	go test ./apps/mobile/...

dev:
	go tool nsgo dev apps/kando

build:
	go tool nsgo build -target $(TARGET) apps/kando

# Kando's official script plugins, signed with the official release key and laid out as the community plugin repository, yldm-tech/kando-plugins, takes them, scripts/<id>/<id>-<version>.kandoplugin with the source in scripts/<id>/src/ (.github/scripts/official-plugins.sh): NSGO_RELEASE_KEY holds the release key's secret and KANDO_OFFICIAL_CERT names its certificate, kept outside the repository. KANDO_PLUGINS_OUT names where the layout goes, a kando-plugins checkout to write into it directly, a new temporary folder when empty; the plugins are then published by a pull request to kando-plugins. PLUGINS names plugin folders, every one under apps/kando/plugins/official when empty.
official-plugins:
	.github/scripts/official-plugins.sh $(PLUGINS)

# The next revocation list of the plugin root, signed with NSGO_ROOT_KEY on the computer that keeps it and written to apps/kando/plugins/revocations (.github/scripts/plugin-revocations.sh), which every release then publishes: REVOKE holds what to revoke, such as -keys <key ID> or -packages <sha256>, and nothing to only renew the list before it expires.
plugin-revocations:
	.github/scripts/plugin-revocations.sh $(REVOKE)
