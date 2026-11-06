package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/sidebar"
)

func (a *app) SignInGoogle() {
	if a.cloud.googleBusy || !a.GoogleConfigured() {
		return
	}
	a.cloud.googleBusy = true
	a.cloud.googleErr = ""
	cfg := cloud.GoogleConfig{
		ClientID:     os.Getenv("LINGOCUT_GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("LINGOCUT_GOOGLE_CLIENT_SECRET"),
		Open:         openBrowser,
	}
	login := a.cloud.googleLogin
	if login == nil {
		login = cloud.Login
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		acc, err := login(ctx, cfg)
		a.update(func() {
			a.cloud.googleBusy = false
			if err != nil {
				a.cloud.googleErr = "Google 登录没有完成"
				return
			}
			a.cloud.googleEmail = acc.Email
			a.cloud.googleRefresh = acc.RefreshToken
			if saveErr := a.saveSettings(); saveErr != nil {
				a.cloud.googleErr = "账号无法保存"
			}
		})
	}()
}

func (a *app) SignOutGoogle() {
	a.cloud.googleEmail = ""
	a.cloud.googleRefresh = ""
	a.cloud.googleErr = ""
	a.cloud.googleBusy = false
	if err := a.saveSettings(); err != nil {
		a.cloud.googleErr = "账号无法保存"
	}
}

func (a *app) UploadView(item films.Film) film.Upload {
	if strings.TrimSpace(item.VideoPath) == "" {
		return film.Upload{}
	}
	view := film.Upload{Show: true, Label: a.uploadLabel()}
	if a.cloud.uploadPath == item.VideoPath {
		view.Detail = uploadProgress(a.cloud.uploadWritten, a.cloud.uploadTotal)
		view.Status = film.UploadBusy
		if a.cloud.uploadTotal > 0 {
			view.Progress = float64(a.cloud.uploadWritten) / float64(a.cloud.uploadTotal)
		}
		return view
	}
	if a.cloud.uploadPath != "" {
		view.Detail = "正在上传另一部影片"
		view.Status = film.UploadBlocked
		return view
	}
	if reason := a.uploadBlocked(); reason != "" {
		view.Detail = reason
		view.Status = film.UploadBlocked
		view.Fix = "去设置"
		if strings.TrimSpace(a.cloud.googleEmail) == "" {
			view.Fix = "去登录"
		}
		return view
	}
	view.Enabled = true
	done := a.uploadMemo(item.VideoPath)
	if a.cloud.uploadErrPath == done && a.cloud.uploadErr != "" {
		view.Detail = a.cloud.uploadErr
		view.Status = film.UploadFailed
		return view
	}
	if key := a.cloud.uploaded[done]; key != "" {
		view.Detail = "已上传 " + key
		view.Status = film.UploadDone
	}
	return view
}

func (a *app) BeginUpload(item films.Film) {
	if strings.TrimSpace(item.VideoPath) == "" || a.cloud.uploadPath != "" || a.uploadBlocked() != "" {
		return
	}
	info, err := os.Stat(item.VideoPath)
	if err != nil || info.IsDir() {
		a.cloud.uploadErr = "视频文件不在本地"
		a.cloud.uploadErrPath = item.VideoPath
		return
	}
	lib := sidebar.Title(a.snap().LibraryDir)
	videoKey := cloud.ObjectKey(a.cloud.googleEmail, lib, item.VideoPath)
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
	a.cloud.uploadPath = item.VideoPath
	a.cloud.uploadWritten, a.cloud.uploadTotal = 0, 0
	a.cloud.uploadErr = ""
	a.cloud.uploadErrPath = ""
	cfg := a.currentStore()
	videoPath := item.VideoPath
	go a.runUpload(videoPath, videoKey, cfg, files)
}

func (a *app) runUpload(videoPath, videoKey string, cfg cloud.Store, files []cloud.File) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	err := a.putFiles(ctx, cfg, files, func(written, total int64) {
		a.update(func() {
			if a.cloud.uploadPath != videoPath {
				return
			}
			a.cloud.uploadWritten, a.cloud.uploadTotal = written, total
		})
	})
	a.update(func() {
		if a.cloud.uploadPath != videoPath {
			return
		}
		a.cloud.uploadPath = ""
		a.cloud.uploadWritten, a.cloud.uploadTotal = 0, 0
		done := storeKind(cfg.Kind) + "\n" + videoPath
		if err != nil {
			a.cloud.uploadErr = uploadFailure(err, cfg.Secret, cfg.AccessKey)
			a.cloud.uploadErrPath = done
			return
		}
		if a.cloud.uploaded == nil {
			a.cloud.uploaded = map[string]string{}
		}
		a.cloud.uploaded[done] = videoKey
	})
}

func (a *app) putFiles(ctx context.Context, cfg cloud.Store, files []cloud.File, progress func(written, total int64)) error {
	if a.cloud.uploader != nil {
		return a.cloud.uploader(ctx, cfg, files, progress)
	}
	client, err := cloud.Open(cfg)
	if err != nil {
		return err
	}
	return cloud.Upload(ctx, client, cfg.Bucket, files, progress)
}

func (a *app) uploadBlocked() string {
	if strings.TrimSpace(a.cloud.googleEmail) == "" {
		return "先在「设置 › 账号与上传」里登录 Google"
	}
	cfg := a.currentStore()
	if storeReady(cfg) {
		return ""
	}
	switch cfg.Kind {
	case cloud.KindS3:
		return "先在「设置 › 账号与上传」里填写 S3"
	case cloud.KindOSS:
		return "先在「设置 › 账号与上传」里填写 OSS"
	case cloud.KindCOS:
		return "先在「设置 › 账号与上传」里填写 COS"
	case cloud.KindCompat:
		return "先在「设置 › 账号与上传」里填写 S3 兼容"
	default:
		return "先在「设置 › 账号与上传」里填写 R2"
	}
}

func (a *app) currentStore() cloud.Store {
	switch a.storageKind() {
	case cloud.KindS3:
		return cloud.Store{
			Kind:      cloud.KindS3,
			Region:    strings.TrimSpace(a.cloud.s3Region),
			Bucket:    strings.TrimSpace(a.cloud.s3Bucket),
			AccessKey: strings.TrimSpace(a.cloud.s3AccessKey),
			Secret:    strings.TrimSpace(a.cloud.s3Secret),
		}
	case cloud.KindOSS:
		return cloud.Store{
			Kind:      cloud.KindOSS,
			Region:    strings.TrimSpace(a.cloud.ossRegion),
			Bucket:    strings.TrimSpace(a.cloud.ossBucket),
			AccessKey: strings.TrimSpace(a.cloud.ossAccessKey),
			Secret:    strings.TrimSpace(a.cloud.ossSecret),
		}
	case cloud.KindCOS:
		return cloud.Store{
			Kind:      cloud.KindCOS,
			Region:    strings.TrimSpace(a.cloud.cosRegion),
			Bucket:    strings.TrimSpace(a.cloud.cosBucket),
			AccessKey: strings.TrimSpace(a.cloud.cosAccessKey),
			Secret:    strings.TrimSpace(a.cloud.cosSecret),
		}
	case cloud.KindCompat:
		return cloud.Store{
			Kind:      cloud.KindCompat,
			Endpoint:  strings.TrimSpace(a.cloud.compatEndpoint),
			Region:    strings.TrimSpace(a.cloud.compatRegion),
			Bucket:    strings.TrimSpace(a.cloud.compatBucket),
			AccessKey: strings.TrimSpace(a.cloud.compatAccessKey),
			Secret:    strings.TrimSpace(a.cloud.compatSecret),
			PathStyle: a.cloud.compatPath,
		}
	default:
		return cloud.Store{
			Kind:      cloud.KindR2,
			Endpoint:  strings.TrimSpace(a.cloud.r2Endpoint),
			Bucket:    strings.TrimSpace(a.cloud.r2Bucket),
			AccessKey: strings.TrimSpace(a.cloud.r2AccessKey),
			Secret:    strings.TrimSpace(a.cloud.r2Secret),
		}
	}
}

func (a *app) storageKind() string { return storageFrom(a.cloud.storage) }

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
	a.cloud.googleEmail = strings.TrimSpace(saved.GoogleEmail)
	a.cloud.googleRefresh = strings.TrimSpace(saved.GoogleRefresh)
	a.cloud.storage = storageFrom(firstSet(os.Getenv("LINGOCUT_STORAGE"), saved.Storage))
	a.cloud.r2Endpoint = firstSet(os.Getenv("LINGOCUT_R2_ENDPOINT"), saved.R2Endpoint)
	a.cloud.r2Bucket = firstSet(os.Getenv("LINGOCUT_R2_BUCKET"), saved.R2Bucket)
	a.cloud.r2AccessKey = firstSet(os.Getenv("LINGOCUT_R2_ACCESS_KEY"), saved.R2AccessKey)
	a.cloud.r2Secret = firstSet(os.Getenv("LINGOCUT_R2_SECRET"), saved.R2Secret)
	a.cloud.s3Region = firstSet(os.Getenv("LINGOCUT_S3_REGION"), saved.S3Region)
	a.cloud.s3Bucket = firstSet(os.Getenv("LINGOCUT_S3_BUCKET"), saved.S3Bucket)
	a.cloud.s3AccessKey = firstSet(os.Getenv("LINGOCUT_S3_ACCESS_KEY"), saved.S3AccessKey)
	a.cloud.s3Secret = firstSet(os.Getenv("LINGOCUT_S3_SECRET"), saved.S3Secret)
	a.cloud.ossRegion = firstSet(os.Getenv("LINGOCUT_OSS_REGION"), saved.OSSRegion)
	a.cloud.ossBucket = firstSet(os.Getenv("LINGOCUT_OSS_BUCKET"), saved.OSSBucket)
	a.cloud.ossAccessKey = firstSet(os.Getenv("LINGOCUT_OSS_ACCESS_KEY"), saved.OSSAccessKey)
	a.cloud.ossSecret = firstSet(os.Getenv("LINGOCUT_OSS_SECRET"), saved.OSSSecret)
	a.cloud.cosRegion = firstSet(os.Getenv("LINGOCUT_COS_REGION"), saved.COSRegion)
	a.cloud.cosBucket = firstSet(os.Getenv("LINGOCUT_COS_BUCKET"), saved.COSBucket)
	a.cloud.cosAccessKey = firstSet(os.Getenv("LINGOCUT_COS_ACCESS_KEY"), saved.COSAccessKey)
	a.cloud.cosSecret = firstSet(os.Getenv("LINGOCUT_COS_SECRET"), saved.COSSecret)
	a.cloud.compatEndpoint = firstSet(os.Getenv("LINGOCUT_S3COMPAT_ENDPOINT"), saved.CompatEndpoint)
	a.cloud.compatRegion = firstSet(os.Getenv("LINGOCUT_S3COMPAT_REGION"), saved.CompatRegion)
	a.cloud.compatBucket = firstSet(os.Getenv("LINGOCUT_S3COMPAT_BUCKET"), saved.CompatBucket)
	a.cloud.compatAccessKey = firstSet(os.Getenv("LINGOCUT_S3COMPAT_ACCESS_KEY"), saved.CompatAccessKey)
	a.cloud.compatSecret = firstSet(os.Getenv("LINGOCUT_S3COMPAT_SECRET"), saved.CompatSecret)
	a.cloud.compatPath = pathStyleFrom(os.Getenv("LINGOCUT_S3COMPAT_PATH_STYLE"), saved.CompatPathStyle)
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
