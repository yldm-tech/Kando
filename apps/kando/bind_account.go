package main

import (
	"os"
	"strings"
)

// The settings window's 账号与上传 category's reads and writes (account.App). Signing in and out with Google lives in cloud_app.go.

func (a *app) GoogleEmail() string { return a.cloud.googleEmail }

func (a *app) GoogleBusy() bool { return a.cloud.googleBusy }

func (a *app) GoogleErr() string { return a.cloud.googleErr }

func (a *app) GoogleConfigured() bool {
	return strings.TrimSpace(os.Getenv("KANDO_GOOGLE_CLIENT_ID")) != ""
}

func (a *app) Storage() *string { return &a.cloud.storage }

func (a *app) StorageKind() string { return a.storageKind() }

func (a *app) R2Endpoint() *string { return &a.cloud.r2Endpoint }

func (a *app) R2Bucket() *string { return &a.cloud.r2Bucket }

func (a *app) R2AccessKey() *string { return &a.cloud.r2AccessKey }

func (a *app) R2Secret() *string { return &a.cloud.r2Secret }

func (a *app) S3Region() *string { return &a.cloud.s3Region }

func (a *app) S3Bucket() *string { return &a.cloud.s3Bucket }

func (a *app) S3AccessKey() *string { return &a.cloud.s3AccessKey }

func (a *app) S3Secret() *string { return &a.cloud.s3Secret }

func (a *app) OSSRegion() *string { return &a.cloud.ossRegion }

func (a *app) OSSBucket() *string { return &a.cloud.ossBucket }

func (a *app) OSSAccessKey() *string { return &a.cloud.ossAccessKey }

func (a *app) OSSSecret() *string { return &a.cloud.ossSecret }

func (a *app) COSRegion() *string { return &a.cloud.cosRegion }

func (a *app) COSBucket() *string { return &a.cloud.cosBucket }

func (a *app) COSAccessKey() *string { return &a.cloud.cosAccessKey }

func (a *app) COSSecret() *string { return &a.cloud.cosSecret }

func (a *app) CompatEndpoint() *string { return &a.cloud.compatEndpoint }

func (a *app) CompatRegion() *string { return &a.cloud.compatRegion }

func (a *app) CompatBucket() *string { return &a.cloud.compatBucket }

func (a *app) CompatAccessKey() *string { return &a.cloud.compatAccessKey }

func (a *app) CompatSecret() *string { return &a.cloud.compatSecret }

func (a *app) CompatPathStyle() *bool { return &a.cloud.compatPath }
