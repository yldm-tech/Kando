module kando/apps/kando

go 1.27.4

tool github.com/nsgo-dev/nsgo/cmd/nsgo

require (
	github.com/aws/aws-sdk-go-v2 v1.48.0
	github.com/aws/aws-sdk-go-v2/credentials v1.21.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.10
	github.com/aws/smithy-go v1.28.9
	github.com/cloudsoda/go-smb2 v0.0.0-20261105190815-95be04a983b8
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/nsgo-dev/nsgo v0.2.1-0.20261121221132-091984681603
	github.com/pkg/sftp v1.13.12
	github.com/studio-b12/gowebdav v0.13.0
	github.com/yldm-tech/ai-logo/nsgo v1.22.0
	github.com/yldm-tech/nodra v1.6.0
	go.yaml.in/yaml/v3 v3.0.6
	golang.org/x/crypto v0.59.0
	golang.org/x/image v0.48.0
	golang.org/x/net v0.62.0
	golang.org/x/oauth2 v0.38.0
	golang.org/x/sys v0.50.0
	golang.org/x/text v0.44.0
	kando/packages/core v0.0.0
	kando/packages/media v0.0.0
	kando/packages/study v0.0.0
	kando/packages/sync v0.0.0
	kando/packages/track v0.0.0
	rsc.io/qr v0.2.0
)

replace (
	kando/packages/core => ../../packages/core
	kando/packages/media => ../../packages/media
	kando/packages/study => ../../packages/study
	kando/packages/sync => ../../packages/sync
	kando/packages/track => ../../packages/track
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
	github.com/cloudsoda/sddl v0.0.0-20261105185302-1c8f96799263 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dlclark/regexp2/v2 v2.11.0 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/egoist/mygo v0.9.0 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/go-ego/gse v1.3.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/gofrs/flock v0.13.1 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/licenseclassifier/v2 v2.0.0 // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/ikawaha/kagome-dict v1.1.7 // indirect
	github.com/ikawaha/kagome-dict/ipa v1.2.6 // indirect
	github.com/ikawaha/kagome/v2 v2.11.1 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/makiuchi-d/gozxing v0.1.1 // indirect
	github.com/nsgo-dev/goja v0.1.0-nsgo.2.0.20261121173409-f05dab9a4261 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/vcaesar/cedar v0.50.1 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)
