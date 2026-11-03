package films

import (
	"os"
	"path/filepath"
	"strings"

	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// Film is one card on the shelf or the film open on the player page.
type Film struct {
	Key, Title, Name, Source, Detail string
	Stage                            core.Stage
	JobID, VideoPath, SubtitlePath   string
}

func List(root string, jobs []core.Job, videos []media.Video, nameOf func(core.Job) string, detailOf func(core.Job) string) []Film {
	used := map[string]bool{}
	out := make([]Film, 0, len(jobs)+len(videos))
	for i := range jobs {
		job := jobs[i]
		if !InLibrary(root, job) {
			continue
		}
		if job.VideoPath != "" {
			used[job.VideoPath] = true
		}
		out = append(out, FromJob(job, nameOf(job), detailOf(job)))
	}
	for _, video := range videos {
		if used[video.Path] || !InDir(root, video.Path) {
			continue
		}
		name := strings.TrimSuffix(video.Name, filepath.Ext(video.Name))
		out = append(out, Film{
			Key: video.Path, Title: Shorten(name, 22), Name: name, Detail: video.Path, VideoPath: video.Path,
		})
	}
	return out
}

func FromJob(job core.Job, name, detail string) Film {
	return Film{
		Key: job.ID, Title: Shorten(name, 22), Name: name, Source: job.Source,
		Detail: detail, Stage: job.Stage, JobID: job.ID,
		VideoPath: job.VideoPath, SubtitlePath: job.SubtitlePath,
	}
}

// InLibrary reports whether job belongs on the open library's shelf. A finished job follows its video file. A job that has not downloaded yet follows the library it was started in.
func InLibrary(root string, job core.Job) bool {
	if root == "" {
		return false
	}
	if job.VideoPath != "" {
		return InDir(root, job.VideoPath)
	}
	if job.LibraryDir != "" {
		return filepath.Clean(job.LibraryDir) == filepath.Clean(root)
	}
	return !job.Stage.Done()
}

func InDir(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func Find(items []Film, key string) (Film, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return Film{}, false
}

func DisplayPath(path string) string {
	if path == "" {
		return "未设置"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + path[len(prefix):]
	}
	return path
}

func Shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func StageLabel(stage core.Stage) string {
	switch stage {
	case core.StageQueued:
		return "排队"
	case core.StageDownloading:
		return "下载中"
	case core.StageTranscribing:
		return "转录中"
	case core.StageTranslating:
		return "翻译中"
	case core.StageExporting:
		return "导出中"
	case core.StageComplete:
		return "已完成"
	case core.StageFailed:
		return "失败"
	default:
		return string(stage)
	}
}

// Columns is how many posters fit across the window: the sidebar and page padding stay fixed, and each poster keeps its minimum width.
func Columns(windowWidth float32) int {
	content := windowWidth - sidebar.Width - sidebar.PagePadX*2
	if content < sidebar.CardMinWidth {
		return 1
	}
	n := int((content + sidebar.CardGap) / (sidebar.CardMinWidth + sidebar.CardGap))
	if n < 1 {
		return 1
	}
	return n
}

// ContentWidth is the main column inside the page padding.
func ContentWidth(windowWidth float32) float32 {
	w := windowWidth - sidebar.Width - sidebar.PagePadX*2
	if w < 1 {
		return 1
	}
	return w
}

func posterColor(key string) ui.Color {
	colors := []ui.Color{ui.Hex("#3a2c1e"), ui.Hex("#243044"), ui.Hex("#1e3330"), ui.Hex("#3a2430")}
	n := 0
	for _, r := range key {
		n += int(r)
	}
	return colors[n%len(colors)]
}
