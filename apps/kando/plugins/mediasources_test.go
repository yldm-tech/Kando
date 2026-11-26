package plugins

import (
	"errors"
	"strings"
	"testing"
)

// sourceManifest is a script plugin that contributes a WebDAV media source and an Alist one, as docs/media-sources.md 11.1 shows: kando.source.fetch, no network rules, no credentials.
func sourceManifest() string {
	m := scriptManifest()
	m = strings.Replace(m, `"host": ["kando.saved.read", "kando.words.read", "kando.settings.get"],
    "network": {"http": [{"host": "127.0.0.1", "port": 8765, "schemes": ["http"], "methods": ["POST"], "paths": ["/"]}]}`, `"host": ["kando.source.fetch", "kando.settings.get"]`, 1)
	return strings.Replace(m, `"contributes": {"commands": [`, `"contributes": {
    "sources": [
      {"id": "webdav", "title": {"en": "WebDAV", "zh-Hans": "WebDAV"}, "icon": "server", "fields": [
        {"key": "endpoint", "type": "url", "title": {"en": "Server address", "zh-Hans": "服务器地址"}},
        {"key": "user", "type": "text", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"basic": ["user", "password"]}, "title": {"en": "Password"}}
      ]},
      {"id": "alist", "title": {"en": "Alist"}, "icon": "cloud", "postPaths": ["/api/auth/login", "/api/fs/"], "fields": [
        {"key": "endpoint", "type": "url", "title": {"en": "Server address"}, "hint": {"en": "As http://nas.local:5244"}},
        {"key": "user", "type": "text", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"login": true}, "title": {"en": "Password"}},
        {"key": "token", "type": "secret", "auth": {"header": "X-Api-Token", "format": "Bearer {secret}"}, "title": {"en": "Token"}}
      ]}
    ],
    "commands": [`, 1)
}

// TestValidateMediaSources checks docs/media-sources.md 11.1: a plugin's media sources, with the exports they call, their fields and how each secret is sent, and that a plugin with sources reaches out only through kando.source.fetch.
func TestValidateMediaSources(t *testing.T) {
	p, err := Validate(writeScript(t, sourceManifest()))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.ExportsNeeded(); strings.Join(got, ",") != "run,list,stat,open" {
		t.Errorf("exports %v", got)
	}
	s := p.Contributes.Sources
	if len(s) != 2 || s[1].Fields[0].Hint.In("en") != "As http://nas.local:5244" || strings.Join(s[1].SecretKeys(), ",") != "password,token" {
		t.Errorf("sources %+v", s)
	}
	if p.ReadsLines() || Risky(p.Manifest, p.Declared()) || Online(p.Manifest) {
		t.Error("a media source reads no line, is not the risky combination, and works offline on the local network")
	}
	if PermSourceFetch.Group() != GroupNetwork || PermSourceFetch.Library() {
		t.Error("kando.source.fetch is listed under the network and reads nothing of the library")
	}
	for _, tt := range []struct {
		name, from, to string
		want           error
	}{
		{"sources without source.fetch", `"kando.source.fetch", `, ``, ErrBadField},
		{"source.fetch as optional", `"kando.source.fetch", `, ``, nil},
		{"network rules", `"host": ["kando.source.fetch", "kando.settings.get"]`, `"host": ["kando.source.fetch", "kando.settings.get"], "network": {"http": [{"host": "example.com", "schemes": ["https"], "methods": ["GET"], "paths": ["/"]}]}`, ErrSourceNetwork},
		{"credentialFetch", `"kando.source.fetch", `, `"kando.source.fetch", "kando.net.credentialFetch", `, ErrBadField},
		{"the AI service", `"kando.source.fetch", `, `"kando.source.fetch", "kando.llm.chat", `, ErrSourceNetwork},
		{"a page in the browser", `"optional": ["kando.toast"]`, `"optional": ["kando.toast", "kando.ui.openURL"]`, ErrSourceNetwork},
		{"the lines seen", `"kando.source.fetch", `, `"kando.source.fetch", "kando.lines.seen", `, ErrSourceNetwork},
		{"a source's id", `"id": "webdav"`, `"id": "Web DAV"`, ErrBadField},
		{"a source's id twice", `"id": "alist"`, `"id": "webdav"`, ErrBadField},
		{"a source without a title", `"title": {"en": "Alist"}`, `"title": {}`, ErrBadName},
		{"an icon not on the list", `"icon": "cloud"`, `"icon": "skull"`, ErrBadField},
		{"no endpoint", `{"key": "endpoint", "type": "url", "title": {"en": "Server address", "zh-Hans": "服务器地址"}},`, ``, ErrBadField},
		{"a url field that is not the endpoint", `{"key": "user", "type": "text", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"basic"`, `{"key": "user", "type": "text", "title": {"en": "User"}},
        {"key": "other", "type": "url", "title": {"en": "Other"}},
        {"key": "password", "type": "secret", "auth": {"basic"`, ErrBadField},
		{"a field's type", `"type": "text", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"login"`, `"type": "file", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"login"`, ErrBadField},
		{"a field's key", `{"key": "user", "type": "text", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"basic"`, `{"key": "User", "type": "text", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"basic"`, ErrBadField},
		{"a secret that says not how it is sent", `"auth": {"login": true}, `, ``, ErrBadField},
		{"a secret sent two ways", `"auth": {"login": true}`, `"auth": {"login": true, "header": "X-Key"}`, ErrBadField},
		{"basic with a field that is not text", `"basic": ["user", "password"]`, `"basic": ["endpoint", "password"]`, ErrBadField},
		{"basic sending another secret", `"basic": ["user", "password"]`, `"basic": ["user", "user"]`, ErrBadField},
		{"a header Kando sets", `"header": "X-Api-Token"`, `"header": "Cookie"`, ErrBadField},
		{"a format without the secret", `"format": "Bearer {secret}"`, `"format": "Bearer"`, ErrBadField},
		{"a sign-in Kando has not", `"format": "Bearer {secret}"`, `"format": "Bearer {secret}", "signIn": "emby"`, ErrBadField},
		{"two sign-ins", `"auth": {"login": true}, "title": {"en": "Password"}},
        {"key": "token", "type": "secret", "auth": {"header": "X-Api-Token", "format": "Bearer {secret}"}`, `"auth": {"header": "X-Key", "signIn": "plex"}, "title": {"en": "Password"}},
        {"key": "token", "type": "secret", "auth": {"header": "X-Api-Token", "format": "Bearer {secret}", "signIn": "plex"}`, ErrBadField},
		{"a sign-in not in a header", `"auth": {"login": true}`, `"auth": {"login": true, "signIn": "plex"}`, ErrBadField},
		{"a format with a line break", `"format": "Bearer {secret}"`, `"format": "Bearer {secret}\r\nX-Evil: 1"`, ErrBadField},
		{"auth on a text field", `{"key": "user", "type": "text", "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"basic"`, `{"key": "user", "type": "text", "auth": {"login": true}, "title": {"en": "User"}},
        {"key": "password", "type": "secret", "auth": {"basic"`, ErrBadField},
		{"a login secret without postPaths", `"postPaths": ["/api/auth/login", "/api/fs/"], `, ``, ErrBadField},
		{"a post path that climbs", `"/api/fs/"`, `"/api/../admin"`, ErrBadField},
		{"a post path that is not one", `"/api/fs/"`, `"api/fs"`, ErrBadField},
	} {
		manifest := strings.Replace(sourceManifest(), tt.from, tt.to, 1)
		if manifest == sourceManifest() {
			t.Fatalf("%s: %q is not in the manifest", tt.name, tt.from)
		}
		if tt.name == "source.fetch as optional" {
			manifest = strings.Replace(manifest, `"optional": ["kando.toast"]`, `"optional": ["kando.toast", "kando.source.fetch"]`, 1)
			tt.want = ErrBadField
		}
		_, err := Validate(writeScript(t, manifest))
		if err == nil {
			t.Errorf("%s: taken", tt.name)
		} else if !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
	if _, err := Validate(writeScript(t, strings.Replace(scriptManifest(), `"kando.settings.get"]`, `"kando.settings.get", "kando.source.fetch"]`, 1))); !errors.Is(err, ErrBadField) {
		t.Errorf("source.fetch without sources: %v", err)
	}
}

// TestValidateMediaSourceLimits checks the limits of docs/media-sources.md 3.2 and 11.1: at most 4 sources, 12 fields, 3 of them secret, and 8 POST prefixes.
func TestValidateMediaSourceLimits(t *testing.T) {
	source := func(id string, fields, secrets, posts int) string {
		var f []string
		f = append(f, `{"key": "endpoint", "type": "url", "title": {"en": "Address"}}`)
		for i := range fields - 1 - secrets {
			f = append(f, `{"key": "f`+string(rune('a'+i))+`", "type": "text", "title": {"en": "F"}}`)
		}
		for i := range secrets {
			f = append(f, `{"key": "s`+string(rune('a'+i))+`", "type": "secret", "auth": {"header": "X-K`+string(rune('a'+i))+`"}, "title": {"en": "S"}}`)
		}
		var p []string
		for i := range posts {
			p = append(p, `"/p`+string(rune('a'+i))+`"`)
		}
		return `{"id": "` + id + `", "title": {"en": "S"}, "icon": "server", "postPaths": [` + strings.Join(p, ",") + `], "fields": [` + strings.Join(f, ",") + `]}`
	}
	with := func(sources ...string) string {
		m := strings.Replace(scriptManifest(), `"host": ["kando.saved.read", "kando.words.read", "kando.settings.get"],
    "network": {"http": [{"host": "127.0.0.1", "port": 8765, "schemes": ["http"], "methods": ["POST"], "paths": ["/"]}]}`, `"host": ["kando.source.fetch"]`, 1)
		return strings.Replace(m, `"contributes": {"commands": [`, `"contributes": {"sources": [`+strings.Join(sources, ",")+`], "commands": [`, 1)
	}
	for _, tt := range []struct {
		name     string
		manifest string
		ok       bool
	}{
		{"the most of everything", with(source("a", 12, 3, 8), source("b", 1, 0, 0), source("c", 1, 0, 0), source("d", 1, 0, 0)), true},
		{"five sources", with(source("a", 1, 0, 0), source("b", 1, 0, 0), source("c", 1, 0, 0), source("d", 1, 0, 0), source("e", 1, 0, 0)), false},
		{"13 fields", with(source("a", 13, 0, 0)), false},
		{"4 secrets", with(source("a", 5, 4, 0)), false},
		{"9 post paths", with(source("a", 1, 0, 9)), false},
	} {
		_, err := Validate(writeScript(t, tt.manifest))
		if (err == nil) != tt.ok {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

// TestMediaSourceAllowsPost checks where a source's plugin may POST: under one of its postPaths by whole segments, and nowhere else.
func TestMediaSourceAllowsPost(t *testing.T) {
	s := MediaSource{PostPaths: []string{"/api/auth/login", "/api/fs/"}}
	for _, tt := range []struct {
		path string
		want bool
	}{
		{"/api/auth/login", true},
		{"/api/auth/login/x", true},
		{"/api/fs/list", true},
		{"/api/auth/logins", false},
		{"/api/fs", false},
		{"/api/admin/user/delete", false},
		{"/api/fs/../admin", false},
		{"/api/fs/%2e%2e/admin", false},
		{"/api/fs//x", false},
	} {
		if got := s.AllowsPost(tt.path); got != tt.want {
			t.Errorf("%s: %t, want %t", tt.path, got, tt.want)
		}
	}
	if (MediaSource{}).AllowsPost("/") {
		t.Error("a source with no postPaths POSTs")
	}
}
