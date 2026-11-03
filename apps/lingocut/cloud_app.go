package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"lingocut/apps/lingocut/cloud"
	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/sidebar"
)

func (a *app) GoogleEmail() string { return a.googleEmail }

func (a *app) GoogleBusy() bool { return a.googleBusy }

func (a *app) GoogleErr() string { return a.googleErr }

func (a *app) GoogleConfigured() bool {
	return strings.TrimSpace(os.Getenv("LINGOCUT_GOOGLE_CLIENT_ID")) != ""
}

func (a *app) SignInGoogle() {
	if a.googleBusy || !a.GoogleConfigured() {
		return
	}
	a.googleBusy = true
	a.googleErr = ""
	cfg := cloud.GoogleConfig{
		ClientID:     os.Getenv("LINGOCUT_GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("LINGOCUT_GOOGLE_CLIENT_SECRET"),
		Open:         openBrowser,
	}
	login := a.googleLogin
	if login == nil {
		login = cloud.Login
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		acc, err := login(ctx, cfg)
		a.update(func() {
			a.googleBusy = false
			if err != nil {
				a.googleErr = "Google 登录没有完成"
				return
			}
			a.googleEmail = acc.Email
			a.googleRefresh = acc.RefreshToken
			if saveErr := a.saveSettings(); saveErr != nil {
				a.googleErr = "账号无法保存"
			}
		})
	}()
}

func (a *app) SignOutGoogle() {
	a.googleEmail = ""
	a.googleRefresh = ""
	a.googleErr = ""
	a.googleBusy = false
	if err := a.saveSettings(); err != nil {
		a.googleErr = "账号无法保存"
	}
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

func (a *app) UploadView(item films.Film) film.Upload {
	if strings.TrimSpace(item.VideoPath) == "" {
		return film.Upload{}
	}
	view := film.Upload{Show: true, Label: a.uploadLabel()}
	if a.uploadPath == item.VideoPath {
		view.Detail = uploadProgress(a.uploadWritten, a.uploadTotal)
		return view
	}
	if a.uploadPath != "" {
		view.Detail = "正在上传另一部影片"
		return view
	}
	if reason := a.uploadBlocked(); reason != "" {
		view.Detail = reason
		return view
	}
	view.Enabled = true
	done := a.uploadMemo(item.VideoPath)
	if a.uploadErrPath == done && a.uploadErr != "" {
		view.Detail = a.uploadErr
		return view
	}
	if key := a.uploaded[done]; key != "" {
		view.Detail = "已上传 " + key
	}
	return view
}

func (a *app) BeginUpload(item films.Film) {
	if strings.TrimSpace(item.VideoPath) == "" || a.uploadPath != "" || a.uploadBlocked() != "" {
		return
	}
	info, err := os.Stat(item.VideoPath)
	if err != nil || info.IsDir() {
		a.uploadErr = "视频文件不在本地"
		a.uploadErrPath = item.VideoPath
		return
	}
	lib := sidebar.Title(a.snap().LibraryDir)
	videoKey := cloud.ObjectKey(a.googleEmail, lib, item.VideoPath)
	files := []cloud.File{{
		Key:         videoKey,
		Path:        item.VideoPath,
		ContentType: cloud.ContentType(item.VideoPath),
	}}
	if item.SubtitlePath != "" {
		if st, err := os.Stat(item.SubtitlePath); err == nil && !st.IsDir() {
			files = append(files, cloud.File{
				Key:         cloud.SiblingKey(videoKey, item.SubtitlePath),
				Path:        item.SubtitlePath,
				ContentType: cloud.ContentType(item.SubtitlePath),
			})
		}
	}
	a.uploadPath = item.VideoPath
	a.uploadWritten, a.uploadTotal = 0, 0
	a.uploadErr = ""
	a.uploadErrPath = ""
	cfg := a.currentStore()
	videoPath := item.VideoPath
	go a.runUpload(videoPath, videoKey, cfg, files)
}

func (a *app) runUpload(videoPath, videoKey string, cfg cloud.Store, files []cloud.File) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	err := a.putFiles(ctx, cfg, files, func(written, total int64) {
		a.update(func() {
			if a.uploadPath != videoPath {
				return
			}
			a.uploadWritten, a.uploadTotal = written, total
		})
	})
	a.update(func() {
		if a.uploadPath != videoPath {
			return
		}
		a.uploadPath = ""
		a.uploadWritten, a.uploadTotal = 0, 0
		done := storeKind(cfg.Kind) + "\n" + videoPath
		if err != nil {
			a.uploadErr = uploadFailure(err, cfg.Secret, cfg.AccessKey)
			a.uploadErrPath = done
			return
		}
		if a.uploaded == nil {
			a.uploaded = map[string]string{}
		}
		a.uploaded[done] = videoKey
	})
}

func (a *app) putFiles(ctx context.Context, cfg cloud.Store, files []cloud.File, progress func(written, total int64)) error {
	if a.uploader != nil {
		return a.uploader(ctx, cfg, files, progress)
	}
	client, err := cloud.Open(cfg)
	if err != nil {
		return err
	}
	return cloud.Upload(ctx, client, cfg.Bucket, files, progress)
}

func (a *app) uploadBlocked() string {
	if strings.TrimSpace(a.googleEmail) == "" {
		return "先在「账号」里登录 Google"
	}
	cfg := a.currentStore()
	if storeReady(cfg) {
		return ""
	}
	switch cfg.Kind {
	case cloud.KindS3:
		return "先在「账号」里填写 S3"
	case cloud.KindOSS:
		return "先在「账号」里填写 OSS"
	case cloud.KindCOS:
		return "先在「账号」里填写 COS"
	case cloud.KindCompat:
		return "先在「账号」里填写 S3 兼容"
	default:
		return "先在「账号」里填写 R2"
	}
}

func (a *app) currentStore() cloud.Store {
	switch a.storageKind() {
	case cloud.KindS3:
		return cloud.Store{
			Kind:      cloud.KindS3,
			Region:    strings.TrimSpace(a.s3Region),
			Bucket:    strings.TrimSpace(a.s3Bucket),
			AccessKey: strings.TrimSpace(a.s3AccessKey),
			Secret:    strings.TrimSpace(a.s3Secret),
		}
	case cloud.KindOSS:
		return cloud.Store{
			Kind:      cloud.KindOSS,
			Region:    strings.TrimSpace(a.ossRegion),
			Bucket:    strings.TrimSpace(a.ossBucket),
			AccessKey: strings.TrimSpace(a.ossAccessKey),
			Secret:    strings.TrimSpace(a.ossSecret),
		}
	case cloud.KindCOS:
		return cloud.Store{
			Kind:      cloud.KindCOS,
			Region:    strings.TrimSpace(a.cosRegion),
			Bucket:    strings.TrimSpace(a.cosBucket),
			AccessKey: strings.TrimSpace(a.cosAccessKey),
			Secret:    strings.TrimSpace(a.cosSecret),
		}
	case cloud.KindCompat:
		return cloud.Store{
			Kind:      cloud.KindCompat,
			Endpoint:  strings.TrimSpace(a.compatEndpoint),
			Region:    strings.TrimSpace(a.compatRegion),
			Bucket:    strings.TrimSpace(a.compatBucket),
			AccessKey: strings.TrimSpace(a.compatAccessKey),
			Secret:    strings.TrimSpace(a.compatSecret),
			PathStyle: a.compatPath,
		}
	default:
		return cloud.Store{
			Kind:      cloud.KindR2,
			Endpoint:  strings.TrimSpace(a.r2Endpoint),
			Bucket:    strings.TrimSpace(a.r2Bucket),
			AccessKey: strings.TrimSpace(a.r2AccessKey),
			Secret:    strings.TrimSpace(a.r2Secret),
		}
	}
}

func (a *app) storageKind() string { return storageFrom(a.storage) }

func (a *app) uploadLabel() string {
	switch a.storageKind() {
	case cloud.KindS3:
		return "上传到 S3"
	case cloud.KindOSS:
		return "上传到 OSS"
	case cloud.KindCOS:
		return "上传到 COS"
	case cloud.KindCompat:
		return "上传到 S3 兼容"
	default:
		return "上传到 R2"
	}
}

func (a *app) uploadMemo(path string) string {
	return a.storageKind() + "\n" + path
}

func storeReady(cfg cloud.Store) bool {
	if strings.TrimSpace(cfg.Bucket) == "" || strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.Secret) == "" {
		return false
	}
	switch cfg.Kind {
	case cloud.KindS3:
		return cloud.Region(cfg.Region) != ""
	case cloud.KindOSS:
		endpoint, region := cloud.OSS(cfg.Region)
		return endpoint != "" && region != ""
	case cloud.KindCOS:
		endpoint, region := cloud.COS(cfg.Region)
		return endpoint != "" && region != ""
	case cloud.KindCompat:
		return cloud.CompatEndpoint(cfg.Endpoint) != "" && cloud.Region(cfg.Region) != ""
	default:
		return cloud.Endpoint(cfg.Endpoint) != ""
	}
}

func storeKind(kind string) string {
	switch kind {
	case cloud.KindS3, cloud.KindOSS, cloud.KindCOS, cloud.KindCompat:
		return kind
	default:
		return cloud.KindR2
	}
}

func applyCloudEnv(a *app, saved savedSettings) {
	a.googleEmail = strings.TrimSpace(saved.GoogleEmail)
	a.googleRefresh = strings.TrimSpace(saved.GoogleRefresh)
	a.storage = storageFrom(firstSet(os.Getenv("LINGOCUT_STORAGE"), saved.Storage))
	a.r2Endpoint = firstSet(os.Getenv("LINGOCUT_R2_ENDPOINT"), saved.R2Endpoint)
	a.r2Bucket = firstSet(os.Getenv("LINGOCUT_R2_BUCKET"), saved.R2Bucket)
	a.r2AccessKey = firstSet(os.Getenv("LINGOCUT_R2_ACCESS_KEY"), saved.R2AccessKey)
	a.r2Secret = firstSet(os.Getenv("LINGOCUT_R2_SECRET"), saved.R2Secret)
	a.s3Region = firstSet(os.Getenv("LINGOCUT_S3_REGION"), saved.S3Region)
	a.s3Bucket = firstSet(os.Getenv("LINGOCUT_S3_BUCKET"), saved.S3Bucket)
	a.s3AccessKey = firstSet(os.Getenv("LINGOCUT_S3_ACCESS_KEY"), saved.S3AccessKey)
	a.s3Secret = firstSet(os.Getenv("LINGOCUT_S3_SECRET"), saved.S3Secret)
	a.ossRegion = firstSet(os.Getenv("LINGOCUT_OSS_REGION"), saved.OSSRegion)
	a.ossBucket = firstSet(os.Getenv("LINGOCUT_OSS_BUCKET"), saved.OSSBucket)
	a.ossAccessKey = firstSet(os.Getenv("LINGOCUT_OSS_ACCESS_KEY"), saved.OSSAccessKey)
	a.ossSecret = firstSet(os.Getenv("LINGOCUT_OSS_SECRET"), saved.OSSSecret)
	a.cosRegion = firstSet(os.Getenv("LINGOCUT_COS_REGION"), saved.COSRegion)
	a.cosBucket = firstSet(os.Getenv("LINGOCUT_COS_BUCKET"), saved.COSBucket)
	a.cosAccessKey = firstSet(os.Getenv("LINGOCUT_COS_ACCESS_KEY"), saved.COSAccessKey)
	a.cosSecret = firstSet(os.Getenv("LINGOCUT_COS_SECRET"), saved.COSSecret)
	a.compatEndpoint = firstSet(os.Getenv("LINGOCUT_S3COMPAT_ENDPOINT"), saved.CompatEndpoint)
	a.compatRegion = firstSet(os.Getenv("LINGOCUT_S3COMPAT_REGION"), saved.CompatRegion)
	a.compatBucket = firstSet(os.Getenv("LINGOCUT_S3COMPAT_BUCKET"), saved.CompatBucket)
	a.compatAccessKey = firstSet(os.Getenv("LINGOCUT_S3COMPAT_ACCESS_KEY"), saved.CompatAccessKey)
	a.compatSecret = firstSet(os.Getenv("LINGOCUT_S3COMPAT_SECRET"), saved.CompatSecret)
	a.compatPath = pathStyleFrom(os.Getenv("LINGOCUT_S3COMPAT_PATH_STYLE"), saved.CompatPathStyle)
}

func storageFrom(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "s3", "aws":
		return cloud.KindS3
	case "oss", "aliyun":
		return cloud.KindOSS
	case "cos", "tencent":
		return cloud.KindCOS
	case "compat", "s3compat":
		return cloud.KindCompat
	default:
		return cloud.KindR2
	}
}

func pathStyleFrom(env string, saved bool) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return saved
	}
}

func firstSet(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uploadProgress(written, total int64) string {
	if total <= 0 {
		return "正在上传"
	}
	return "正在上传 " + strconv.FormatInt(written*100/total, 10) + "%"
}

func uploadFailure(err error, secret, accessKey string) string {
	msg := err.Error()
	if (secret != "" && strings.Contains(msg, secret)) || (accessKey != "" && strings.Contains(msg, accessKey)) {
		return "上传失败"
	}
	return "上传失败：" + msg
}

func openBrowser(raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	return cmd.Run()
}
