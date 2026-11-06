package main

import (
	"context"
	"strings"

	"lingocut/apps/lingocut/cloud"
)

// cloudUI is the state of the account page and of uploads: the Google account, the storage each kind of bucket is reached with, and the upload running now (cloud_app.go).
type cloudUI struct {
	googleEmail     string
	googleRefresh   string
	googleBusy      bool
	googleErr       string
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
	uploadErr       string
	uploadErrPath   string
	uploaded        map[string]string
	uploader        func(context.Context, cloud.Store, []cloud.File, func(written, total int64)) error
}

func newCloudUI() cloudUI { return cloudUI{} }

// savedCloud is the account's part of settings.json: the Google account and each kind of storage.
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
	}
}

// cloudHooks: the account saves the Google account and the storage; the environment's LINGOCUT_* settings win over the saved ones (applyCloudEnv).
func (a *app) cloudHooks() areaHooks {
	return areaHooks{
		save:    a.saveCloud,
		restore: func(saved savedSettings) { applyCloudEnv(a, saved) },
	}
}
