.PHONY: test dev build

test:
	go test ./packages/core/...
	go test ./packages/media/...
	go test ./apps/lingocut/...

dev:
	go tool mygo dev -C apps/lingocut

build:
	go tool mygo build -C apps/lingocut
