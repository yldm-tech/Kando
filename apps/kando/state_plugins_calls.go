package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/saved"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The host calls of a script plugin that wait for something outside the engine (docs/plugin-system.md 5.3): kando.llm.chat asks the user's AI service within the plugin's daily budget of tokens, kando.sound.read cuts a line's sound with ffmpeg, and kando.net.credentialFetch sends a request with a secret Kando keeps for the plugin, which its code never sees. Each runs off the engine's goroutine and answers through update; while it runs, the watchdog of the plugin's call waiting for it is paused (script.Runner.Pause), as the time is Kando's, not the plugin's. Each fails with a code alone, never the app's own words or the service's.

// The limits of these calls: how long Kando waits for the AI service or a credential's request before it gives up (the plugin is told to wait longer, docs/plugin-system.md 3.3, so it hears Kando's answer rather than its own timeout); the most characters of an instruction and a message sent to the AI service, and of the reply it may ask for; the most bytes of a line's sound; and, for a credential's request, the most headers the plugin may add, the bytes of its body and of the reply's.
const (
	pluginsCallTimeout   = 120 * time.Second
	pluginsChatSystemMax = 8000
	pluginsChatUserMax   = 32000
	pluginsChatTokens    = 1024
	pluginsSoundMax      = 2 << 20
	pluginsFetchHeaders  = 16
	pluginsFetchBodyMax  = 1 << 20
	pluginsFetchReplyMax = 4 << 20
)

// The codes these calls fail with, besides pluginsDenied, pluginsQuota, pluginsBad and pluginsBusy.
const (
	pluginsUnavailable = "unavailable"
	pluginsOverBudget  = "budget"
	pluginsCutOff      = "cut-off"
	pluginsTimedOut    = "timeout"
	pluginsFailed      = "failed"
	pluginsNoFFmpeg    = "no-ffmpeg"
	pluginsNotFound    = "not-found"
	pluginsTooLarge    = "too-large"
	pluginsNoSecret    = "no-credential"
)

// pluginsWaiting runs work off the engine's goroutine for the plugin l's call c, one at a time per plugin and kind (key), with the watchdog of the call running paused meanwhile, and answers on the engine's goroutine with what done makes of its result; a second while the first runs fails with pluginsBusy.
func pluginsWaiting[T any](a *app, l *pluginsScript, c *ns.HostCall, key string, work func(ctx context.Context) (T, error), done func(v T, err error)) {
	s := &a.plugins.script
	if s.waiting == nil {
		s.waiting = map[string]bool{}
	}
	key = l.pack.ID + "\x00" + key
	if s.waiting[key] {
		pluginsRefuse(c, pluginsBusy)
		return
	}
	s.waiting[key] = true
	resume := a.pluginsRunner().PauseFor(l.plugin)
	aiAsk(a, pluginsCallTimeout, work, func() bool { return true }, func(v T, err error) {
		delete(s.waiting, key)
		resume()
		done(v, err)
		a.Changed()
	})
}

// pluginsString is the string field name of what a call sent, "" when it sent none.
func pluginsString(c *ns.HostCall, name string) string {
	m, _ := c.Data.(map[string]any)
	s, _ := m[name].(string)
	return s
}

// pluginsLLMChat answers kando.llm.chat: {system, user, maxTokens} asked of the user's AI service, as the command palette asks it, so not while Kando is offline for a cloud service, nor without one (paletteTranslator); within the plugin's daily budget of tokens (plugins.State.Budget), which what the request may take must fit; with X-Title saying which plugin asked. It answers {text, model, tokens}, never the service's address or key; the tokens are counted against the budget whether or not the reply came, as the service may bill them, and written to the audit log.
func (a *app) pluginsLLMChat(l *pluginsScript, c *ns.HostCall) {
	system, user := pluginsString(c, "system"), pluginsString(c, "user")
	if strings.TrimSpace(user) == "" || utf8.RuneCountInString(system) > pluginsChatSystemMax || utf8.RuneCountInString(user) > pluginsChatUserMax {
		pluginsRefuse(c, pluginsBad)
		return
	}
	t, why := a.paletteTranslator(locales.PluginsTaskNoAI)
	if why != nil {
		pluginsRefuse(c, pluginsUnavailable)
		return
	}
	t.Pass = media.PassPlugin
	id := l.pack.ID
	tokens := pluginsArg(c, "maxTokens", pluginsChatTokens, plugins.MaxTaskTokens)
	if need := media.EstimateTokens(system) + media.EstimateTokens(user) + tokens; need > a.plugins.state.Budget(id)-a.pluginsSpent(id) {
		pluginsRefuse(c, pluginsOverBudget)
		return
	}
	req := media.ChatRequest{System: system, User: user, MaxTokens: tokens, Header: http.Header{"X-Title": {"Kando plugin " + id}}}
	model := strings.TrimSpace(t.Model)
	pluginsWaiting(a, l, c, "llm", func(ctx context.Context) (media.ChatReply, error) { return t.ChatWith(ctx, req) }, func(reply media.ChatReply, err error) {
		spent := reply.Usage.Total()
		a.pluginsSpend(id, spent)
		a.pluginsAudit(plugins.AuditLLM, id, "tokens "+strconv.Itoa(spent))
		switch {
		case errors.Is(err, media.ErrReplyCutOff):
			pluginsRefuse(c, pluginsCutOff)
		case errors.Is(err, context.DeadlineExceeded):
			pluginsRefuse(c, pluginsTimedOut)
		case err != nil:
			pluginsRefuse(c, pluginsFailed)
		default:
			c.Resolve(map[string]any{"text": reply.Text, "model": model, "tokens": spent})
		}
	})
}

// pluginsSoundRead answers kando.sound.read: the sound of the saved line {saved: id}, or of the line playing {current: true}, cut out of its video as the Anki export cuts it (saved.SoundFor), as {mime, base64, durationMs}, at most pluginsSoundMax bytes. Without ffmpeg it fails with pluginsNoFFmpeg rather than send a sound kept from before for one line and none for the next.
func (a *app) pluginsSoundRead(l *pluginsScript, c *ns.HostCall) {
	ffmpeg := a.ffmpegPath()
	if ffmpeg == "" {
		pluginsRefuse(c, pluginsNoFFmpeg)
		return
	}
	book := a.savedBook()
	var line saved.Line
	var delay time.Duration
	m, _ := c.Data.(map[string]any)
	if id, ok := m["saved"].(string); ok && id != "" {
		i := slices.IndexFunc(book.Lines, func(x saved.Line) bool { return x.ID == id })
		if i < 0 {
			pluginsRefuse(c, pluginsNotFound)
			return
		}
		line = book.Lines[i]
		delay = a.player.delays[book.Abs(line)]
	} else if current, _ := m["current"].(bool); current {
		i := a.pluginsCurrentLine()
		if i < 0 || a.player.film.video == "" {
			pluginsRefuse(c, pluginsNotFound)
			return
		}
		cue := a.playerCues()[i]
		line, delay = saved.Line{Video: book.Key(a.player.film.video), Start: cue.Start, End: cue.End}, a.player.film.delay
	} else {
		pluginsRefuse(c, pluginsBad)
		return
	}
	type clip struct {
		data []byte
		ms   int64
	}
	streams := a.linkedStreams()
	pluginsWaiting(a, l, c, "sound", func(ctx context.Context) (clip, error) {
		dir, err := os.MkdirTemp("", "kando-plugin-sound-*")
		if err != nil {
			return clip{}, err
		}
		defer os.RemoveAll(dir)
		path, err := saved.SoundFor(saved.WithStreams(ctx, streams), ffmpeg, book, line, delay, dir)
		if err != nil {
			return clip{}, err
		}
		if path == "" {
			return clip{}, errPluginsNoSound
		}
		info, err := os.Stat(path)
		if err != nil {
			return clip{}, err
		}
		if info.Size() > pluginsSoundMax {
			return clip{}, errPluginsTooLarge
		}
		data, err := os.ReadFile(path)
		from, to := saved.ClipSpan(line, delay)
		return clip{data, (to - from).Milliseconds()}, err
	}, func(got clip, err error) {
		switch {
		case errors.Is(err, errPluginsTooLarge):
			pluginsRefuse(c, pluginsTooLarge)
		case errors.Is(err, errPluginsNoSound):
			pluginsRefuse(c, pluginsNotFound)
		case errors.Is(err, context.DeadlineExceeded):
			pluginsRefuse(c, pluginsTimedOut)
		case err != nil:
			pluginsRefuse(c, pluginsFailed)
		default:
			c.Resolve(map[string]any{"mime": "audio/mp4", "base64": base64.StdEncoding.EncodeToString(got.data), "durationMs": got.ms})
		}
	})
}

// Why a sound was not read.
var (
	errPluginsNoSound  = errors.New("the line has no sound")
	errPluginsTooLarge = errors.New("the sound is too large")
)

// pluginsFetchReply is what a credential's request answered: its status, its Content-Type and its body.
type pluginsFetchReply struct {
	status      int
	contentType string
	body        []byte
}

// pluginsCredentialFetch answers kando.net.credentialFetch: {credential, method, url, header, body} sent with the secret the user gave Kando for the credential, which Kando puts in the credential's header (docs/plugin-system.md 6.3), and only when the credential is granted as the plugin declares it now and the request is within it: https, its host, one of its methods and a path under one of its prefixes, on the default port, with no user in the URL. The plugin's own headers may not set the credential's nor one Kando or the request sets (Host, Cookie and the like). Redirects are not followed; the reply keeps its status, its body, at most pluginsFetchReplyMax bytes, and Content-Type alone of its headers, Set-Cookie and every other dropped. It answers {status, header: {"Content-Type"}, body}.
func (a *app) pluginsCredentialFetch(l *pluginsScript, c *ns.HostCall) {
	cred, ok := a.pluginsEffective(l.pack).Credential(pluginsString(c, "credential"))
	if !ok {
		pluginsRefuse(c, pluginsDenied)
		return
	}
	method := pluginsString(c, "method")
	u, err := url.Parse(pluginsString(c, "url"))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" || u.Fragment != "" || !cred.Allows(method, u.Hostname(), u.EscapedPath()) {
		pluginsRefuse(c, pluginsDenied)
		return
	}
	body := pluginsString(c, "body")
	if len(body) > pluginsFetchBodyMax {
		pluginsRefuse(c, pluginsTooLarge)
		return
	}
	header := http.Header{}
	m, _ := c.Data.(map[string]any)
	given, _ := m["header"].(map[string]any)
	if len(given) > pluginsFetchHeaders {
		pluginsRefuse(c, pluginsBad)
		return
	}
	for k, v := range given {
		value, ok := v.(string)
		if !ok || !plugins.PluginHeader(k, cred.Header) || strings.ContainsAny(value, "\r\n\x00") || len(value) > 1024 {
			pluginsRefuse(c, pluginsBad)
			return
		}
		header.Set(k, value)
	}
	secrets := a.pluginsHostSecrets(l.pack.ID, l.pack.Signer)
	client := a.pluginsHTTPClient()
	target := u.String()
	pluginsWaiting(a, l, c, "fetch", func(ctx context.Context) (pluginsFetchReply, error) {
		secret, kept, err := secrets.Get(plugins.SecretKey(cred.ID))
		if err != nil {
			return pluginsFetchReply{}, err
		}
		if !kept || secret == "" {
			return pluginsFetchReply{}, errPluginsNoSecret
		}
		req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
		if err != nil {
			return pluginsFetchReply{}, err
		}
		req.Header = header
		req.Header.Set(cred.Header, cred.Value(secret))
		resp, err := client.Do(req)
		if err != nil {
			return pluginsFetchReply{}, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, pluginsFetchReplyMax+1))
		if err != nil {
			return pluginsFetchReply{}, err
		}
		if len(data) > pluginsFetchReplyMax {
			return pluginsFetchReply{}, errPluginsTooLarge
		}
		return pluginsFetchReply{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), body: data}, nil
	}, func(got pluginsFetchReply, err error) {
		switch {
		case errors.Is(err, errPluginsNoSecret):
			pluginsRefuse(c, pluginsNoSecret)
		case errors.Is(err, ns.ErrSecretsUnavailable):
			pluginsRefuse(c, pluginsUnavailable)
		case errors.Is(err, errPluginsTooLarge):
			pluginsRefuse(c, pluginsTooLarge)
		case errors.Is(err, context.DeadlineExceeded):
			pluginsRefuse(c, pluginsTimedOut)
		case err != nil:
			pluginsRefuse(c, pluginsFailed)
		default:
			out := map[string]any{}
			if got.contentType != "" {
				out["Content-Type"] = got.contentType
			}
			c.Resolve(map[string]any{"status": got.status, "header": out, "body": string(bytes.ToValidUTF8(got.body, []byte("�")))})
		}
	})
}

// errPluginsNoSecret is a credential the user gave Kando no secret for.
var errPluginsNoSecret = errors.New("no secret is kept for the credential")

// pluginsHTTPClient is the client credentials' requests go through: one that follows no redirect, whose reply goes back to the plugin as it is; the tests set another, which reaches their server.
func (a *app) pluginsHTTPClient() *http.Client {
	base := a.plugins.script.client
	if base == nil {
		base = &http.Client{}
	}
	c := *base
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.Jar = nil
	return &c
}
