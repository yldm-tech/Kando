package media

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"kando/packages/core"
)

func TestLocalEndpoint(t *testing.T) {
	for base, want := range map[string]bool{
		"http://127.0.0.1:11434/v1":  true,
		"http://localhost:1234/v1":   true,
		"http://LOCALHOST./v1":       true,
		"http://ollama.localhost/v1": true,
		"http://[::1]:8080/v1":       true,
		"http://127.8.9.10/v1":       true,
		EveryAPIBaseURL:              false,
		"https://api.openai.com/v1":  false,
		"http://192.168.1.20:11434":  false,
		"http://localhost.evil.com/": false,
		"":                           false,
		"not a url":                  false,
	} {
		if got := LocalEndpoint(base); got != want {
			t.Errorf("LocalEndpoint(%q) = %v, want %v", base, got, want)
		}
	}
}

// offlineServer is a chat endpoint that counts the requests it gets; the test fails if offline mode lets one through to a service it should refuse.
func offlineServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"m"}]}`))
		default:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"[\"hola\"]"}}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestOfflineTranslatorRefusesCloudServices(t *testing.T) {
	srv, hits := offlineServer(t)
	// The test server listens on 127.0.0.1; named by a host that is not a loopback one it stands for a cloud service, which the client still reaches through the dialer below.
	cloudBase := "http://cloud.example/v1"
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, srv.Listener.Addr().String())
	}}}
	tr := ChatTranslator{BaseURL: cloudBase, Model: "m", Client: client, Offline: true}
	ctx := context.Background()
	segments := []core.Segment{{Text: "hello"}}
	if err := tr.Ping(ctx); !errors.Is(err, ErrOffline) {
		t.Errorf("Ping = %v, want ErrOffline", err)
	}
	if _, err := tr.Chat(ctx, "s", "u"); !errors.Is(err, ErrOffline) {
		t.Errorf("Chat = %v, want ErrOffline", err)
	}
	if _, err := tr.Translate(ctx, segments, "es"); !errors.Is(err, ErrOffline) {
		t.Errorf("Translate = %v, want ErrOffline", err)
	}
	if _, err := tr.Proofread(ctx, segments, "t"); !errors.Is(err, ErrOffline) {
		t.Errorf("Proofread = %v, want ErrOffline", err)
	}
	if _, err := tr.Models(ctx); !errors.Is(err, ErrOffline) {
		t.Errorf("Models = %v, want ErrOffline", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("offline translator sent %d requests to a cloud service", n)
	}
	// A credential is a sign-in through a program that asks a server, so it is refused too, without being asked.
	asked := false
	signed := ChatTranslator{BaseURL: "http://127.0.0.1:1/v1", Model: "m", Offline: true, Credential: func(context.Context, bool) (Endpoint, error) {
		asked = true
		return Endpoint{}, nil
	}}
	if _, err := signed.Chat(ctx, "s", "u"); !errors.Is(err, ErrOffline) || asked {
		t.Errorf("Chat with a credential = %v (asked %v), want ErrOffline without asking", err, asked)
	}
	// Online, the same translator reaches the service.
	tr.Offline = false
	if err := tr.Ping(ctx); err != nil {
		t.Fatalf("online Ping: %v", err)
	}
	if hits.Load() == 0 {
		t.Fatal("online translator sent nothing")
	}
}

func TestOfflineTranslatorKeepsLocalServices(t *testing.T) {
	srv, hits := offlineServer(t)
	tr := ChatTranslator{BaseURL: srv.URL + "/v1", Model: "m", Offline: true}
	if !tr.Local() {
		t.Fatalf("%s is not local", tr.BaseURL)
	}
	ctx := context.Background()
	if err := tr.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	got, err := tr.Translate(ctx, []core.Segment{{Text: "hello"}}, "es")
	if err != nil || len(got) != 1 || got[0].Text != "hola" {
		t.Fatalf("Translate = %v, %v", got, err)
	}
	if models, err := tr.Models(ctx); err != nil || len(models) != 1 {
		t.Fatalf("Models = %v, %v", models, err)
	}
	if hits.Load() < 3 {
		t.Fatalf("local service got %d requests", hits.Load())
	}
}

func TestOfflineToolsRefuseLinksAndCloudTranslation(t *testing.T) {
	tools := Tools{YTDLP: "/nonexistent/yt-dlp", OutputDir: t.TempDir(), Offline: true,
		Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: "m", APIKey: "k"}}
	ctx := context.Background()
	if _, err := tools.Download(ctx, "https://www.youtube.com/watch?v=abc"); !errors.Is(err, ErrOffline) {
		t.Errorf("Download = %v, want ErrOffline", err)
	}
	if _, err := tools.ListVideos(ctx, "https://www.youtube.com/playlist?list=x", 0); !errors.Is(err, ErrOffline) {
		t.Errorf("ListVideos = %v, want ErrOffline", err)
	}
	if err := tools.CheckTranslate(); !errors.Is(err, ErrOffline) {
		t.Errorf("CheckTranslate = %v, want ErrOffline", err)
	}
	if _, err := tools.Translate(ctx, []core.Segment{{Text: "x"}}, "es"); !errors.Is(err, ErrOffline) {
		t.Errorf("Translate = %v, want ErrOffline", err)
	}
	if _, err := tools.Proofread(ctx, []core.Segment{{Text: "x"}}, "t"); !errors.Is(err, ErrOffline) {
		t.Errorf("Proofread = %v, want ErrOffline", err)
	}
	// A local video is read where it is, offline or not.
	video := writeOfflineVideo(t)
	if got, err := tools.Download(ctx, core.FileURL(video)); err != nil || got != video {
		t.Errorf("Download of a local video = %q, %v", got, err)
	}
	// A translator on this computer still checks out.
	tools.Translator = ChatTranslator{BaseURL: "http://127.0.0.1:11434/v1", Model: "m"}
	if err := tools.CheckTranslate(); err != nil {
		t.Errorf("CheckTranslate with a local service = %v", err)
	}
}

// writeOfflineVideo writes a file that stands for a local video.
func writeOfflineVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
