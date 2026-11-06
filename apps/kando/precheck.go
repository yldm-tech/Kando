package main

import (
	"errors"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"
)

// What the library's message bar says when a link or a file cannot be added. The texts are the prototype's.
const (
	noticeEmpty    = "先粘贴一个视频链接，或选择本地文件。"
	noticeNotLink  = "这不像一个视频链接。支持 YouTube、哔哩哔哩，以及 yt-dlp 能解析的上千个网站。"
	noticeNoLib    = "先新建或打开一个资料库"
	noticeModel    = "先在「语音识别」里选一个模型"
	noticeLLMModel = "先在「AI 服务」里填模型"
	noticeEveryAPI = "先登录 EveryAPI，或在「AI 服务」里填 Key"
	noticeTools    = "本机还没装好处理视频的程序"
	noticeNoVideo  = "只能添加视频文件（mp4、mov、mkv、webm 等）。"
)

// precheck says why sources cannot be processed yet, and the settings section that fixes it ("" when none does), in the prototype's order: an empty field, something that is not a link, no library, then the tools and settings a job needs. It reads only what the app already holds, so a click can afford it; what it cannot tell without running a program, as an EveryAPI login that has expired, still fails the job, whose card says so.
func (a *app) precheck(sources []string) (msg, section string) {
	if len(sources) == 0 {
		return noticeEmpty, ""
	}
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			return noticeEmpty, ""
		}
		if _, ok := core.ParseLink(source); !ok {
			return noticeNotLink, ""
		}
	}
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return noticeNoLib, ""
	}
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return "", ""
	}
	// It asks for every tool media.Tools.Check asks for, so a job it lets through does not fail at its first step. That includes yt-dlp for a local video, which the pipeline reads where it is without downloading, because Check still asks for yt-dlp for every job.
	var missing []string
	if tools.YTDLP == "" {
		missing = append(missing, "yt-dlp")
	}
	if tools.FFmpeg == "" {
		missing = append(missing, "ffmpeg")
	}
	if tools.Whisper == "" {
		missing = append(missing, "whisper-cli")
	}
	switch {
	case len(missing) > 0:
		return noticeTools + "：" + strings.Join(missing, "、"), settings.SectionGeneral
	case tools.WhisperModel == "":
		return noticeModel, settings.SectionSpeech
	case tools.Translator.Model == "":
		return noticeLLMModel, settings.SectionAI
	case tools.Translator.Credential == nil && tools.Translator.BaseURL == media.EveryAPIBaseURL && tools.Translator.APIKey == "":
		return noticeEveryAPI, settings.SectionAI
	}
	return "", ""
}

// sourceError says in the library's words why core.NewJob, through Enqueue, refused a source.
func sourceError(err error) string {
	switch {
	case errors.Is(err, core.ErrEmptySource):
		return noticeEmpty
	case err.Error() == "video source must be a valid URL":
		return noticeNotLink
	}
	return err.Error()
}

// failureHint says what a failed job's error means for the user, and the settings section that fixes it ("" when none does).
func failureHint(item films.Film) (reason, section string) {
	detail := strings.TrimPrefix(item.Detail, "失败：")
	switch {
	case strings.Contains(detail, "LINGOCUT_WHISPER_MODEL"), strings.Contains(detail, "whisper model"):
		return noticeModel, settings.SectionSpeech
	case strings.Contains(detail, "is not installed"):
		return noticeTools, settings.SectionGeneral
	case strings.Contains(detail, "EveryAPI is not signed in"):
		return noticeEveryAPI, settings.SectionAI
	case strings.Contains(detail, "LINGOCUT_LLM_MODEL"):
		return noticeLLMModel, settings.SectionAI
	case strings.Contains(detail, media.ErrNoSpeech.Error()):
		return "视频里没有识别出说话的声音", ""
	}
	return films.Shorten(detail, 42), ""
}
