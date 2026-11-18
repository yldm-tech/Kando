package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/feedback"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
)

func (a *app) SignInGoogle() {
	if a.cloud.googleBusy || !a.GoogleConfigured() {
		return
	}
	if a.privacy.offline {
		a.cloud.googleErr = locales.Messages.PrivacyNoSignIn
		return
	}
	a.cloud.googleBusy = true
	a.cloud.googleErr = nil
	cfg := cloud.GoogleConfig{
		ClientID:     os.Getenv("KANDO_GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("KANDO_GOOGLE_CLIENT_SECRET"),
		Open:         openBrowser,
		Language:     a.prefs.interfaceLanguage,
	}
	login := a.cloud.googleLogin
	if login == nil {
		login = cloud.Login
	}
	// Offline mode turned on meanwhile stops the sign-in (privacyContext).
	net := a.privacyContext()
	go func() {
		ctx, cancel := context.WithTimeout(net, 3*time.Minute)
		defer cancel()
		acc, err := login(ctx, cfg)
		a.update(func() {
			a.cloud.googleBusy = false
			if err != nil {
				a.cloud.googleErr = locales.Messages.AccountGoogleFailed
				if a.privacy.offline {
					a.cloud.googleErr = locales.Messages.PrivacyNoSignIn
				}
				return
			}
			a.cloud.googleEmail = acc.Email
			a.cloud.googleRefresh = acc.RefreshToken
			if saveErr := a.saveSettings(); saveErr != nil {
				a.cloud.googleErr = locales.Messages.AccountSaveFailed
			}
		})
	}()
}

func (a *app) SignOutGoogle() {
	a.cloud.googleEmail = ""
	a.cloud.googleRefresh = ""
	a.cloud.googleErr = nil
	a.cloud.googleBusy = false
	if err := a.saveSettings(); err != nil {
		a.cloud.googleErr = locales.Messages.AccountSaveFailed
	}
}

// UploadView is what the player's upload button and capsule show of item's upload, in the interface's language: the player reads it in its bindings, which follow the language as it changes.
func (a *app) UploadView(item films.Film) film.Upload {
	if strings.TrimSpace(item.VideoPath) == "" {
		return film.Upload{}
	}
	m := a.messages()
	view := film.Upload{Show: true, Label: a.uploadLabel()}
	if a.cloud.uploadPath == item.VideoPath {
		view.Detail = uploadProgress(m, a.cloud.uploadWritten, a.cloud.uploadTotal)
		view.Status = film.UploadBusy
		view.Canceling = a.cloud.uploadCancel == nil
		if a.cloud.uploadTotal > 0 {
			view.Progress = float64(a.cloud.uploadWritten) / float64(a.cloud.uploadTotal)
		}
		return view
	}
	if a.cloud.uploadPath != "" {
		view.Detail = m.AccountUploadingOther()
		view.Status = film.UploadBlocked
		return view
	}
	if reason := a.uploadBlocked(); reason != "" {
		view.Detail = reason
		view.Status = film.UploadBlocked
		// The account's settings do not lift offline mode.
		if !a.privacy.offline {
			view.Fix = m.AccountFixSettings()
		}
		return view
	}
	view.Enabled = true
	done := a.uploadMemo(item.VideoPath)
	if a.cloud.uploadErrPath == done && a.cloud.uploadErr != nil {
		view.Detail = a.cloud.uploadErr(m)
		view.Status = film.UploadFailed
		if a.cloud.uploadCanceled {
			view.Status = film.UploadCanceled
		}
		if a.cloud.uploadFix {
			view.Fix = m.AccountFixSettings()
		}
		return view
	}
	if key := a.cloud.uploaded[done]; key != "" {
		view.Detail = m.AccountUploaded(key)
		view.Status = film.UploadDone
	}
	return view
}

func (a *app) BeginUpload(item films.Film) {
	if strings.TrimSpace(item.VideoPath) == "" || a.cloud.uploadPath != "" || a.uploadBlocked() != "" {
		return
	}
	a.cloud.uploadCanceled = false
	a.cloud.uploadFix = false
	info, err := os.Stat(item.VideoPath)
	if err != nil || info.IsDir() {
		a.cloud.uploadErr = locales.Messages.AccountVideoMissing
		a.cloud.uploadErrPath = a.uploadMemo(item.VideoPath)
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
	a.cloud.uploadErr = nil
	a.cloud.uploadErrPath = ""
	cfg := a.currentStore()
	videoPath := item.VideoPath
	// No deadline: a long film on a slow connection takes as long as it takes. The upload ends when CancelUpload cancels ctx, or with cloud.ErrStalled when the storage stops taking it.
	ctx, cancel := context.WithCancel(context.Background())
	a.cloud.uploadCancel = cancel
	go a.runUpload(ctx, cancel, videoPath, videoKey, cfg, files)
}

// CancelUpload stops the upload running now. The upload ends as soon as the storage's client gives up, and runUpload then frees the slot for another film and says the upload was canceled.
func (a *app) CancelUpload() {
	if a.cloud.uploadCancel == nil {
		return
	}
	a.cloud.uploadCancel()
	a.cloud.uploadCancel = nil
}

// runUpload sends files with ctx, which BeginUpload made and cancel releases, and records how it ended: the object key of a film uploaded, saved in settings.json, or why it failed, or that it was canceled.
func (a *app) runUpload(ctx context.Context, cancel context.CancelFunc, videoPath, videoKey string, cfg cloud.Store, files []cloud.File) {
	defer cancel()
	// Progress comes every 256 KB; only a change of the whole percent the upload shows (uploadProgress) goes to the engine's goroutine, as every update runs the app's bindings and hooks again. The finish posts its own update below.
	var mu sync.Mutex
	shown := -1
	err := a.putFiles(ctx, cfg, files, func(written, total int64) {
		percent := 0
		if total > 0 {
			percent = int(written * 100 / total)
		}
		mu.Lock()
		same := percent == shown
		shown = percent
		mu.Unlock()
		if same {
			return
		}
		a.update(func() {
			if a.cloud.uploadPath != videoPath {
				return
			}
			a.cloud.uploadWritten, a.cloud.uploadTotal = written, total
		})
	})
	// Whether CancelUpload stopped it is decided here, before the deferred cancel runs: a.update may run its function after runUpload returns (it only posts it to the engine's goroutine), when ctx is canceled for every upload.
	canceled := err != nil && (errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled))
	a.update(func() {
		if a.cloud.uploadPath != videoPath {
			return
		}
		a.cloud.uploadPath = ""
		a.cloud.uploadWritten, a.cloud.uploadTotal = 0, 0
		a.cloud.uploadCancel = nil
		done := storeKind(cfg.Kind) + "\n" + videoPath
		if canceled {
			a.cloud.uploadErr = locales.Messages.AccountUploadCanceled
			a.cloud.uploadErrPath = done
			a.cloud.uploadCanceled = true
			return
		}
		if err != nil {
			// The capsule says what went wrong and what to do; the storage's whole error goes to the log file, where the feedback dialog's log and the user can read it.
			log.Printf("upload: %s to %s failed: %s", filepath.Base(videoPath), storeKind(cfg.Kind), feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(err.Error()))
			a.cloud.uploadErr = func(m locales.Messages) string { return uploadFailure(m, err, cfg) }
			a.cloud.uploadErrPath = done
			a.cloud.uploadFix = uploadNeedsSettings(err)
			return
		}
		if a.cloud.uploaded == nil {
			a.cloud.uploaded = map[string]string{}
		}
		a.cloud.uploaded[done] = videoKey
		if saveErr := a.saveSettings(); saveErr != nil {
			a.saveErr = a.messages().AccountUploadUnsaved(saveErr.Error())
		}
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

// uploadBlocked is why an upload cannot start now, in the interface's language, "" when one can. A Google account is not needed: it only names the folder the film goes under (cloud.ObjectKey).
func (a *app) uploadBlocked() string {
	m := a.messages()
	if a.privacy.offline {
		return m.PrivacyNoUpload()
	}
	cfg := a.currentStore()
	if storeReady(cfg) {
		return ""
	}
	return m.AccountNeedsStore(uploadStoreName(m, cfg.Kind))
}

// uploadStoreName names a kind of storage as the upload button and its messages do: the brand of R2, S3, OSS and COS, and S3 兼容 in m's language.
func uploadStoreName(m locales.Messages, kind string) string {
	switch kind {
	case cloud.KindS3:
		return "S3"
	case cloud.KindOSS:
		return "OSS"
	case cloud.KindCOS:
		return "COS"
	case cloud.KindCompat:
		return m.AccountStoreCompat()
	default:
		return "R2"
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

// uploadLabel is the upload button's words, in the interface's language.
func (a *app) uploadLabel() string {
	m := a.messages()
	return m.AccountUploadTo(uploadStoreName(m, a.storageKind()))
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
	a.cloud.storage = storageFrom(firstSet(os.Getenv("KANDO_STORAGE"), saved.Storage))
	a.cloud.r2Endpoint = firstSet(os.Getenv("KANDO_R2_ENDPOINT"), saved.R2Endpoint)
	a.cloud.r2Bucket = firstSet(os.Getenv("KANDO_R2_BUCKET"), saved.R2Bucket)
	a.cloud.r2AccessKey = firstSet(os.Getenv("KANDO_R2_ACCESS_KEY"), saved.R2AccessKey)
	a.cloud.r2Secret = firstSet(os.Getenv("KANDO_R2_SECRET"), saved.R2Secret)
	a.cloud.s3Region = firstSet(os.Getenv("KANDO_S3_REGION"), saved.S3Region)
	a.cloud.s3Bucket = firstSet(os.Getenv("KANDO_S3_BUCKET"), saved.S3Bucket)
	a.cloud.s3AccessKey = firstSet(os.Getenv("KANDO_S3_ACCESS_KEY"), saved.S3AccessKey)
	a.cloud.s3Secret = firstSet(os.Getenv("KANDO_S3_SECRET"), saved.S3Secret)
	a.cloud.ossRegion = firstSet(os.Getenv("KANDO_OSS_REGION"), saved.OSSRegion)
	a.cloud.ossBucket = firstSet(os.Getenv("KANDO_OSS_BUCKET"), saved.OSSBucket)
	a.cloud.ossAccessKey = firstSet(os.Getenv("KANDO_OSS_ACCESS_KEY"), saved.OSSAccessKey)
	a.cloud.ossSecret = firstSet(os.Getenv("KANDO_OSS_SECRET"), saved.OSSSecret)
	a.cloud.cosRegion = firstSet(os.Getenv("KANDO_COS_REGION"), saved.COSRegion)
	a.cloud.cosBucket = firstSet(os.Getenv("KANDO_COS_BUCKET"), saved.COSBucket)
	a.cloud.cosAccessKey = firstSet(os.Getenv("KANDO_COS_ACCESS_KEY"), saved.COSAccessKey)
	a.cloud.cosSecret = firstSet(os.Getenv("KANDO_COS_SECRET"), saved.COSSecret)
	a.cloud.compatEndpoint = firstSet(os.Getenv("KANDO_S3COMPAT_ENDPOINT"), saved.CompatEndpoint)
	a.cloud.compatRegion = firstSet(os.Getenv("KANDO_S3COMPAT_REGION"), saved.CompatRegion)
	a.cloud.compatBucket = firstSet(os.Getenv("KANDO_S3COMPAT_BUCKET"), saved.CompatBucket)
	a.cloud.compatAccessKey = firstSet(os.Getenv("KANDO_S3COMPAT_ACCESS_KEY"), saved.CompatAccessKey)
	a.cloud.compatSecret = firstSet(os.Getenv("KANDO_S3COMPAT_SECRET"), saved.CompatSecret)
	a.cloud.compatPath = pathStyleFrom(os.Getenv("KANDO_S3COMPAT_PATH_STYLE"), saved.CompatPathStyle)
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

// uploadProgress says how far the upload running now got, in m's language.
func uploadProgress(m locales.Messages, written, total int64) string {
	if total <= 0 {
		return m.AccountUploading()
	}
	return m.AccountUploadingPercent(int(written * 100 / total))
}

// uploadFailure says why an upload to cfg failed and what to do about it, in m's language. An error none of cloud.Diagnose's reasons names reads as the storage's own code and message, unless they hold the secret or the access key, or, when the storage did not answer, points to the log file, which has the whole error.
func uploadFailure(m locales.Messages, err error, cfg cloud.Store) string {
	f := cloud.Diagnose(err)
	switch f.Reason {
	case cloud.ReasonStalled:
		return m.AccountUploadStalled()
	case cloud.ReasonOffline:
		return m.AccountUploadOffline()
	case cloud.ReasonUnreachable:
		if f.Host != "" {
			return m.AccountUploadUnreachable(f.Host)
		}
	case cloud.ReasonInsecure:
		if f.Host != "" {
			return m.AccountUploadInsecure(f.Host)
		}
	case cloud.ReasonKeys:
		return m.AccountUploadKeys()
	case cloud.ReasonDenied:
		return m.AccountUploadDenied(cfg.Bucket)
	case cloud.ReasonNoBucket:
		return m.AccountUploadNoBucket(cfg.Bucket)
	case cloud.ReasonRegion:
		return m.AccountUploadRegion(cfg.Bucket)
	case cloud.ReasonTooLarge:
		return m.AccountUploadTooLarge()
	}
	if f.Detail == "" {
		return m.AccountUploadFailedSeeLog()
	}
	if (cfg.Secret != "" && strings.Contains(f.Detail, cfg.Secret)) || (cfg.AccessKey != "" && strings.Contains(f.Detail, cfg.AccessKey)) {
		return m.AccountUploadFailed()
	}
	return m.AccountUploadFailedBecause(f.Detail)
}

// uploadNeedsSettings is whether an upload failed because of what is filled in for the storage in 设置 › 账号与上传: the endpoint, region, bucket or keys, which the capsule's button opens.
func uploadNeedsSettings(err error) bool {
	switch cloud.Diagnose(err).Reason {
	case cloud.ReasonUnreachable, cloud.ReasonInsecure, cloud.ReasonKeys, cloud.ReasonDenied, cloud.ReasonNoBucket, cloud.ReasonRegion:
		return true
	}
	return false
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
