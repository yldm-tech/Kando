package main

import (
	"context"
	"maps"
	"strings"

	"kando/apps/kando/cloud"
)

// cloudUI is the state of the account page and of uploads: the Google account, the storage each kind of bucket is reached with, and the upload running now (cloud_app.go). googleErr and uploadErr say why the last sign-in or upload failed, or that it was canceled (uploadCanceled), as functions so the message reads in the language the page shows when it is drawn; nil when nothing failed. uploadCancel stops the upload running now (CancelUpload), nil when none runs. uploaded holds the object key of each film uploaded, by uploadMemo (the storage's kind and the video's path), and is saved in settings.json so a film still reads as uploaded after a restart.
type cloudUI struct {
	googleEmail     string
	googleRefresh   string
	googleBusy      bool
	googleErr       func() string
	googleLogin     func(context.Context, cloud.GoogleConfig) (cloud.Account, error)
	storage         string
	r2Endpoint      string
	r2Bucket        string
	r2AccessKey     string
	r2Secret        string
	s3Region        string
	s3Bucket        string
	s3AccessKey     string
	s3Secret        string
	ossRegion       string
	ossBucket       string
	ossAccessKey    string
	ossSecret       string
	cosRegion       string
	cosBucket       string
	cosAccessKey    string
	cosSecret       string
	compatEndpoint  string
	compatRegion    string
	compatBucket    string
	compatAccessKey string
	compatSecret    string
	compatPath      bool
	uploadPath      string
	uploadWritten   int64
	uploadTotal     int64
	uploadErr       func() string
	uploadErrPath   string
	uploadCanceled  bool
	uploadCancel    context.CancelFunc
	uploaded        map[string]string
	uploader        func(context.Context, cloud.Store, []cloud.File, func(written, total int64)) error
}

func newCloudUI() cloudUI { return cloudUI{} }

// savedCloud is the account's part of settings.json: the Google account, each kind of storage, and the films uploaded.
type savedCloud struct {
	GoogleEmail     string `json:"googleEmail,omitempty"`
	GoogleRefresh   string `json:"googleRefresh,omitempty"`
	Storage         string `json:"storage,omitempty"`
	R2Endpoint      string `json:"r2Endpoint,omitempty"`
	R2Bucket        string `json:"r2Bucket,omitempty"`
	R2AccessKey     string `json:"r2AccessKey,omitempty"`
	R2Secret        string `json:"r2Secret,omitempty"`
	S3Region        string `json:"s3Region,omitempty"`
	S3Bucket        string `json:"s3Bucket,omitempty"`
	S3AccessKey     string `json:"s3AccessKey,omitempty"`
	S3Secret        string `json:"s3Secret,omitempty"`
	OSSRegion       string `json:"ossRegion,omitempty"`
	OSSBucket       string `json:"ossBucket,omitempty"`
	OSSAccessKey    string `json:"ossAccessKey,omitempty"`
	OSSSecret       string `json:"ossSecret,omitempty"`
	COSRegion       string `json:"cosRegion,omitempty"`
	COSBucket       string `json:"cosBucket,omitempty"`
	COSAccessKey    string `json:"cosAccessKey,omitempty"`
	COSSecret       string `json:"cosSecret,omitempty"`
	CompatEndpoint  string `json:"s3CompatEndpoint,omitempty"`
	CompatRegion    string `json:"s3CompatRegion,omitempty"`
	CompatBucket    string `json:"s3CompatBucket,omitempty"`
	CompatAccessKey string `json:"s3CompatAccessKey,omitempty"`
	CompatSecret    string `json:"s3CompatSecret,omitempty"`
	CompatPathStyle bool   `json:"s3CompatPathStyle,omitempty"`

	// Uploaded is the object key of each film uploaded, by the storage's kind and the video's path (cloudUI.uploaded).
	Uploaded map[string]string `json:"uploaded,omitempty"`
}

func (s *savedCloud) clean() {
	s.GoogleEmail = strings.TrimSpace(s.GoogleEmail)
	s.GoogleRefresh = strings.TrimSpace(s.GoogleRefresh)
	s.Storage = strings.TrimSpace(s.Storage)
	s.R2Endpoint = strings.TrimSpace(s.R2Endpoint)
	s.R2Bucket = strings.TrimSpace(s.R2Bucket)
	s.R2AccessKey = strings.TrimSpace(s.R2AccessKey)
	s.R2Secret = strings.TrimSpace(s.R2Secret)
	s.S3Region = strings.TrimSpace(s.S3Region)
	s.S3Bucket = strings.TrimSpace(s.S3Bucket)
	s.S3AccessKey = strings.TrimSpace(s.S3AccessKey)
	s.S3Secret = strings.TrimSpace(s.S3Secret)
	s.OSSRegion = strings.TrimSpace(s.OSSRegion)
	s.OSSBucket = strings.TrimSpace(s.OSSBucket)
	s.OSSAccessKey = strings.TrimSpace(s.OSSAccessKey)
	s.OSSSecret = strings.TrimSpace(s.OSSSecret)
	s.COSRegion = strings.TrimSpace(s.COSRegion)
	s.COSBucket = strings.TrimSpace(s.COSBucket)
	s.COSAccessKey = strings.TrimSpace(s.COSAccessKey)
	s.COSSecret = strings.TrimSpace(s.COSSecret)
	s.CompatEndpoint = strings.TrimSpace(s.CompatEndpoint)
	s.CompatRegion = strings.TrimSpace(s.CompatRegion)
	s.CompatBucket = strings.TrimSpace(s.CompatBucket)
	s.CompatAccessKey = strings.TrimSpace(s.CompatAccessKey)
	s.CompatSecret = strings.TrimSpace(s.CompatSecret)
	for memo, key := range s.Uploaded {
		if strings.TrimSpace(memo) == "" || strings.TrimSpace(key) == "" {
			delete(s.Uploaded, memo)
		}
	}
	if len(s.Uploaded) == 0 {
		s.Uploaded = nil
	}
}

func (a *app) saveCloud(saved *savedSettings) {
	saved.savedCloud = savedCloud{
		GoogleEmail:     a.cloud.googleEmail,
		GoogleRefresh:   a.cloud.googleRefresh,
		Storage:         a.storageKind(),
		R2Endpoint:      a.cloud.r2Endpoint,
		R2Bucket:        a.cloud.r2Bucket,
		R2AccessKey:     a.cloud.r2AccessKey,
		R2Secret:        a.cloud.r2Secret,
		S3Region:        a.cloud.s3Region,
		S3Bucket:        a.cloud.s3Bucket,
		S3AccessKey:     a.cloud.s3AccessKey,
		S3Secret:        a.cloud.s3Secret,
		OSSRegion:       a.cloud.ossRegion,
		OSSBucket:       a.cloud.ossBucket,
		OSSAccessKey:    a.cloud.ossAccessKey,
		OSSSecret:       a.cloud.ossSecret,
		COSRegion:       a.cloud.cosRegion,
		COSBucket:       a.cloud.cosBucket,
		COSAccessKey:    a.cloud.cosAccessKey,
		COSSecret:       a.cloud.cosSecret,
		CompatEndpoint:  a.cloud.compatEndpoint,
		CompatRegion:    a.cloud.compatRegion,
		CompatBucket:    a.cloud.compatBucket,
		CompatAccessKey: a.cloud.compatAccessKey,
		CompatSecret:    a.cloud.compatSecret,
		CompatPathStyle: a.cloud.compatPath,
		Uploaded:        maps.Clone(a.cloud.uploaded),
	}
}

// cloudSecrets are the values of the account and the storage that must not leave the machine, such as in a feedback issue: the Google account and its token, and every kind of storage's endpoint, bucket, access key and secret. Only the storage's kind and the regions are left out. Keep it next to savedCloud: a value added there belongs here too (feedback_test.go checks).
func (c cloudUI) cloudSecrets() []string {
	return []string{
		c.googleEmail, c.googleRefresh,
		c.r2Endpoint, c.r2Bucket, c.r2AccessKey, c.r2Secret,
		c.s3Bucket, c.s3AccessKey, c.s3Secret,
		c.ossBucket, c.ossAccessKey, c.ossSecret,
		c.cosBucket, c.cosAccessKey, c.cosSecret,
		c.compatEndpoint, c.compatBucket, c.compatAccessKey, c.compatSecret,
	}
}

// cloudHooks: the account saves the Google account, the storage and the films uploaded; the environment's KANDO_* settings win over the saved ones (applyCloudEnv).
func (a *app) cloudHooks() areaHooks {
	return areaHooks{
		save: a.saveCloud,
		restore: func(saved savedSettings) {
			applyCloudEnv(a, saved)
			a.cloud.uploaded = maps.Clone(saved.Uploaded)
		},
	}
}
