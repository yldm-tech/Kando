package settings

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"

	"kando/apps/kando/locales"
	"kando/packages/media"
)

// KitsError says in plain words, in the language of m, why the download of a program failed, as it reads after "下载失败：", or "" when it did not.
func KitsError(m locales.Messages, err error) string {
	var status interface{ StatusCode() int }
	switch {
	case err == nil:
		return ""
	case errors.Is(err, media.ErrOffline):
		return m.PrivacyNoNetworkDownload()
	case errors.Is(err, media.ErrStalled):
		return m.KitsDownloadStalled()
	case errors.As(err, &status):
		return m.KitsDownloadFailedStatus(status.StatusCode())
	case errors.Is(err, media.ErrDisconnected):
		return m.KitsDownloadDisconnected()
	}
	return err.Error()
}

// TrashError is why a speech model's move to the Trash left the whisper catalog model ID where it was: Err, or the model being in use when Err is nil. SpeechDownloadError words it in the language it is shown in.
type TrashError struct {
	ID  string
	Err error
}

func (e TrashError) Error() string {
	if e.Err == nil {
		return e.ID + " is in use"
	}
	return e.ID + ": " + e.Err.Error()
}

// ModelFileError is why the model file at Path, chosen with 选择模型文件…, is not used: Err is media.ErrNotSpeechModel for a file that is not a whisper.cpp model, or why it could not be read. SpeechDownloadError words it in the language it is shown in.
type ModelFileError struct {
	Path string
	Err  error
}

func (e ModelFileError) Error() string { return e.Path + ": " + e.Err.Error() }

func (e ModelFileError) Unwrap() error { return e.Err }

// DownloadError is why the download of the whisper catalog model ID from the download source Source (media.SpeechSources) failed: Err. SpeechDownloadError suggests hf-mirror.com when Hugging Face could not be reached or refused it, and MirrorHelps offers to download ID from there.
type DownloadError struct {
	ID     string
	Source string
	Err    error
}

func (e DownloadError) Error() string { return e.Err.Error() }

func (e DownloadError) Unwrap() error { return e.Err }

// ErrNoModelsDir is a speech model download's error when Kando has no folder to put the models in.
var ErrNoModelsDir = errors.New("no models folder")

// MirrorHelps reports whether downloading from hf-mirror.com may get the speech model whose download err failed, and which model that is: Hugging Face could not be reached, stopped sending, or refused it, as it does where it is blocked. A file that failed its checksum would not be another from the mirror.
func MirrorHelps(err error) (id string, ok bool) {
	var download DownloadError
	var status interface{ StatusCode() int }
	if !errors.As(err, &download) || download.Source != "" {
		return "", false
	}
	if errors.Is(err, media.ErrStalled) || errors.Is(err, media.ErrDisconnected) || errors.As(err, &status) {
		return download.ID, true
	}
	return "", false
}

// SpeechDownloadError says in plain words, in the language of m, why a speech model's download failed, or "" when it did not. A stalled or dropped download kept what it got, so these say that downloading again picks up from there; one from Hugging Face also suggests downloading from hf-mirror.com instead, which is reachable where Hugging Face often is not, and which the page offers next to the error (MirrorHelps).
func SpeechDownloadError(m locales.Messages, err error) string {
	var status interface{ StatusCode() int }
	var trash TrashError
	var file ModelFileError
	var download DownloadError
	fromHuggingFace := errors.As(err, &download) && download.Source == ""
	switch {
	case err == nil:
		return ""
	case errors.Is(err, media.ErrOffline):
		return m.PrivacyNoNetworkDownload()
	case errors.As(err, &file) && errors.Is(file.Err, media.ErrNotSpeechModel):
		return m.SettingsSpeechNotModelFile(filepath.Base(file.Path))
	case errors.As(err, &file):
		// the system's reason, in English, goes to the log
		return m.SettingsSpeechModelFileUnreadable(filepath.Base(file.Path))
	case errors.As(err, &trash) && trash.Err == nil:
		return m.SettingsSpeechTrashInUse(SpeechModelNameIn(m, trash.ID))
	case errors.As(err, &trash):
		// desktop.Trash's error is a fixed English sentinel and the system's reason is in the log, so the page says it in a sentence of its own
		return m.SettingsSpeechTrashFailed(SpeechModelNameIn(m, trash.ID))
	case errors.Is(err, ErrNoModelsDir):
		return m.SettingsSpeechNoModelsDir()
	case errors.Is(err, media.ErrSpeechModelMismatch):
		return m.SettingsSpeechDownloadMismatch()
	case errors.Is(err, media.ErrStalled) && fromHuggingFace:
		return m.SettingsSpeechDownloadStalledMirror()
	case errors.Is(err, media.ErrStalled):
		return m.SettingsSpeechDownloadStalled()
	case errors.As(err, &status) && fromHuggingFace:
		return m.SettingsSpeechDownloadFailedStatusMirror(status.StatusCode())
	case errors.As(err, &status):
		return m.SettingsSpeechDownloadFailedStatus(status.StatusCode())
	case errors.Is(err, media.ErrDisconnected) && fromHuggingFace:
		return m.SettingsSpeechDownloadDisconnectedMirror()
	case errors.Is(err, media.ErrDisconnected):
		return m.SettingsSpeechDownloadDisconnected()
	}
	return err.Error()
}

// LLMModelsError says in plain words, in the language of m, why the AI service's model list could not be fetched.
func LLMModelsError(m locales.Messages, err error) string {
	if errors.Is(err, media.ErrOffline) {
		return m.PrivacyNoCloudAI()
	}
	var status interface{ StatusCode() int }
	if errors.As(err, &status) {
		switch status.StatusCode() {
		case http.StatusUnauthorized, http.StatusForbidden:
			return m.SettingsAIModelsUnauthorized()
		case http.StatusNotFound:
			return m.SettingsAIModelsNotFound()
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return m.SettingsAIModelsTimeout()
	}
	return m.SettingsAIModelsFailed(err.Error())
}

// LLMTestError says in plain words, in the language of m, why the AI service's connection test failed, or "" when it passed. It reads the error as a failed translation's is read (media.ClassifyTranslate), so the test and a failed job say the same reason; what the service answered goes to the log and the status's tooltip instead.
func LLMTestError(m locales.Messages, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, media.ErrNoModel):
		return m.SettingsAITestNoModel()
	case errors.Is(err, media.ErrNoBaseURL):
		return m.SettingsAITestNoBaseURL()
	case errors.Is(err, media.ErrOffline):
		return m.PrivacyNoCloudAI()
	case errors.Is(err, media.ErrEveryAPIMissing):
		return m.PrecheckEveryAPIMissing()
	case errors.Is(err, media.ErrEveryAPISignedOut):
		return m.PrecheckEveryAPISignedOut()
	}
	switch media.ClassifyTranslate(err.Error()) {
	case media.TranslateQuota:
		return m.SettingsAITestQuota()
	case media.TranslateAuth:
		return m.SettingsAITestUnauthorized()
	case media.TranslateRejected:
		return m.SettingsAITestRejected()
	case media.TranslateBusy:
		return m.SettingsAITestBusy()
	case media.TranslateTimeout:
		return m.SettingsAITestTimeout()
	case media.TranslateNetwork:
		return m.SettingsAITestNetwork()
	case media.TranslateMalformed:
		return m.SettingsAITestMalformed()
	}
	return m.SettingsAITestFailed()
}
