.PHONY: test dev build

# The NSGo target of this machine, such as macos_arm64 or linux_x64, which make build builds; TARGET=windows_x64 or a list (TARGET=macos_arm64,macos_x64) builds others.
TARGET ?= $(subst darwin,macos,$(shell go env GOOS))_$(subst amd64,x64,$(shell go env GOARCH))

test:
	go test ./packages/core/...
	go test ./packages/media/...
	go test ./apps/kando/...

dev:
	go tool nsgo dev apps/kando

# A release build leaves nodra's MyGo helpers out of the app (apps/kando/nsgo.yaml says why), as the Package workflow does.
build:
	GOFLAGS=-tags=nodra_nomygo go tool nsgo build -target $(TARGET) apps/kando
