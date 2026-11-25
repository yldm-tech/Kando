module kando/packages/remotefs

go 1.27.4

require (
	github.com/cloudsoda/go-smb2 v0.0.0-20261105190815-95be04a983b8
	github.com/pkg/sftp v1.13.12
	github.com/studio-b12/gowebdav v0.13.0
	golang.org/x/crypto v0.59.0
	golang.org/x/net v0.63.0
	kando/packages/sync v0.0.0
)

require (
	github.com/cloudsoda/sddl v0.0.0-20261105185302-1c8f96799263 // indirect
	github.com/geoffgarside/ber v1.1.0 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/goidentity/v6 v6.0.1 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/kr/fs v0.1.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

replace kando/packages/sync => ../sync
