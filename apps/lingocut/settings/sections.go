package settings

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"lingocut/apps/lingocut/settings/sectionid"
)

// Sections of the settings window, as App.OpenSettings in the main package takes them and the window's sidebar lists them. The values are declared in the leaf package sectionid, which pages outside the settings use instead of importing this package.
const (
	SectionGeneral   = sectionid.General
	SectionSpeech    = sectionid.Speech
	SectionAI        = sectionid.AI
	SectionTranslate = sectionid.Translate
	SectionLook      = sectionid.Look
	SectionShortcuts = sectionid.Shortcuts
	SectionAccount   = sectionid.Account
)

// section is one category of the settings window: its title, its glyph in the sidebar and the words the search field finds it by besides its title.
type section struct {
	id, title string
	icon      *ui.SVG
	keywords  string
}

var sections = []section{
	{SectionGeneral, "通用", iconGeneral, "资料库 位置 访达 界面语言 版本 组件 yt-dlp ffmpeg whisper-cli 重新检测"},
	{SectionSpeech, "语音识别", iconSpeech, "whisper 模型 下载 识别 HuggingFace Vimpo hyperframes"},
	{SectionAI, "AI 服务", iconAI, "翻译 服务 EveryAPI OpenAI OpenRouter Groq DeepSeek xAI Mistral 通义千问 Kimi 智谱 硅基流动 Ollama LM Studio Base URL API Key 模型 测试连接"},
	{SectionTranslate, "翻译", iconTranslate, "目标语言 智能翻译 上下文 提示词 恢复默认"},
	{SectionLook, "字幕外观", iconLook, "字幕 显示方式 双语 仅原文 仅译文 顺序 译文 原文 弱化 留空 让位 预览"},
	{SectionShortcuts, "快捷键", iconShortcuts, "键盘 按键 命令"},
	{SectionAccount, "账号与上传", iconAccount, "Google 登录 退出 对象存储 上传 R2 S3 OSS COS Endpoint Region Bucket Access Key Secret 路径样式"},
}

// Known reports whether id is one of the Section values.
func Known(id string) bool {
	for _, s := range sections {
		if s.id == id {
			return true
		}
	}
	return false
}

// title is the heading of section id.
func title(id string) string {
	for _, s := range sections {
		if s.id == id {
			return s.title
		}
	}
	return ""
}

// matches reports whether the search query finds s, by its title or its keywords, ignoring case. An empty query finds every section.
func (s section) matches(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s.title+" "+s.keywords), q)
}
