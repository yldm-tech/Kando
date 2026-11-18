module kando/apps/kando

go 1.27.4

tool github.com/nsgo-dev/nsgo/cmd/nsgo

require (
	github.com/aws/aws-sdk-go-v2 v1.48.0
	github.com/aws/aws-sdk-go-v2/credentials v1.21.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.10
	github.com/aws/smithy-go v1.28.9
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/nsgo-dev/nsgo v0.1.1-0.20261116154601-80d94246b875
	github.com/yldm-tech/ai-logo/nsgo v1.22.0
	github.com/yldm-tech/nodra v1.6.0
	go.yaml.in/yaml/v3 v3.0.6
	golang.org/x/image v0.48.0
	golang.org/x/net v0.62.0
	golang.org/x/oauth2 v0.38.0
	golang.org/x/sys v0.50.0
	golang.org/x/text v0.44.0
	kando/packages/core v0.0.0
	kando/packages/media v0.0.0
	kando/packages/study v0.0.0
	kando/packages/sync v0.0.0
	rsc.io/qr v0.2.0
)

replace (
	kando/packages/core => ../../packages/core
	kando/packages/media => ../../packages/media
	kando/packages/study => ../../packages/study
	kando/packages/sync => ../../packages/sync
)

require (
	github.com/andybalholm/brotli v1.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.27 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.12 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.12 // indirect
	github.com/dop251/goja v0.0.0-20261027234916-5f913d1693d9 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/egoist/mygo v0.9.0 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)
