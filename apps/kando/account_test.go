package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/kando/cloud"
	"kando/apps/kando/films"
	"kando/apps/kando/settings"
)

func TestCloudSettingsRoundTrip(t *testing.T) {
	a := newApp(&fakePipeline{}, func(func()) {})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.cloud.googleEmail = "a@b.c"
	a.cloud.googleRefresh = "rt-test"
	a.cloud.storage = "s3"
	a.cloud.r2Endpoint = "acct"
	a.cloud.r2Bucket = "clips"
	a.cloud.r2AccessKey = "key"
	a.cloud.r2Secret = "secret-value"
	a.cloud.s3Region = "ap-northeast-1"
	a.cloud.s3Bucket = "aws-clips"
	a.cloud.s3AccessKey = "aws-key"
	a.cloud.s3Secret = "aws-secret"
	a.cloud.ossRegion = "cn-hangzhou"
	a.cloud.ossBucket = "oss-clips"
	a.cloud.ossAccessKey = "oss-key"
	a.cloud.ossSecret = "oss-secret"
	a.cloud.cosRegion = "ap-guangzhou"
	a.cloud.cosBucket = "video-1250000000"
	a.cloud.cosAccessKey = "cos-key"
	a.cloud.cosSecret = "cos-secret"
	a.cloud.compatEndpoint = "https://s3.example.com"
	a.cloud.compatRegion = "us-east-1"
	a.cloud.compatBucket = "compat-clips"
	a.cloud.compatAccessKey = "compat-key"
	a.cloud.compatSecret = "compat-secret"
	a.cloud.compatPath = true
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	loaded := loadSavedSettings(a.settingsPath)
	if loaded.GoogleEmail != "a@b.c" || loaded.R2Bucket != "clips" || loaded.R2Endpoint != "acct" {
		t.Fatalf("email %q bucket %q endpoint %q", loaded.GoogleEmail, loaded.R2Bucket, loaded.R2Endpoint)
	}
	if loaded.GoogleRefresh != "rt-test" || loaded.R2AccessKey != "key" || loaded.R2Secret != "secret-value" {
		t.Fatal("secret fields did not round-trip")
	}
	if loaded.Storage != "s3" || loaded.S3Region != "ap-northeast-1" || loaded.S3Bucket != "aws-clips" || loaded.S3AccessKey != "aws-key" || loaded.S3Secret != "aws-secret" {
		t.Fatal("s3 fields did not round-trip")
	}
	if loaded.OSSRegion != "cn-hangzhou" || loaded.OSSBucket != "oss-clips" || loaded.OSSAccessKey != "oss-key" || loaded.OSSSecret != "oss-secret" {
		t.Fatal("oss fields did not round-trip")
	}
	if loaded.COSRegion != "ap-guangzhou" || loaded.COSBucket != "video-1250000000" || loaded.COSAccessKey != "cos-key" || loaded.COSSecret != "cos-secret" {
		t.Fatal("cos fields did not round-trip")
	}
	if loaded.CompatEndpoint != "https://s3.example.com" || loaded.CompatRegion != "us-east-1" || !loaded.CompatPathStyle || loaded.CompatSecret != "compat-secret" {
		t.Fatal("s3 compatible fields did not round-trip")
	}
}

func TestR2EnvOverridesSavedSettings(t *testing.T) {
	t.Setenv("LINGOCUT_R2_BUCKET", "from-env")
	t.Setenv("LINGOCUT_R2_ENDPOINT", "")
	t.Setenv("LINGOCUT_R2_SECRET", "")
	a := newApp(&fakePipeline{}, func(func()) {})
	t.Setenv("LINGOCUT_STORAGE", "s3")
	t.Setenv("LINGOCUT_S3_REGION", "eu-west-1")
	t.Setenv("LINGOCUT_OSS_REGION", "cn-beijing")
	applyCloudEnv(a, savedSettings{
		Storage: "r2", R2Bucket: "from-file", R2Endpoint: "https://example.r2.cloudflarestorage.com", R2Secret: "saved-secret",
		S3Region: "ap-northeast-1", S3Bucket: "saved-bucket",
		OSSRegion: "cn-hangzhou",
	})
	if a.cloud.r2Bucket != "from-env" || a.cloud.r2Endpoint != "https://example.r2.cloudflarestorage.com" || a.cloud.r2Secret != "saved-secret" {
		t.Fatalf("bucket %q endpoint %q", a.cloud.r2Bucket, a.cloud.r2Endpoint)
	}
	if a.cloud.storage != "s3" || a.cloud.s3Region != "eu-west-1" || a.cloud.s3Bucket != "saved-bucket" || a.cloud.ossRegion != "cn-beijing" {
		t.Fatalf("storage %q region %q bucket %q oss %q", a.cloud.storage, a.cloud.s3Region, a.cloud.s3Bucket, a.cloud.ossRegion)
	}
}

func TestGoogleSignInSavesTheAccount(t *testing.T) {
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_ID", "")
	a, tt, updates := newTestApp(&fakePipeline{})
	a.cloud.googleLogin = func(context.Context, cloud.GoogleConfig) (cloud.Account, error) {
		t.Fatal("login ran without a client id")
		return cloud.Account{}, nil
	}
	a.SignInGoogle()
	if a.cloud.googleBusy {
		t.Fatal("sign-in started without a client id")
	}

	t.Setenv("LINGOCUT_GOOGLE_CLIENT_ID", "client")
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.cloud.googleLogin = func(_ context.Context, cfg cloud.GoogleConfig) (cloud.Account, error) {
		if cfg.ClientID != "client" || cfg.Open == nil {
			return cloud.Account{}, errors.New("bad config")
		}
		return cloud.Account{Email: "a@b.c", RefreshToken: "rt-test"}, nil
	}
	a.SignInGoogle()
	if !a.cloud.googleBusy {
		t.Fatal("sign-in did not start")
	}
	waitUpdate(t, updates)
	if a.cloud.googleEmail != "a@b.c" || a.cloud.googleBusy || a.cloud.googleErr != "" || a.cloud.googleRefresh != "rt-test" {
		t.Fatalf("email %q busy %v err %q", a.cloud.googleEmail, a.cloud.googleBusy, a.cloud.googleErr)
	}
	raw, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"googleEmail": "a@b.c"`) || !strings.Contains(string(raw), `"googleRefresh": "rt-test"`) {
		t.Fatal("account was not saved")
	}
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	if !tt.HasText("a@b.c") || !tt.HasText("退出登录") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("退出登录"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.cloud.googleEmail != "" || a.cloud.googleRefresh != "" || !tt.HasText("使用 Google 登录") {
		t.Fatal("still signed in")
	}
	raw, err = os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "rt-test") || strings.Contains(string(raw), "a@b.c") {
		t.Fatal("signed-out account is still in the file")
	}
}

func TestBeginUploadUsesTheS3Region(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	updates := make(chan func(), 4)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.cloud.googleEmail = "a@b.c"
	a.cloud.storage = cloud.KindS3
	a.cloud.s3Region = "ap-northeast-1"
	a.cloud.s3Bucket = "aws-clips"
	a.cloud.s3AccessKey = "key"
	a.cloud.s3Secret = "secret-value"
	var kind, region, bucket string
	a.cloud.uploader = func(_ context.Context, cfg cloud.Store, _ []cloud.File, _ func(int64, int64)) error {
		kind, region, bucket = cfg.Kind, cfg.Region, cfg.Bucket
		if cfg.Secret != "secret-value" {
			return errors.New("secret was not passed")
		}
		return nil
	}
	a.BeginUpload(films.Film{VideoPath: video})
	waitUpdate(t, updates)
	if kind != cloud.KindS3 || region != "ap-northeast-1" || bucket != "aws-clips" {
		t.Fatalf("kind %q region %q bucket %q", kind, region, bucket)
	}
	if a.cloud.uploaded[cloud.KindS3+"\n"+video] == "" {
		t.Fatal("s3 upload was not recorded")
	}
}

func TestBeginUploadUsesTheOSSRegion(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	updates := make(chan func(), 4)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.cloud.googleEmail = "a@b.c"
	a.cloud.storage = "aliyun"
	a.cloud.ossRegion = "cn-hangzhou"
	a.cloud.ossBucket = "oss-clips"
	a.cloud.ossAccessKey = "key"
	a.cloud.ossSecret = "secret-value"
	var kind, region, bucket string
	a.cloud.uploader = func(_ context.Context, cfg cloud.Store, _ []cloud.File, _ func(int64, int64)) error {
		kind, region, bucket = cfg.Kind, cfg.Region, cfg.Bucket
		return nil
	}
	a.BeginUpload(films.Film{VideoPath: video})
	waitUpdate(t, updates)
	if kind != cloud.KindOSS || region != "cn-hangzhou" || bucket != "oss-clips" {
		t.Fatalf("kind %q region %q bucket %q", kind, region, bucket)
	}
	if a.cloud.uploaded[cloud.KindOSS+"\n"+video] == "" {
		t.Fatal("oss upload was not recorded")
	}
}

func TestBeginUploadUsesCOS(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	updates := make(chan func(), 4)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.cloud.googleEmail = "a@b.c"
	a.cloud.storage = "tencent"
	a.cloud.cosRegion = "ap-guangzhou"
	a.cloud.cosBucket = "video-1250000000"
	a.cloud.cosAccessKey = "key"
	a.cloud.cosSecret = "secret-value"
	var kind, region, bucket string
	a.cloud.uploader = func(_ context.Context, cfg cloud.Store, _ []cloud.File, _ func(int64, int64)) error {
		kind, region, bucket = cfg.Kind, cfg.Region, cfg.Bucket
		return nil
	}
	a.BeginUpload(films.Film{VideoPath: video})
	waitUpdate(t, updates)
	if kind != cloud.KindCOS || region != "ap-guangzhou" || bucket != "video-1250000000" {
		t.Fatalf("kind %q region %q bucket %q", kind, region, bucket)
	}
}

func TestBeginUploadUsesACompatibleEndpoint(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "talk.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	updates := make(chan func(), 4)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.cloud.googleEmail = "a@b.c"
	a.cloud.storage = "compat"
	a.cloud.compatEndpoint = "https://play.min.io"
	a.cloud.compatRegion = "us-east-1"
	a.cloud.compatBucket = "clips"
	a.cloud.compatAccessKey = "key"
	a.cloud.compatSecret = "secret-value"
	a.cloud.compatPath = true
	var kind, endpoint string
	var pathStyle bool
	a.cloud.uploader = func(_ context.Context, cfg cloud.Store, _ []cloud.File, _ func(int64, int64)) error {
		kind, endpoint, pathStyle = cfg.Kind, cfg.Endpoint, cfg.PathStyle
		return nil
	}
	a.BeginUpload(films.Film{VideoPath: video})
	waitUpdate(t, updates)
	if kind != cloud.KindCompat || endpoint != "https://play.min.io" || !pathStyle {
		t.Fatalf("kind %q endpoint %q path %v", kind, endpoint, pathStyle)
	}
}

func TestAccountPageShowsCOSFields(t *testing.T) {
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_ID", "")
	a, tt, _ := newTestApp(&fakePipeline{})
	a.cloud.storage = cloud.KindCOS
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	for _, text := range []string{"腾讯云 COS", "ap-guangzhou", "1250000000", "SecretId"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestAccountPageShowsOSSFields(t *testing.T) {
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_ID", "")
	a, tt, _ := newTestApp(&fakePipeline{})
	a.cloud.storage = cloud.KindOSS
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	for _, text := range []string{"阿里云 OSS", "Region", "cn-hangzhou", "Bucket", "Access Key", "Secret"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if tt.HasText("Account ID") || tt.HasText("ap-northeast-1") {
		t.Fatal("another provider's hint is showing on the OSS form")
	}
}

func TestAccountPageShowsS3Fields(t *testing.T) {
	t.Setenv("LINGOCUT_GOOGLE_CLIENT_ID", "")
	a, tt, _ := newTestApp(&fakePipeline{})
	a.cloud.storage = cloud.KindS3
	tt.Frame()
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	for _, text := range []string{"Amazon S3", "Region", "例如 ap-northeast-1", "Bucket", "Access Key", "Secret"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
	if tt.HasText("Account ID") {
		t.Fatal("the R2 endpoint hint is showing on the S3 form")
	}
}

func TestUploadFailureHidesTheSecret(t *testing.T) {
	msg := uploadFailure(errors.New("signature secret-value"), "secret-value", "key")
	if msg != "上传失败" || strings.Contains(msg, "secret-value") {
		t.Fatalf("msg = %q", msg)
	}
	plain := uploadFailure(errors.New("status 403"), "secret-value", "key")
	if plain != "上传失败：status 403" {
		t.Fatalf("msg = %q", plain)
	}
}

// TestUploadBlockedPointsToTheSettingsWindow checks every message that blocks an upload sends the user to 设置 › 账号与上传, where the account now lives, not to the 账号 row the sidebar no longer has.
func TestUploadBlockedPointsToTheSettingsWindow(t *testing.T) {
	a := newApp(&fakePipeline{}, func(func()) {})
	if got, want := a.uploadBlocked(), "先在「设置 › 账号与上传」里登录 Google"; got != want {
		t.Errorf("signed out: %q, want %q", got, want)
	}
	a.cloud.googleEmail = "a@b.c"
	for kind, name := range map[string]string{
		cloud.KindR2: "R2", cloud.KindS3: "S3", cloud.KindOSS: "OSS", cloud.KindCOS: "COS", cloud.KindCompat: "S3 兼容",
	} {
		a.cloud.storage = kind
		if got, want := a.uploadBlocked(), "先在「设置 › 账号与上传」里填写 "+name; got != want {
			t.Errorf("%s: %q, want %q", kind, got, want)
		}
	}
}
