package loopback

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func get(t *testing.T, link, host string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK && resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%s is kept by caches", link)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("%s lets other origins read it", link)
	}
	return resp.StatusCode, string(body)
}

// What the server serves is reached by its URL alone, from loopback alone, and no longer once revoked.
func TestServesWhatItWasGivenByItsURL(t *testing.T) {
	s, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if !strings.HasPrefix(s.host, "127.0.0.1:") {
		t.Fatalf("it listens on %s", s.host)
	}
	served := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "the film") })
	link := s.URL("film", "a film.mp4", served)
	if again := s.URL("film", "a film.mp4", http.NotFoundHandler()); again != link || !strings.HasSuffix(link, "/a%20film.mp4") {
		t.Fatalf("the film is at %s, then %s", link, again)
	}
	if code, body := get(t, link, ""); code != http.StatusOK || body != "the film" {
		t.Fatalf("the film's URL answered %d %q", code, body)
	}
	u, _ := url.Parse(link)
	if code, _ := get(t, "http://"+u.Host+"/", ""); code != http.StatusNotFound {
		t.Fatalf("the root answered %d", code)
	}
	if code, _ := get(t, "http://"+u.Host+"/guess/a.mp4", ""); code != http.StatusNotFound {
		t.Fatalf("a guessed token answered %d", code)
	}
	// A page whose name points at loopback names itself in Host.
	if code, _ := get(t, link, "rebound.example:"+u.Port()); code != http.StatusMisdirectedRequest {
		t.Fatalf("a request for another host answered %d", code)
	}
	s.Revoke("film")
	if code, _ := get(t, link, ""); code != http.StatusNotFound {
		t.Fatalf("the revoked URL answered %d", code)
	}
	if fresh := s.URL("film", "a film.mp4", served); fresh == link {
		t.Fatal("the film is served again at its revoked URL")
	}
}
