module lingocut/apps/lingocut

go 1.27.3

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/egoist/mygo v0.7.4
	github.com/oneo-me/onevra/source v0.2026.17
	github.com/yldm-tech/nodra v1.1.0
	lingocut/packages/core v0.0.0
	lingocut/packages/media v0.0.0
)

replace (
	lingocut/packages/core => ../../packages/core
	lingocut/packages/media => ../../packages/media
)

require (
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)
