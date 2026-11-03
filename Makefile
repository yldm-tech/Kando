.PHONY: test dev build

test:
	go test ./packages/core/...
	go test ./packages/media/...
	go test ./apps/lingocut/...

dev:
	tools/mygo-dev/dev.sh apps/lingocut

build:
	go tool mygo build apps/lingocut
