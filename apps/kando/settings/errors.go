package settings

import (
	"context"
	"errors"
	"net/http"

	"kando/apps/kando/locales"
	"kando/packages/media"
)

// KitsError says in plain words, in the language of m, why the download of a program failed, as it reads after "下载失败：", or "" when it did not.
func KitsError(m locales.Messages, err error) string {
	var status interface{ StatusCode() int }
	switch {
	case err == nil:
		return ""
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

// ErrNoModelsDir is a speech model download's error when Kando has no folder to put the models in.
var ErrNoModelsDir = errors.New("no models folder")

// SpeechDownloadError says in plain words, in the language of m, why a speech model's download failed, or "" when it did not. A stalled or dropped download kept what it got, so these say that downloading again picks up from there.
func SpeechDownloadError(m locales.Messages, err error) string {
	var status interface{ StatusCode() int }
	var trash TrashError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &trash) && trash.Err == nil:
		return m.SettingsSpeechTrashInUse(SpeechModelNameIn(m, trash.ID))
	case errors.As(err, &trash):
		// desktop.Trash's error is a fixed English sentinel and the system's reason is in the log, so the page says it in a sentence of its own
		return m.SettingsSpeechTrashFailed(SpeechModelNameIn(m, trash.ID))
	case errors.Is(err, ErrNoModelsDir):
		return m.SettingsSpeechNoModelsDir()
	case errors.Is(err, media.ErrStalled):
		return m.SettingsSpeechDownloadStalled()
	case errors.As(err, &status):
		return m.SettingsSpeechDownloadFailedStatus(status.StatusCode())
	case errors.Is(err, media.ErrDisconnected):
		return m.SettingsSpeechDownloadDisconnected()
	}
	return err.Error()
}

// LLMModelsError says in plain words, in the language of m, why the AI service's model list could not be fetched.
func LLMModelsError(m locales.Messages, err error) string {
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
