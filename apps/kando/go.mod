module kando/apps/kando

go 1.27.4

tool (
	github.com/egoist/mygo/cmd/mygo
	github.com/nsgo-dev/nsgo/cmd/nsgo
)

require (
	github.com/aws/aws-sdk-go-v2 v1.47.8
	github.com/aws/aws-sdk-go-v2/credentials v1.20.16
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.9
	github.com/egoist/mygo v0.9.0
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/nsgo-dev/nsgo v0.0.0-20261114081444-3f45b10448ad
	github.com/yldm-tech/ai-logo/nsgo v1.21.0
	github.com/yldm-tech/mygo-breeze-ui v0.20.2
	github.com/yldm-tech/nodra v1.5.0
	go.yaml.in/yaml/v3 v3.0.6
	golang.org/x/image v0.47.0
	golang.org/x/net v0.62.0
	golang.org/x/oauth2 v0.38.0
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.43.0
	kando/packages/core v0.0.0
	kando/packages/media v0.0.0
)

replace (
	kando/packages/core => ../../packages/core
	kando/packages/media => ../../packages/media
)

require (
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/andybalholm/brotli v1.2.6 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.26 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.11 // indirect
	github.com/aws/smithy-go v1.28.8 // indirect
	github.com/cloudflare/circl v1.6.2 // indirect
	github.com/dop251/goja v0.0.0-20261027234916-5f913d1693d9 // indirect
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.digitalxero.dev/rpm v0.3.4 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	rsc.io/qr v0.2.0 // indirect
)
