package main

import (
	"context"

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
	showR2Secret    bool
	uploadPath      string
	uploadWritten   int64
	uploadTotal     int64
	uploadErr       string
	uploadErrPath   string
	uploaded        map[string]string
	uploader        func(context.Context, cloud.Store, []cloud.File, func(written, total int64)) error
}
