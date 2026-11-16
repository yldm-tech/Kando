package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/onboarding"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// newFirstRunApp is the app as main starts it on a Mac it has never run on: a home of its own with nothing in it, no library, settings.json where main keeps it, and the settings restored from that missing file.
func newFirstRunApp(t *testing.T, p media.Pipeline) (*app, *nsgotest.App, chan func(), string) {
	t.Helper()
	return newFirstRunAppAt(t, p, t.TempDir())
}

// newFirstRunAppAt is newFirstRunApp with home, which may already hold folders, such as libraries from before.
func newFirstRunAppAt(t *testing.T, p media.Pipeline, home string) (*app, *nsgotest.App, chan func(), string) {
	t.Helper()
	onboardingHome(t, home)
	t.Setenv("KANDO_OUTPUT_DIR", "")
	a, tt, updates := newTestApp(t, p)
	a.edit(func(s *session) { s.LibraryDir = "" })
	a.settingsPath = filepath.Join(home, "Library", "Application Support", "Kando", "settings.json")
	a.prefs.modelsDir = filepath.Join(filepath.Dir(a.settingsPath), "models")
	a.restoreSettings(a.loadSettings())
	a.onboardingReady()
	refresh(tt, a)
	return a, tt, updates, home
}

// relaunch is the app main starts the next time with the same home: a fresh app that reads settings.json from where a saved it, as main does, and opens its window (onboardingReady).
func relaunch(t *testing.T, a *app) (*app, *nsgotest.App) {
	t.Helper()
	b, btt, _ := newTestApp(t, &fakePipeline{})
	b.edit(func(s *session) { s.LibraryDir = "" })
	b.settingsPath = a.settingsPath
	b.prefs.modelsDir = a.prefs.modelsDir
	b.restoreSettings(b.loadSettings())
	b.onboardingReady()
	refresh(btt, b)
	return b, btt
}

// onboardingHome makes home the user's home folder until t ends, so media.MoviesDir and the libraries the guide finds or makes are in it: HOME, and USERPROFILE, which os.UserHomeDir reads on Windows.
func onboardingHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func readSettingsFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// onboardingGuideGone lets the guide's sheet finish closing, as its closing motion keeps it on screen for a moment after it closed.
func onboardingGuideGone(tt *nsgotest.App) { tt.Advance(time.Second) }

// TestOnboardingShowsOnFirstLaunch checks that the guide opens on its first step when settings.json says nothing, instead of the page that asks for a library.
func TestOnboardingShowsOnFirstLaunch(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	if a.OnboardingStep() != 1 {
		t.Fatalf("step %d, want 1", a.OnboardingStep())
	}
	wantText(t, tt, "欢迎使用看懂")
	wantText(t, tt, "1 / 3")
	wantText(t, tt, "字幕翻译成")
	wantNoText(t, tt, "还没有资料库")
}

// TestOnboardingStaysClosedOnceSeen checks that a saved onboarded flag keeps the guide closed, and that someone who already has a library from before the guide existed is not walked through it either.
func TestOnboardingStaysClosedOnceSeen(t *testing.T) {
	for name, saved := range map[string]map[string]any{
		"onboarded":     {"onboarded": true},
		"has library":   {"libraryDir": "/Movies/old.kando"},
		"has libraries": {"libraries": []string{"/Movies/old.kando"}},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			data, _ := json.Marshal(saved)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			a, _, _ := newTestApp(t, &fakePipeline{})
			a.settingsPath = path
			a.restoreSettings(loadSavedSettings(path))
			if a.OnboardingStep() != 0 || !a.onb.done {
				t.Fatalf("step %d done %v, want closed and done", a.OnboardingStep(), a.onb.done)
			}
		})
	}
}

// TestOnboardingSkipCreatesTheLibrary checks D8: skipping the guide on a Mac without a library makes 我的影片.kando in ~/Movies (~/Videos off macOS), as 新建资料库 would, opens it, saves it and the onboarded flag, and leaves the library page on 从一段视频开始 with the library's name under the title and the note about 帮助 › 重新查看入门引导.
func TestOnboardingSkipCreatesTheLibrary(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	tt.Press("跳过")
	onboardingGuideGone(tt)
	want := filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt)
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("library folder: %v", err)
	}
	if got := a.snap().LibraryDir; got != want {
		t.Fatalf("open library %q, want %q", got, want)
	}
	if a.OnboardingStep() != 0 {
		t.Fatalf("guide still open at step %d", a.OnboardingStep())
	}
	saved := readSettingsFile(t, a.settingsPath)
	if saved["libraryDir"] != want || saved["onboarded"] != true {
		t.Fatalf("settings.json %v", saved)
	}
	wantText(t, tt, "从一段视频开始")
	wantText(t, tt, "我的影片")
	wantText(t, tt, a.onboardingTip())
	wantNoText(t, tt, "还没有资料库")
	wantNoText(t, tt, "欢迎使用看懂")

	// The library is remembered: the next launch opens it again and the guide stays closed.
	b, btt, _ := newTestApp(t, &fakePipeline{})
	b.edit(func(s *session) { s.LibraryDir = "" })
	b.settingsPath = a.settingsPath
	b.restoreSettings(loadSavedSettings(b.settingsPath))
	refresh(btt, b)
	if b.snap().LibraryDir != want || b.OnboardingStep() != 0 {
		t.Fatalf("next launch: library %q step %d", b.snap().LibraryDir, b.OnboardingStep())
	}
}

// TestOnboardingOpensAnExistingLibrary checks that a 我的影片.kando already in ~/Movies, from a run whose settings are gone, is opened as it is rather than refused as a name that is taken.
func TestOnboardingOpensAnExistingLibrary(t *testing.T) {
	home := t.TempDir()
	onboardingHome(t, home)
	dir := filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt)
	if err := os.MkdirAll(filepath.Join(dir, "talk"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.edit(func(s *session) { s.LibraryDir = "" })
	a.settingsPath = filepath.Join(home, "settings.json")
	a.restoreSettings(savedSettings{})
	a.onboardingReady()
	refresh(tt, a)
	tt.Press("跳过")
	if a.snap().LibraryDir != dir || a.saveErr != "" {
		t.Fatalf("library %q err %q", a.snap().LibraryDir, a.saveErr)
	}
	if _, err := os.Stat(filepath.Join(dir, "talk")); err != nil {
		t.Fatal("the library's contents were touched")
	}
}

// TestOnboardingSteps checks 继续, 上一步 and the step each shows, and that the language tiles are a setting that persists.
func TestOnboardingSteps(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	wantNoText(t, tt, "上一步")
	tt.Press("继续")
	wantText(t, tt, "2 / 3")
	wantText(t, tt, "下载组件和语音识别模型")
	tt.Press("上一步")
	wantText(t, tt, "1 / 3")
	tt.Press("Français")
	if !slices.Contains(a.OnboardingLearning(), "fr") {
		t.Fatalf("learning %q after choosing Français", a.OnboardingLearning())
	}
	tt.Press("English")
	saved := readSettingsFile(t, a.settingsPath)
	if got := fmt.Sprint(saved["learnLanguages"]); got != "[ja fr]" {
		t.Fatalf("learnLanguages %s, want [ja fr]", got)
	}
	tt.Press("继续")
	tt.Press("继续")
	wantText(t, tt, "3 / 3")
	wantText(t, tt, "粘贴第一个链接")
	wantText(t, tt, "先随便看看")
	tt.Press("完成")
	if a.OnboardingStep() != 0 || !a.onb.done {
		t.Fatalf("step %d done %v after 完成", a.OnboardingStep(), a.onb.done)
	}
}

// TestOnboardingLanguageGoesToTheJobs checks that the target language picked in the guide is the settings' one, the language jobs are translated into.
func TestOnboardingLanguageGoesToTheJobs(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	*a.Language() = "日本語"
	a.OnboardingSave()
	refresh(tt, a)
	// The tiles leave out the language subtitles are translated into.
	if textCount(tt, "日语") != 0 {
		t.Fatalf("the target's own tile is offered: %q", tt.Texts())
	}
	if saved := readSettingsFile(t, a.settingsPath); saved["language"] != "日本語" {
		t.Fatalf("language saved %v", saved["language"])
	}
	tt.Press("继续")
	tt.Press("继续")
	tt.Type("第一个视频的链接", "https://www.youtube.com/watch?v=k9Yt2LmQ0pX")
	tt.Press("加入并开始")
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Language != "日本語" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// TestOnboardingLinkIsEnqueued checks that the link typed on the last step goes to the queue of the library the guide opened, through Enqueue, and that the button says so.
func TestOnboardingLinkIsEnqueued(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	tt.Press("继续")
	tt.Press("继续")
	tt.Type("第一个视频的链接", "youtube.com/watch?v=k9Yt2LmQ0pX")
	wantText(t, tt, "识别为 YouTube 视频")
	tt.Press("加入并开始")
	if len(a.library.Jobs) != 1 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	job := a.library.Jobs[0]
	if job.Source != "https://youtube.com/watch?v=k9Yt2LmQ0pX" || job.LibraryDir != filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt) {
		t.Fatalf("job %+v", job)
	}
	if a.OnboardingStep() != 0 {
		t.Fatal("guide still open")
	}
}

// TestOnboardingRefusesWhatIsNotAVideo checks that a link that is not a video, or names a whole channel, cannot be added: the hint says why, the button stays off and Return does nothing.
func TestOnboardingRefusesWhatIsNotAVideo(t *testing.T) {
	for _, link := range []string{"not a link", "https://www.youtube.com/@SomeChannel"} {
		t.Run(link, func(t *testing.T) {
			a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
			a.onb.step = 3
			*a.OnboardingLink() = link
			refresh(tt, a)
			if !tt.Find("加入并开始").Props.Disabled {
				t.Fatal("加入并开始 is on for a link that cannot be added")
			}
			tt.Submit("第一个视频的链接")
			if a.OnboardingStep() != 3 || len(a.library.Jobs) != 0 {
				t.Fatalf("step %d jobs %d", a.OnboardingStep(), len(a.library.Jobs))
			}
			if hint, ok := onboarding.LinkHint(a.cx, link); ok || textCount(tt, hint) == 0 {
				t.Fatalf("hint %q ok %v, texts %q", hint, ok, tt.Texts())
			}
		})
	}
}

// TestOnboardingEscapeSkips checks that Escape closes the guide as 跳过 does.
func TestOnboardingEscapeSkips(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	tt.Key("escape")
	if a.OnboardingStep() != 0 || !a.onb.done || a.snap().LibraryDir == "" {
		t.Fatalf("step %d done %v library %q", a.OnboardingStep(), a.onb.done, a.snap().LibraryDir)
	}
}

// TestOnboardingTipIsDismissedForGood checks the note shown after the guide: it stays until its × closes it, also across a restart, and not after.
func TestOnboardingTipIsDismissedForGood(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	tt.Press("跳过")
	b, btt, _ := newTestApp(t, &fakePipeline{})
	b.settingsPath = a.settingsPath
	b.restoreSettings(loadSavedSettings(b.settingsPath))
	refresh(btt, b)
	wantText(t, btt, b.onboardingTip())
	btt.Press("关闭提示")
	wantNoText(t, btt, b.onboardingTip())
	c, ctt, _ := newTestApp(t, &fakePipeline{})
	c.settingsPath = a.settingsPath
	c.restoreSettings(loadSavedSettings(c.settingsPath))
	refresh(ctt, c)
	wantNoText(t, ctt, c.onboardingTip())
}

// TestHelpMenuReopensOnboarding checks 帮助 › 重新查看入门引导: it opens the guide at its first step with the link field empty, and finishing it again keeps the library that is open.
func TestHelpMenuReopensOnboarding(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.onb.done = true
	a.onb.link = "https://example.com/old"
	refresh(tt, a)
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), "帮助", "重新查看入门引导"))
	if a.OnboardingStep() != 1 || a.onb.link != "" {
		t.Fatalf("step %d link %q", a.OnboardingStep(), a.onb.link)
	}
	wantText(t, tt, "欢迎使用看懂")
	if !a.modalOpen() {
		t.Fatal("the guide does not hold the menu bar's commands while it is open")
	}
	tt.Press("跳过")
	if a.modalOpen() {
		t.Fatal("the guide still holds the menu bar's commands once closed")
	}
	if a.snap().LibraryDir != "/videos" {
		t.Fatalf("library %q", a.snap().LibraryDir)
	}
}

// TestOnboardingRecommendsModels checks the second step's models: Large v3 Turbo (compressed) and Small for most languages, Base first when only English was chosen, and a model already in use listed as such.
func TestOnboardingRecommendsModels(t *testing.T) {
	a, _, _, _ := newFirstRunApp(t, &fakePipeline{})
	ids := func() []string {
		var out []string
		for _, m := range a.OnboardingModels(a.cx) {
			out = append(out, m.ID)
		}
		return out
	}
	if got := ids(); !slices.Equal(got, []string{"large-v3-turbo-q5", "small"}) {
		t.Fatalf("models %q", got)
	}
	a.onb.learning = []string{"en"}
	if got := ids(); !slices.Equal(got, []string{"base", "large-v3-turbo-q5"}) {
		t.Fatalf("English only: models %q", got)
	}
}

// toolsInstalled is a pipeline that has yt-dlp, ffmpeg and whisper-cli, so the guide's second step shows its card of programs as one ready line and the models below it in view: the tests of the model downloads click on them, and with the card's three rows of programs Kando cannot download (no OwnToolsDir in tests) the models start below the step's visible area where its text is taller, as in Linux CI's fonts.
var toolsInstalled = media.Tools{YTDLP: "/usr/local/bin/yt-dlp", FFmpeg: "/usr/local/bin/ffmpeg", Whisper: "/usr/local/bin/whisper-cli"}

// fakeModelServer serves a whisper model file of size bytes, holding the body back until release is closed when it is not nil.
func fakeModelServer(t *testing.T, size int, release chan struct{}) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(size))
		half := strings.Repeat("x", size/2)
		_, _ = w.Write([]byte(half))
		w.(http.Flusher).Flush()
		if release != nil {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = w.Write([]byte(strings.Repeat("x", size-len(half))))
	}))
	t.Cleanup(srv.Close)
	// A test that fails while a download waits on the held-back body leaves the download's goroutine blocked on the test's updates, so the request would never end and Close would wait for it until the test binary times out, hiding the failure: the connections are cut first, which ends the handler.
	t.Cleanup(srv.CloseClientConnections)
	serveSpeechModels(t, srv.URL, []byte(strings.Repeat("x", size)), nil)
	return srv
}

// TestOnboardingDownloadsTheModel checks 下载并继续: it downloads the recommended model into the models folder for real, shows its progress, goes on to the next step while it runs, and once it has arrived the pipeline uses it and settings.json says so.
func TestOnboardingDownloadsTheModel(t *testing.T) {
	release := make(chan struct{})
	fakeModelServer(t, 4096, release)
	a, tt, updates, _ := newFirstRunApp(t, toolsInstalled)
	tt.Press("继续")
	wantText(t, tt, "下载并继续")
	tt.Press("下载并继续")
	if a.OnboardingStep() != 3 || !a.OnboardingDownloading() {
		t.Fatalf("step %d downloading %v", a.OnboardingStep(), a.OnboardingDownloading())
	}
	waitUntil(t, tt, a, updates, func() bool { return a.prefs.downloadWritten > 0 })
	tt.Press("上一步")
	wantText(t, tt, "下载中 50%")
	close(release)
	waitUntil(t, tt, a, updates, func() bool { return !a.OnboardingDownloading() })
	want := filepath.Join(a.prefs.modelsDir, "ggml-large-v3-turbo-q5_0.bin")
	if info, err := os.Stat(want); err != nil || info.Size() != 4096 {
		t.Fatalf("model file: %v", err)
	}
	if got := a.pipeline.(media.Tools).WhisperModel; got != want {
		t.Fatalf("pipeline model %q, want %q", got, want)
	}
	if saved := readSettingsFile(t, a.settingsPath); saved["whisperModel"] != want {
		t.Fatalf("whisperModel saved %v", saved["whisperModel"])
	}
	wantText(t, tt, "已下载 · 正在使用")
	wantText(t, tt, "继续")
	wantNoText(t, tt, "下载并继续")
}

// TestOnboardingCancelsTheDownload checks 取消 on a model being downloaded: the download stops and leaves nothing behind, and the model can be downloaded again.
func TestOnboardingCancelsTheDownload(t *testing.T) {
	fakeModelServer(t, 4096, make(chan struct{}))
	a, tt, updates, _ := newFirstRunApp(t, toolsInstalled)
	tt.Press("继续")
	a.OnboardingDownload("small")
	waitUntil(t, tt, a, updates, func() bool { return a.prefs.downloadWritten > 0 })
	tt.Press("取消")
	waitUntil(t, tt, a, updates, func() bool { return !a.OnboardingDownloading() })
	entries, _ := os.ReadDir(a.prefs.modelsDir)
	if len(entries) != 0 {
		t.Fatalf("models folder holds %v after 取消", entries)
	}
	if a.pipeline.(media.Tools).WhisperModel != "" {
		t.Fatal("a canceled download was put to use")
	}
	wantText(t, tt, "下载并继续")
}

// TestOnboardingSettingsPartIsItsOwn checks that the guide keeps its settings in its own part of settings.json.
func TestOnboardingSettingsPartIsItsOwn(t *testing.T) {
	var saved savedSettings
	saved.savedOnboarding = savedOnboarding{Onboarded: new(true), LearnLanguages: []string{" en ", "", "xx"}, OnboardingTip: true}
	saved.clean()
	if !slices.Equal(saved.LearnLanguages, []string{"en"}) {
		t.Fatalf("cleaned learnLanguages %q", saved.LearnLanguages)
	}
}

// TestOnboardingAIStatus checks what the second step says about translation: the API Key of a cloud service, a model to choose, the everyapi program, its sign-in or a key when one is missing, nothing claimed before the sign-in was checked, and the model once it is ready.
func TestOnboardingAIStatus(t *testing.T) {
	signIn := func(context.Context, bool) (media.Endpoint, error) { return media.Endpoint{}, nil }
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		name       string
		translator media.ChatTranslator
		checked    bool
		signInErr  error
		want       onboarding.AI
	}{
		{"no model", media.ChatTranslator{BaseURL: "https://api.openai.com/v1", APIKey: "k"}, false, nil, onboarding.AI{Text: msgs.PrecheckNoLLMModel(), Missing: true}},
		{"no key", media.ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt"}, false, nil, onboarding.AI{Text: msgs.OnboardingAINeedsKey(), Missing: true}},
		{"no key on this computer", media.ChatTranslator{BaseURL: "http://127.0.0.1:11434/v1", Model: "llama"}, false, nil, onboarding.AI{Text: "翻译模型：llama", Ready: true}},
		{"everyapi without key or program", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m"}, false, nil, onboarding.AI{Text: msgs.PrecheckEveryAPIMissing(), Missing: true}},
		{"signed out", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", Credential: signIn}, true, media.ErrEveryAPISignedOut, onboarding.AI{Text: msgs.PrecheckEveryAPISignedOut(), Missing: true}},
		{"program gone since found", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", Credential: signIn}, true, media.ErrEveryAPIMissing, onboarding.AI{Text: msgs.PrecheckEveryAPIMissing(), Missing: true}},
		{"sign-in not checked", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", Credential: signIn}, false, nil, onboarding.AI{Text: "翻译模型：m · 用本机登录的 EveryAPI，开始处理前会检查登录"}},
		{"signed in", media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: "m", Credential: signIn}, true, nil, onboarding.AI{Text: "翻译模型：m", Ready: true}},
		{"key", media.ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt", APIKey: "k"}, false, nil, onboarding.AI{Text: "翻译模型：gpt", Ready: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newTestApp(t, media.Tools{Translator: tc.translator})
			a.prefs.llmProvider = settings.ProviderID(tc.translator.BaseURL)
			a.side.ready.signInDone, a.side.ready.signInErr = tc.checked, tc.signInErr
			if got := a.OnboardingAI(a.cx); got != tc.want {
				t.Fatalf("OnboardingAI = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestOnboardingSetsUpTranslation checks that the guide's second step makes translation work without the settings: EveryAPI without its everyapi program says so and offers to get it; a cloud service chosen there takes its key in place, after which its model list is fetched and a model chosen from it, all saved; and 测试连接 shows its result there.
func TestOnboardingSetsUpTranslation(t *testing.T) {
	llmEnv(t, "", "", "")
	tools := toolsInstalled
	tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
	a, tt, updates, _ := newFirstRunApp(t, tools)
	a.prefs.listModels = func(_ context.Context, tr media.ChatTranslator) ([]string, error) {
		if tr.APIKey != "gsk-test" {
			return nil, errors.New("translation API returned 401 Unauthorized")
		}
		return []string{"whisper-large-v3", "llama-3.3-70b-versatile"}, nil
	}
	tt.Press("继续")
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.PrecheckEveryAPIMissing())
	if !tt.Exists("id:onboarding-everyapi-get") || tt.Exists("id:onboarding-ai-key") {
		t.Fatalf("EveryAPI without its program: %q", tt.Texts())
	}

	tt.Press("id:onboarding-ai-service")
	tt.Press(`text:"Groq"`)
	refresh(tt, a)
	if a.prefs.llmProvider != "groq" || !tt.Exists("id:onboarding-ai-get-key") {
		t.Fatalf("provider %q, texts %q", a.prefs.llmProvider, tt.Texts())
	}
	wantText(t, tt, msgs.OnboardingAINeedsKey())
	tt.Focus("id:onboarding-ai-key")
	tt.Type("id:onboarding-ai-key", "gsk-test")
	waitUntil(t, tt, a, updates, func() bool { return a.prefs.llmModel != "" })
	refresh(tt, a)
	if got := translator(t, a); got.APIKey != "gsk-test" || got.Model != "llama-3.3-70b-versatile" {
		t.Fatalf("translator %+v", got)
	}
	if saved := loadSavedSettings(a.settingsPath); saved.LLMAPIKey != "gsk-test" || saved.LLMModel != "llama-3.3-70b-versatile" {
		t.Fatalf("saved key %q model %q", saved.LLMAPIKey, saved.LLMModel)
	}
	wantText(t, tt, "翻译模型：llama-3.3-70b-versatile")
	tt.Press("id:onboarding-ai-get-key")
	if want := []string{"https://console.groq.com/keys"}; !slices.Equal(tt.OpenedURLs, want) {
		t.Fatalf("opened %q, want %q", tt.OpenedURLs, want)
	}

	server := chatServer(t, func() (int, string) { return http.StatusOK, "" }, nil)
	apply(tt, a, func() {
		a.prefs.llmProvider, a.prefs.llmBase = "custom", server.URL
		a.applyLLM()
	})
	tt.Press("id:onboarding-ai-test")
	waitUntil(t, tt, a, updates, func() bool { return !a.prefs.testingLLM })
	refresh(tt, a)
	wantText(t, tt, msgs.SettingsAIConnected())
}

// TestOnboardingReopensAfterQuitMidGuide checks that quitting while the guide is still open does not count as having seen it: the library the guide made under it is saved with onboarded false, and the next launch opens that library again with the guide back on its first step.
func TestOnboardingReopensAfterQuitMidGuide(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	tt.Press("继续")
	tt.Press("继续")
	want := filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt)
	saved := readSettingsFile(t, a.settingsPath)
	if saved["libraryDir"] != want || saved["onboarded"] != false {
		t.Fatalf("settings.json while the guide is open: %v", saved)
	}
	b, btt := relaunch(t, a)
	if b.OnboardingStep() != 1 || b.onb.done {
		t.Fatalf("next launch: step %d done %v, want the guide on step 1", b.OnboardingStep(), b.onb.done)
	}
	if b.snap().LibraryDir != want || b.saveErr != "" {
		t.Fatalf("next launch: library %q err %q", b.snap().LibraryDir, b.saveErr)
	}
	wantText(t, btt, "欢迎使用看懂")
	btt.Press("跳过")
	if saved := readSettingsFile(t, a.settingsPath); saved["onboarded"] != true {
		t.Fatalf("settings.json after 跳过: %v", saved)
	}
	c, _ := relaunch(t, a)
	if c.OnboardingStep() != 0 {
		t.Fatalf("after 跳过 the guide opened again at step %d", c.OnboardingStep())
	}
}

// TestOnboardingLeavesADamagedSettingsFileAlone checks a settings.json that cannot be read in full, such as one with a field of another type from another version, or one cut short: startup neither opens the guide nor makes a library nor writes the file, keeps a copy of it as settings.json.bad, says so, and keeps every field that could be read.
func TestOnboardingLeavesADamagedSettingsFileAlone(t *testing.T) {
	for name, body := range map[string]string{
		"field of another type": `{"libraryDir":%q,"libraries":[%q],"llmModel":"keep-me","subMode":7}`,
		"cut short":             `{"libraryDir":%q,"libraries":[%q],"llmModel":"kee`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			onboardingHome(t, home)
			t.Setenv("KANDO_OUTPUT_DIR", "")
			old := filepath.Join(media.MoviesDir(), "影视库"+media.LibraryExt)
			if err := os.MkdirAll(old, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, "Library", "Application Support", "Kando", "settings.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			data := []byte(fmt.Sprintf(body, old, old))
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			a, tt, _ := newTestApp(t, &fakePipeline{})
			a.edit(func(s *session) { s.LibraryDir = "" })
			a.settingsPath = path
			saved := a.loadSettings()
			a.restoreSettings(saved)
			a.onboardingReady()
			refresh(tt, a)
			if a.OnboardingStep() != 0 {
				t.Fatalf("the guide opened at step %d over a damaged settings.json", a.OnboardingStep())
			}
			if got, _ := os.ReadFile(path); string(got) != string(data) {
				t.Fatalf("settings.json was rewritten at startup: %s", got)
			}
			if got, err := os.ReadFile(path + ".bad"); err != nil || string(got) != string(data) {
				t.Fatalf("settings.json.bad: %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt)); !os.IsNotExist(err) {
				t.Fatalf("a library was made: %v", err)
			}
			if !strings.Contains(a.saveErr, "settings.json.bad") {
				t.Fatalf("saveErr %q", a.saveErr)
			}
			if name == "field of another type" && (a.snap().LibraryDir != old || saved.LLMModel != "keep-me") {
				t.Fatalf("readable fields lost: library %q llmModel %q", a.snap().LibraryDir, saved.LLMModel)
			}
		})
	}
}

// TestOnboardingFindsTheLibrariesAlreadyThere checks a first run, without settings, on a Mac whose ~/Movies already holds .kando libraries, as after settings were lost: the guide lists them all and opens the one changed last instead of making an empty 我的影片.kando.
func TestOnboardingFindsTheLibrariesAlreadyThere(t *testing.T) {
	home := t.TempDir()
	onboardingHome(t, home)
	older := filepath.Join(media.MoviesDir(), "影视库"+media.LibraryExt)
	newer := filepath.Join(media.MoviesDir(), "英语"+media.LibraryExt)
	for i, dir := range []string{older, newer} {
		if err := os.MkdirAll(filepath.Join(dir, "talk"), 0o755); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(time.Duration(i-2) * time.Hour)
		if err := os.Chtimes(dir, when, when); err != nil {
			t.Fatal(err)
		}
	}
	a, _, _, _ := newFirstRunAppAt(t, &fakePipeline{}, home)
	if a.snap().LibraryDir != newer || a.saveErr != "" {
		t.Fatalf("library %q err %q, want %q", a.snap().LibraryDir, a.saveErr, newer)
	}
	if got := a.knownLibraries(); !slices.Contains(got, older) || !slices.Contains(got, newer) || len(got) != 2 {
		t.Fatalf("libraries %q", got)
	}
	if _, err := os.Stat(filepath.Join(media.MoviesDir(), "我的影片"+media.LibraryExt)); !os.IsNotExist(err) {
		t.Fatalf("我的影片.kando was made beside them: %v", err)
	}
	if a.OnboardingStep() != 1 {
		t.Fatalf("step %d, want the guide open", a.OnboardingStep())
	}
}

// TestOnboardingReopensWithoutAMenuBar checks Linux and Windows, where the app has no menu bar of its own: the guide is reopened with F1, a row of the shortcut table that the cheat sheet and 设置 › 快捷键 list, and the note it leaves names that key rather than a 帮助 menu that is not there. With the menu bar the row is the menu's item alone.
func TestOnboardingReopensWithoutAMenuBar(t *testing.T) {
	keep := appMenuBar
	t.Cleanup(func() { appMenuBar = keep })

	appMenuBar = true
	withMenu := &app{}
	withMenu.prefs.interfaceLanguage = testlocale.Current()
	if rows := withMenu.onboardingKeys(nil); len(rows) != 0 {
		t.Fatalf("with a menu bar: rows %+v", rows)
	}
	if !strings.Contains(withMenu.onboardingTip(), "帮助 › 重新查看入门引导") {
		t.Fatalf("with a menu bar: tip %q", withMenu.onboardingTip())
	}

	appMenuBar = false
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.onb.done = true
	refresh(tt, a)
	rows := a.onboardingKeys(a.cx)
	if len(rows) != 1 || rows[0].Group != groupHelp || rows[0].Key != "f1" || rows[0].Label != "重新查看入门引导" {
		t.Fatalf("rows %+v", rows)
	}
	tt.Key("f1")
	if a.OnboardingStep() != 1 {
		t.Fatalf("F1: step %d", a.OnboardingStep())
	}
	tt.Press("跳过")
	if strings.Contains(a.onboardingTip(), "帮助") || !strings.Contains(a.onboardingTip(), "F1") {
		t.Fatalf("without a menu bar: tip %q", a.onboardingTip())
	}
	wantText(t, tt, a.onboardingTip())
}

// TestOnboardingTipWaitsBehindTheFinishMessage checks the note after the guide finishes with a link: 已加入，开始处理。 shows first, the note takes the message bar once that is closed, and it is gone for good once its own × has closed it, also on the next launch.
func TestOnboardingTipWaitsBehindTheFinishMessage(t *testing.T) {
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	tt.Press("继续")
	tt.Press("继续")
	tt.Type("第一个视频的链接", "https://www.youtube.com/watch?v=k9Yt2LmQ0pX")
	tt.Press("加入并开始")
	wantText(t, tt, "已加入，开始处理。")
	wantNoText(t, tt, a.onboardingTip())
	if saved := readSettingsFile(t, a.settingsPath); saved["onboardingTip"] != true {
		t.Fatalf("settings.json %v", saved)
	}
	tt.Press("关闭提示")
	wantText(t, tt, a.onboardingTip())
	tt.Press("关闭提示")
	wantNoText(t, tt, a.onboardingTip())
	b, btt := relaunch(t, a)
	wantNoText(t, btt, b.onboardingTip())
}

// TestOnboardingInEnglish walks the guide in English: each step's title, buttons and lines, what the link field reads a link as, and the note it leaves in the library once it is closed.
func TestOnboardingInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _, _ := newFirstRunApp(t, &fakePipeline{})
	for _, want := range []string{"Welcome to Kando", "1 / 3", "Skip", "Translate subtitles into", "Languages of the videos you watch · Choose any", "日本語", "Japanese", "Continue"} {
		wantText(t, tt, want)
	}
	for _, chinese := range []string{"欢迎使用看懂", "字幕翻译成", "日语", "继续", "跳过"} {
		wantNoText(t, tt, chinese)
	}
	tt.Press("Continue")
	for _, want := range []string{"2 / 3", "Download Components and a Speech Model", "Translation · AI Service", "Set up", "Settings…", "Back"} {
		wantText(t, tt, want)
	}
	tt.Press("Continue")
	for _, want := range []string{"Paste Your First Link", "Just Look Around", "Done", "Press Enter to start. No link? Skip for now and drag a local video into the window later."} {
		wantText(t, tt, want)
	}
	*a.OnboardingLink() = "https://www.youtube.com/watch?v=k9Yt2LmQ0pX&list=PL123"
	refresh(tt, a)
	wantText(t, tt, "Recognized as a YouTube video. Only this one is added, not the whole playlist")
	wantText(t, tt, "Add and Start")
	*a.OnboardingLink() = ""
	refresh(tt, a)
	tt.Press("Just Look Around")
	onboardingGuideGone(tt)
	tip := a.onboardingTip()
	if !strings.Contains(tip, "come back here") {
		t.Fatalf("tip %q is not English", tip)
	}
	wantText(t, tt, tip)
}

// TestOnboardingTipKnownInEveryLanguage checks that the guide's note is still known as the guide's after the language changed under it, so closing it forgets it rather than showing it again.
func TestOnboardingTipKnownInEveryLanguage(t *testing.T) {
	for _, tag := range locales.Interface {
		if !isOnboardingTip(onboardingTipIn(locales.In(tag))) {
			t.Fatalf("the %s note is not known", tag)
		}
	}
	if isOnboardingTip("已加入，开始处理。") {
		t.Fatal("another message counts as the note")
	}
}
