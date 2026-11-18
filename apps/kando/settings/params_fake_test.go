package settings

import "kando/packages/media"

// The fake's reads and writes of the settings this file's pages add: the AI requests, the context lines, the speech parameters, the appearance and the shortcut preset.

func (f *fakeApp) LLMBatchSize() int {
	if f.batchSize == 0 {
		return media.DefaultBatchSize
	}
	return f.batchSize
}

func (f *fakeApp) LLMRetries() int {
	if f.retries == nil {
		return media.DefaultRetries
	}
	return *f.retries
}

func (f *fakeApp) SetLLMRequests(size, retries int) { f.batchSize, f.retries = size, &retries }

func (f *fakeApp) TranslateContextLines() int {
	if f.contextLines == 0 {
		return media.DefaultContextLines
	}
	return f.contextLines
}

func (f *fakeApp) SetTranslateContextLines(n int) { f.contextLines = n }

func (f *fakeApp) Appearance() string { return f.appearance }

func (f *fakeApp) SetAppearance(appearance string) { f.appearance = appearance }

func (f *fakeApp) SpeechThreads() int { return f.tools.WhisperThreads }

func (f *fakeApp) SetSpeechThreads(n int) { f.tools.WhisperThreads = n }

func (f *fakeApp) SpeechHints() string { return f.tools.WhisperHints }

func (f *fakeApp) SetSpeechHints(hints string) { f.tools.WhisperHints = hints }

func (f *fakeApp) SpeechCommand() []string {
	return append([]string{"whisper-cli"}, f.tools.WhisperArgs("auto", "transcript", "audio.wav")...)
}

func (f *fakeApp) ShortcutPreset() string { return f.preset }

func (f *fakeApp) SetShortcutPreset(preset string) { f.preset = preset }

// Review is on until a test turns it off, as the app's is.
func (f *fakeApp) Review() *bool {
	if f.review == nil {
		on := true
		f.review = &on
	}
	return f.review
}

func (f *fakeApp) ReviewAuto() string { return f.reviewAuto }

func (f *fakeApp) SetReviewAuto(auto string) { f.reviewAuto = auto }
