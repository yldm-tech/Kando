package main

import (
	"os"
	"strings"
)

// The account page's reads and writes (account.App). Signing in and out with Google lives in cloud_app.go.

func (a *app) GoogleEmail() string { return a.googleEmail }

func (a *app) GoogleBusy() bool { return a.googleBusy }

func (a *app) GoogleErr() string { return a.googleErr }

func (a *app) GoogleConfigured() bool {
	return strings.TrimSpace(os.Getenv("LINGOCUT_GOOGLE_CLIENT_ID")) != ""
}

func (a *app) Storage() *string { return &a.storage }

func (a *app) StorageKind() string { return a.storageKind() }

func (a *app) R2Endpoint() *string { return &a.r2Endpoint }

func (a *app) R2Bucket() *string { return &a.r2Bucket }

func (a *app) R2AccessKey() *string { return &a.r2AccessKey }

func (a *app) R2Secret() *string { return &a.r2Secret }

func (a *app) S3Region() *string { return &a.s3Region }

func (a *app) S3Bucket() *string { return &a.s3Bucket }

func (a *app) S3AccessKey() *string { return &a.s3AccessKey }

func (a *app) S3Secret() *string { return &a.s3Secret }

func (a *app) OSSRegion() *string { return &a.ossRegion }

func (a *app) OSSBucket() *string { return &a.ossBucket }

func (a *app) OSSAccessKey() *string { return &a.ossAccessKey }

func (a *app) OSSSecret() *string { return &a.ossSecret }

func (a *app) COSRegion() *string { return &a.cosRegion }

func (a *app) COSBucket() *string { return &a.cosBucket }

func (a *app) COSAccessKey() *string { return &a.cosAccessKey }

func (a *app) COSSecret() *string { return &a.cosSecret }

func (a *app) CompatEndpoint() *string { return &a.compatEndpoint }

func (a *app) CompatRegion() *string { return &a.compatRegion }

func (a *app) CompatBucket() *string { return &a.compatBucket }

func (a *app) CompatAccessKey() *string { return &a.compatAccessKey }

func (a *app) CompatSecret() *string { return &a.compatSecret }

func (a *app) CompatPathStyle() *bool { return &a.compatPath }

func (a *app) ShowR2Secret() bool { return a.showR2Secret }

func (a *app) ToggleR2Secret() { a.showR2Secret = !a.showR2Secret }
