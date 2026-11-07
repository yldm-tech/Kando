module kando/apps/kando

go 1.27.3

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/aws/aws-sdk-go-v2 v1.47.5
	github.com/aws/aws-sdk-go-v2/credentials v1.20.13
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.6
	github.com/egoist/mygo v0.7.4
	github.com/yldm-tech/ai-logo/mygo v1.17.0
	github.com/yldm-tech/mygo-breeze-ui v0.12.0
	github.com/yldm-tech/nodra v1.1.0
	golang.org/x/oauth2 v0.37.0
	kando/packages/core v0.0.0
	kando/packages/media v0.0.0
)

replace (
	kando/packages/core => ../../packages/core
	kando/packages/media => ../../packages/media
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.8 // indirect
	github.com/aws/smithy-go v1.28.7 // indirect
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)
