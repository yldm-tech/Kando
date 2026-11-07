package main

import (
	"errors"
	"regexp"
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

// failureHint says in plain Chinese what a failed job's error means for the user, and the settings section that fixes it ("" when none does): a known reason in the user's words, or else the stage that failed and what its tool said, without the tool's prefixes. A job that was stopped rather than failed, as one whose library was switched, already says so in the app's words (statusText), which are kept as they are.
func failureHint(item films.Film) (reason, section string) {
	detail, isError := strings.CutPrefix(item.Detail, "失败：")
	if !isError {
		return item.Detail, ""
	}
	switch {
	case strings.Contains(detail, "KANDO_WHISPER_MODEL"), strings.Contains(detail, "whisper model"):
		return noticeModel, settings.SectionSpeech
	case strings.Contains(detail, "is not installed"):
		return noticeTools, settings.SectionGeneral
	case strings.Contains(detail, "EveryAPI is not signed in"):
		return noticeEveryAPI, settings.SectionAI
	case strings.Contains(detail, "KANDO_LLM_MODEL"):
		return noticeLLMModel, settings.SectionAI
	case strings.Contains(detail, media.ErrNoSpeech.Error()):
		return "视频里没有识别出说话的声音", ""
	}
	// media.Run names the stage that failed before its error.
	stage, rest, ok := strings.Cut(detail, ": ")
	if !ok {
		stage, rest = "", detail
	}
	if stage == "translate" {
		switch media.ClassifyTranslate(rest) {
		case media.TranslateAuth:
			return "AI 服务不接受这个 Key，去「AI 服务」里检查一下。", settings.SectionAI
		case media.TranslateRejected:
			return "AI 服务拒绝了请求，检查「AI 服务」里的地址和模型。", settings.SectionAI
		case media.TranslateMalformed:
			return "AI 返回的译文格式不对，已重发几次还是不行。重试，或在「AI 服务」里换个模型。", settings.SectionAI
		case media.TranslateTimeout:
			return "AI 服务一直没有回应，已自动重试，还是不行。稍后再重试。", ""
		case media.TranslateNetwork:
			return "连不上 AI 服务，检查网络后再重试。", ""
		case media.TranslateBusy:
			return "AI 服务一直很忙，已自动重试，还是不行。过几分钟再重试。", ""
		}
	}
	failed, known := stageFailed[stage]
	if !known {
		failed, rest = "处理失败", detail
	}
	return failed + "：" + films.Shorten(toolMessage(rest), 72), ""
}

// stageFailed is how a failed card names the stage media.Run says failed, for an error no reason is known for.
var stageFailed = map[string]string{
	"check":      "处理失败",
	"download":   "下载失败",
	"transcribe": "生成字幕失败",
	"translate":  "翻译失败",
	"export":     "导出字幕失败",
}

// toolPrefix is what comes before what a tool says went wrong in its error: the tool and how it exited (yt-dlp: exit status 1:, or whisper-cli: signal: killed: for one the system stopped), and in yt-dlp's ERROR line the word ERROR and the extractor with the video id ([youtube] abc:).
var toolPrefix = regexp.MustCompile(`^(?:[\w.+-]+: (?:exit status [^:]*|signal: [^:]*): )?(?:ERROR: )?(?:\[[^\]]+\] (?:[^\s:]+: )?)?`)

// toolMessage is the part of a stage's error that says what went wrong, in its first line, without the tool's prefixes.
func toolMessage(err string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(err), "\n")
	if msg := strings.TrimSpace(toolPrefix.ReplaceAllString(line, "")); msg != "" {
		return msg
	}
	return line
}
