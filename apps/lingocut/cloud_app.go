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

func (a *app) R2Endpoint() *string { return &a.r2Endpoint }

func (a *app) R2Bucket() *string { return &a.r2Bucket }

func (a *app) R2AccessKey() *string { return &a.r2AccessKey }

func (a *app) R2Secret() *string { return &a.r2Secret }

func (a *app) ShowR2Secret() bool { return a.showR2Secret }

func (a *app) ToggleR2Secret() { a.showR2Secret = !a.showR2Secret }

func (a *app) UploadView(item films.Film) film.Upload {
	if strings.TrimSpace(item.VideoPath) == "" {
		return film.Upload{}
	}
	view := film.Upload{Show: true, Label: "上传到 R2"}
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
	if a.uploadErrPath == item.VideoPath && a.uploadErr != "" {
		view.Detail = a.uploadErr
		return view
	}
	if key := a.uploaded[item.VideoPath]; key != "" {
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
	cfg := cloud.R2Config{
		Endpoint:  strings.TrimSpace(a.r2Endpoint),
		Bucket:    strings.TrimSpace(a.r2Bucket),
		AccessKey: strings.TrimSpace(a.r2AccessKey),
		Secret:    strings.TrimSpace(a.r2Secret),
	}
	videoPath := item.VideoPath
	go a.runUpload(videoPath, videoKey, cfg, files)
}

func (a *app) runUpload(videoPath, videoKey string, cfg cloud.R2Config, files []cloud.File) {
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
		if err != nil {
			a.uploadErr = uploadFailure(err, cfg.Secret, cfg.AccessKey)
			a.uploadErrPath = videoPath
			return
		}
		if a.uploaded == nil {
			a.uploaded = map[string]string{}
		}
		a.uploaded[videoPath] = videoKey
	})
}

func (a *app) putFiles(ctx context.Context, cfg cloud.R2Config, files []cloud.File, progress func(written, total int64)) error {
	if a.uploader != nil {
		return a.uploader(ctx, cfg, files, progress)
	}
	client, err := cloud.NewR2(cfg.Endpoint, cfg.AccessKey, cfg.Secret)
	if err != nil {
		return err
	}
	return cloud.Upload(ctx, client, cfg.Bucket, files, progress)
}

func (a *app) uploadBlocked() string {
	if strings.TrimSpace(a.googleEmail) == "" {
		return "先在「账号」里登录 Google"
	}
	if cloud.Endpoint(a.r2Endpoint) == "" || strings.TrimSpace(a.r2Bucket) == "" || strings.TrimSpace(a.r2AccessKey) == "" || strings.TrimSpace(a.r2Secret) == "" {
		return "先在「账号」里填写 R2"
	}
	return ""
}

func applyCloudEnv(a *app, saved savedSettings) {
	a.googleEmail = strings.TrimSpace(saved.GoogleEmail)
	a.googleRefresh = strings.TrimSpace(saved.GoogleRefresh)
	a.r2Endpoint = firstSet(os.Getenv("LINGOCUT_R2_ENDPOINT"), saved.R2Endpoint)
	a.r2Bucket = firstSet(os.Getenv("LINGOCUT_R2_BUCKET"), saved.R2Bucket)
	a.r2AccessKey = firstSet(os.Getenv("LINGOCUT_R2_ACCESS_KEY"), saved.R2AccessKey)
	a.r2Secret = firstSet(os.Getenv("LINGOCUT_R2_SECRET"), saved.R2Secret)
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
