module kando/apps/kando

go 1.27.4

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/aws/aws-sdk-go-v2 v1.47.8
	github.com/aws/aws-sdk-go-v2/credentials v1.20.16
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.9
	github.com/egoist/mygo v0.8.3
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/yldm-tech/ai-logo/mygo v1.18.1
	github.com/yldm-tech/mygo-breeze-ui v0.20.2
	github.com/yldm-tech/nodra v1.2.0
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
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.26 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.11 // indirect
	github.com/aws/smithy-go v1.28.8 // indirect
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)
