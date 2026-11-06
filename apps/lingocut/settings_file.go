package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"lingocut/apps/lingocut/film"
	"lingocut/packages/media"
)

// settings.json: what saveSettings writes and loadSavedSettings reads.

func (a *app) saveSettings() error {
	if a.settingsPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(a.settingsPath), 0o700); err != nil {
		return err
	}
	contextOn := a.translateContext
	yieldSubs := a.player.yieldSubs
	shelf := a.snap()
	saved := savedSettings{
		LibraryDir:       shelf.LibraryDir,
		Libraries:        append([]string{}, a.knownLibraries()...),
		LibraryInodes:    inodesForSave(shelf.Inodes),
		LLMProvider:      a.llmProvider,
		Language:         a.language,
		TranslatePrompt:  a.translatePrompt,
		TranslateContext: &contextOn,
		GoogleEmail:      a.googleEmail,
		GoogleRefresh:    a.googleRefresh,
		Storage:          a.storageKind(),
		R2Endpoint:       a.r2Endpoint,
		R2Bucket:         a.r2Bucket,
		R2AccessKey:      a.r2AccessKey,
		R2Secret:         a.r2Secret,
		S3Region:         a.s3Region,
		S3Bucket:         a.s3Bucket,
		S3AccessKey:      a.s3AccessKey,
		S3Secret:         a.s3Secret,
		OSSRegion:        a.ossRegion,
		OSSBucket:        a.ossBucket,
		OSSAccessKey:     a.ossAccessKey,
		OSSSecret:        a.ossSecret,
		COSRegion:        a.cosRegion,
		COSBucket:        a.cosBucket,
		COSAccessKey:     a.cosAccessKey,
		COSSecret:        a.cosSecret,
		CompatEndpoint:   a.compatEndpoint,
		CompatRegion:     a.compatRegion,
		CompatBucket:     a.compatBucket,
		CompatAccessKey:  a.compatAccessKey,
		CompatSecret:     a.compatSecret,
		CompatPathStyle:  a.compatPath,
		Look:             changedLook(a.player.look),
		SubMode:          a.player.subMode,
		SubOrder:         a.player.subOrder,
		Untranslated:     a.player.untranslated,
		YieldSubs:        &yieldSubs,
		Onboarded:        a.overlay.onboarded,
	}
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.WhisperModel = tools.WhisperModel
		saved.LLMBaseURL = tools.Translator.BaseURL
		saved.LLMModel = tools.Translator.Model
		saved.LLMAPIKey = tools.Translator.APIKey
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := a.settingsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.settingsPath)
}

func loadSavedSettings(path string) savedSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		return savedSettings{}
	}
	// A saved look is read over the default, so a field it lacks, such as one added after it was saved, keeps the default.
	look := film.DefaultLook()
	saved := savedSettings{Look: &look}
	if json.Unmarshal(data, &saved) != nil {
		return savedSettings{}
	}
	saved.LibraryDir = strings.TrimSpace(saved.LibraryDir)
	saved.Libraries = cleanLibraries(saved.Libraries)
	saved.WhisperModel = strings.TrimSpace(saved.WhisperModel)
	saved.LLMBaseURL = strings.TrimSpace(saved.LLMBaseURL)
	saved.LLMModel = strings.TrimSpace(saved.LLMModel)
	saved.LLMAPIKey = strings.TrimSpace(saved.LLMAPIKey)
	saved.LLMProvider = strings.TrimSpace(saved.LLMProvider)
	saved.Language = strings.TrimSpace(saved.Language)
	saved.TranslatePrompt = strings.TrimSpace(saved.TranslatePrompt)
	saved.GoogleEmail = strings.TrimSpace(saved.GoogleEmail)
	saved.GoogleRefresh = strings.TrimSpace(saved.GoogleRefresh)
	saved.Storage = strings.TrimSpace(saved.Storage)
	saved.R2Endpoint = strings.TrimSpace(saved.R2Endpoint)
	saved.R2Bucket = strings.TrimSpace(saved.R2Bucket)
	saved.R2AccessKey = strings.TrimSpace(saved.R2AccessKey)
	saved.R2Secret = strings.TrimSpace(saved.R2Secret)
	saved.S3Region = strings.TrimSpace(saved.S3Region)
	saved.S3Bucket = strings.TrimSpace(saved.S3Bucket)
	saved.S3AccessKey = strings.TrimSpace(saved.S3AccessKey)
	saved.S3Secret = strings.TrimSpace(saved.S3Secret)
	saved.OSSRegion = strings.TrimSpace(saved.OSSRegion)
	saved.OSSBucket = strings.TrimSpace(saved.OSSBucket)
	saved.OSSAccessKey = strings.TrimSpace(saved.OSSAccessKey)
	saved.OSSSecret = strings.TrimSpace(saved.OSSSecret)
	saved.COSRegion = strings.TrimSpace(saved.COSRegion)
	saved.COSBucket = strings.TrimSpace(saved.COSBucket)
	saved.COSAccessKey = strings.TrimSpace(saved.COSAccessKey)
	saved.COSSecret = strings.TrimSpace(saved.COSSecret)
	saved.CompatEndpoint = strings.TrimSpace(saved.CompatEndpoint)
	saved.CompatRegion = strings.TrimSpace(saved.CompatRegion)
	saved.CompatBucket = strings.TrimSpace(saved.CompatBucket)
	saved.CompatAccessKey = strings.TrimSpace(saved.CompatAccessKey)
	saved.CompatSecret = strings.TrimSpace(saved.CompatSecret)
	return saved
}

type savedSettings struct {
	LibraryDir       string                `json:"libraryDir,omitempty"`
	Libraries        []string              `json:"libraries,omitempty"`
	LibraryInodes    map[string]savedInode `json:"libraryInodes,omitempty"`
	WhisperModel     string                `json:"whisperModel,omitempty"`
	LLMProvider      string                `json:"llmProvider,omitempty"`
	LLMBaseURL       string                `json:"llmBaseURL,omitempty"`
	LLMModel         string                `json:"llmModel,omitempty"`
	LLMAPIKey        string                `json:"llmAPIKey,omitempty"`
	Language         string                `json:"language,omitempty"`
	TranslatePrompt  string                `json:"translatePrompt,omitempty"`
	TranslateContext *bool                 `json:"translateContext,omitempty"`
	GoogleEmail      string                `json:"googleEmail,omitempty"`
	GoogleRefresh    string                `json:"googleRefresh,omitempty"`
	Storage          string                `json:"storage,omitempty"`
	R2Endpoint       string                `json:"r2Endpoint,omitempty"`
	R2Bucket         string                `json:"r2Bucket,omitempty"`
	R2AccessKey      string                `json:"r2AccessKey,omitempty"`
	R2Secret         string                `json:"r2Secret,omitempty"`
	S3Region         string                `json:"s3Region,omitempty"`
	S3Bucket         string                `json:"s3Bucket,omitempty"`
	S3AccessKey      string                `json:"s3AccessKey,omitempty"`
	S3Secret         string                `json:"s3Secret,omitempty"`
	OSSRegion        string                `json:"ossRegion,omitempty"`
	OSSBucket        string                `json:"ossBucket,omitempty"`
	OSSAccessKey     string                `json:"ossAccessKey,omitempty"`
	OSSSecret        string                `json:"ossSecret,omitempty"`
	COSRegion        string                `json:"cosRegion,omitempty"`
	COSBucket        string                `json:"cosBucket,omitempty"`
	COSAccessKey     string                `json:"cosAccessKey,omitempty"`
	COSSecret        string                `json:"cosSecret,omitempty"`
	CompatEndpoint   string                `json:"s3CompatEndpoint,omitempty"`
	CompatRegion     string                `json:"s3CompatRegion,omitempty"`
	CompatBucket     string                `json:"s3CompatBucket,omitempty"`
	CompatAccessKey  string                `json:"s3CompatAccessKey,omitempty"`
	CompatSecret     string                `json:"s3CompatSecret,omitempty"`
	CompatPathStyle  bool                  `json:"s3CompatPathStyle,omitempty"`
	Look             *film.Look            `json:"look,omitempty"`
	SubMode          string                `json:"subMode,omitempty"`
	SubOrder         string                `json:"subOrder,omitempty"`
	Untranslated     string                `json:"untranslated,omitempty"`
	YieldSubs        *bool                 `json:"yieldSubs,omitempty"`
	Onboarded        bool                  `json:"onboarded,omitempty"`
}
