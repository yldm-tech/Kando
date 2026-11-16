package media

import (
	"context"
	"strings"
	"time"

	"kando/packages/core"
)

// Info is what the site says about a video once the download has looked it up.
type Info struct {
	Title     string
	Thumbnail string
	Duration  time.Duration
}

// Trace receives what a pipeline learns while a stage runs, as net/http/httptrace's ClientTrace receives what a request does: the Pipeline methods keep their signatures, and a caller that wants to follow a stage puts a Trace in the context it passes. Either field may be nil. A stage calls them on the goroutine that called it, before it returns.
type Trace struct {
	// Progress is how far the stage has gone.
	Progress func(core.Progress)
	// Info is the video's title and thumbnail, once the download knows them.
	Info func(Info)
	// Spoken is the language the transcription heard, as whisper's code (ja, en), once it knows it.
	Spoken func(language string)
}

type traceKey struct{}

// WithTrace returns ctx carrying trace, which Tools' stages report to.
func WithTrace(ctx context.Context, trace *Trace) context.Context {
	return context.WithValue(ctx, traceKey{}, trace)
}

func traceOf(ctx context.Context) *Trace {
	trace, _ := ctx.Value(traceKey{}).(*Trace)
	return trace
}

func reportProgress(ctx context.Context, p core.Progress) {
	if trace := traceOf(ctx); trace != nil && trace.Progress != nil {
		trace.Progress(p)
	}
}

func reportInfo(ctx context.Context, info Info) {
	if trace := traceOf(ctx); trace != nil && trace.Info != nil {
		trace.Info(info)
	}
}

func reportSpoken(ctx context.Context, language string) {
	if trace := traceOf(ctx); trace != nil && trace.Spoken != nil && language != "" {
		trace.Spoken(language)
	}
}

type speechLanguageKey struct{}

// withSpeechLanguage returns ctx telling Tools.Transcribe the language spoken in the video, as whisper's code, in place of Tools.SpeechLanguage: the language the user said a film is spoken in (core.Job.SpeechLanguage).
func withSpeechLanguage(ctx context.Context, code string) context.Context {
	return context.WithValue(ctx, speechLanguageKey{}, code)
}

// speechLanguageOf is the language withSpeechLanguage put in ctx, or "".
func speechLanguageOf(ctx context.Context) string {
	code, _ := ctx.Value(speechLanguageKey{}).(string)
	return strings.TrimSpace(code)
}
