package main

import (
	"net/url"
	"path/filepath"
	"strings"

	"kando/packages/core"
)

// The names the library gives a job's card.

func jobTitle(job core.Job) string {
	return shorten(jobName(job), 22)
}

// libraryJobName is the name the shelf shows for job: the title the user gave its video in Kando (shelf.Library.Names), or else jobName. films.List gives the shelf's films the same.
func (a *app) libraryJobName(job core.Job) string {
	if name := a.library.Names[job.VideoPath]; job.VideoPath != "" && name != "" {
		return name
	}
	return jobName(job)
}

// jobName is the video's title as its site gives it, once the download has looked it up; otherwise the name of its file or folder, or what its link says.
func jobName(job core.Job) string {
	if title := strings.TrimSpace(job.Title); title != "" {
		return title
	}
	if job.VideoPath != "" {
		return displayVideoName(job.VideoPath)
	}
	parsed, err := url.Parse(job.Source)
	if err != nil {
		return job.Source
	}
	if path, local := core.FilePath(parsed); local {
		return displayVideoName(path)
	}
	if parsed.Host == "" {
		return job.Source
	}
	host := strings.TrimPrefix(parsed.Hostname(), "www.")
	if host == "youtu.be" {
		if id := strings.Trim(parsed.Path, "/"); id != "" {
			return "YouTube · " + id
		}
	}
	if host == "youtube.com" || host == "m.youtube.com" || host == "music.youtube.com" {
		if id := parsed.Query().Get("v"); id != "" {
			return "YouTube · " + id
		}
	}
	title := host
	if parsed.Path != "" && parsed.Path != "/" {
		title += parsed.Path
	}
	return title
}

func displayVideoName(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name != "video" {
		return name
	}
	parent := filepath.Base(filepath.Dir(path))
	if parent == "" || parent == "." || parent == string(filepath.Separator) {
		return name
	}
	return parent
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
