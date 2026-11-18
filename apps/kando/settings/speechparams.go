package settings

import (
	"strconv"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// speechParams is the 语音识别 page's 识别参数 section: how many threads whisper recognizes speech with, the words it should spell as written, and the whisper-cli command a job runs with them, which follows them as they change.
func speechParams(cx *core.Scope, a App) core.Node {
	choices := append([]int{0}, media.WhisperThreadChoices...)
	options := make([]ui.Option[int], len(choices))
	for i, n := range choices {
		label := strconv.Itoa(n)
		if n == 0 {
			label = locales.SettingsSpeechThreadsAuto(cx, media.WhisperThreadsAuto())
		}
		options[i] = ui.Option[int]{Value: n, Label: label}
	}
	threads := ui.Select[int]{Options: options, Label: locales.SettingsSpeechThreads(cx), TestID: "settings-speech-threads",
		Value: live.Func(a, a.SpeechThreads), OnChange: live.On(a, a.SetSpeechThreads), Style: core.Style{Width: core.Pt(160)}}
	hints := ui.Input{TestID: "settings-speech-hints", Multiline: true, Label: locales.SettingsSpeechHints(cx), Placeholder: locales.SettingsSpeechHintsPlaceholder(cx),
		Value: live.Func(a, a.SpeechHints), OnChange: live.On(a, a.SetSpeechHints),
		Style: core.Style{Height: core.Pt(64)}}
	command := core.Text{TestID: "settings-speech-command", Selectable: core.SelectableOn,
		ContentFunc: live.Func(a, func() string { return CommandLine(a.SpeechCommand()) }),
		Style:       core.Style{FontSize: 11, LineHeight: 16, FontFamily: "monospace"}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
	pad := func(children ...core.Node) core.Node {
		return core.View{Style: core.Style{Padding: core.Edges{Top: 10, Right: 14, Bottom: 12, Left: 14}, Gap: 6, AlignItems: core.AlignStretch}, Children: children}
	}
	colors := ui.ColorsOf(cx)
	caption := func(text string) core.Node {
		return core.Text{Content: text, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)}
	}
	return core.Group([]core.Node{
		form.Label(locales.SettingsSpeechParams(cx), 14),
		form.Card(
			form.Row{Title: locales.SettingsSpeechThreads(cx), Subtitle: locales.SettingsSpeechThreadsHint(cx), NoGroup: true, Trailing: []core.Node{threads}},
			form.Separator(true),
			pad(caption(locales.SettingsSpeechHints(cx)), hints, core.Text{Content: locales.SettingsSpeechHintsNote(cx), Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)}),
			form.Separator(true),
			pad(caption(locales.SettingsSpeechCommand(cx)), command),
		),
	})
}

// CommandLine writes a program and its arguments as a shell shows a command: an argument that is empty or holds a space or a quote in double quotes, its own double quotes and backslashes escaped.
func CommandLine(args []string) string {
	out := make([]string, len(args))
	for i, arg := range args {
		if arg != "" && !strings.ContainsAny(arg, " \t\n\"'\\") {
			out[i] = arg
			continue
		}
		out[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(arg) + `"`
	}
	return strings.Join(out, " ")
}
