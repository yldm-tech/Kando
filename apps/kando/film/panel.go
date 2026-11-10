package film

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// panelTabs are the side panel's tabs, by the FilmSide each one is; tabLabel names them.
var panelTabs = []string{"", "clips", "info", "style"}

// tabLabel is what a tab of the side panel is called in the interface's language.
func tabLabel(id string) string {
	switch id {
	case "clips":
		return locales.FilmTabClips()
	case "info":
		return locales.FilmTabInfo()
	case "style":
		return locales.FilmTabStyle()
	}
	return locales.FilmSubtitles()
}

// The side panel's surfaces (prototype template 642-876).
var (
	panelCard    = ui.RGBA(255, 255, 255, 0.07)
	panelCurrent = ui.RGBA(255, 255, 255, 0.14)
	panelLine    = ui.RGBA(255, 255, 255, 0.1)
)

// panel is the glass side panel floating over the right of the picture: the subtitle list, the 片段 and the film's files under three tabs, or the subtitle style in their place.
func (s screen) panel(c *ui.Context, t *ui.Theme, item films.Film, panelSpace float32) {
	c.Key("film-side-panel")
	shown := s.panelShown()
	alpha := panelSpace / panelRoom
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	box := ui.Column(c).Absolute().Right(16).Top(62).Bottom(18).Width(panelWidth).Radius(22).Clip().Role(ui.RoleGroup).Label(locales.FilmSidebar()).Translate(panelRoom-panelSpace, 0).Opacity(alpha)
	if !shown {
		box.PassThrough()
	}
	glassFace(box, ui.RGBA(28, 28, 32, 0.6), 34, 0.2, 22).Shadow(0, 20, 50, 0, ui.RGBA(0, 0, 0, 0.35))
	box.Children(func() {
		ui.Column(c).Padding(14, 14, 10, 14).Children(func() { s.panelTabBar(c) })
		if s.StyleView() {
			s.styleView(c)
		} else {
			switch s.FilmSide() {
			case "clips":
				s.clipsTab(c, t)
			case "info":
				s.infoTab(c, item)
			default:
				s.subsTab(c, t, item)
			}
		}
	})
}

// panelTabBar is the prototype's tab bar across the panel (template 718-724): a track of white at 10%, its tabs sharing its width, the chosen one lit with white at 22% and its label semibold. breeze's Tabs keep their own width, centered, so the bar is MyGo's tabs (ui.TabsBase), whose keys and roles are a tab list's, with the prototype's look.
func (s screen) panelTabBar(c *ui.Context) {
	chosen := 0
	for i, tab := range panelTabs {
		if tab == s.FilmSide() {
			chosen = i
		}
	}
	c.Key("film-tabs")
	if s.StyleView() {
		chosen = len(panelTabs) - 1
	}
	tabs := ui.TabsBase(c, &chosen, len(panelTabs))
	tabs.List.Height(32).Padding(3).Radius(12).Background(ui.RGBA(255, 255, 255, 0.1)).Label(locales.FilmSidebar()).Children(func() {
		for i, tab := range panelTabs {
			t := tabs.Tab(i).Grow(1).Basis(0).Height(26).Radius(9).Justify(ui.Center).AlignItems(ui.Center)
			weight := 400
			if i == chosen {
				t.Background(ui.RGBA(255, 255, 255, 0.22))
				weight = 600
			} else if t.Hovered() {
				t.Background(panelLine)
			}
			t.Children(func() { ui.Text(c, tabLabel(tab)).FontSize(12).FontWeight(weight).SingleLine() })
		}
	})
	if chosen == len(panelTabs)-1 {
		s.SetStyleView(true)
		return
	}
	if s.StyleView() {
		s.SetStyleView(false)
	}
	if panelTabs[chosen] != s.FilmSide() {
		s.SetFilmSide(panelTabs[chosen])
	}
}

// subsTab lists the cues with the time each starts, the playing one lit and kept in view as it changes. A click plays from a cue. The field above it searches both lines.
func (s screen) subsTab(c *ui.Context, t *ui.Theme, item films.Film) {
	subs := s.Subs()
	ready := subs.Key == item.Key && subs.Ready
	ui.Column(c).Padding(0, 14, 8, 14).Gap(8).Children(func() {
		c.Key("film-search")
		// The prototype's field is 30 high, white at 10%, its text 12; its bezel and its edge stay the platform's.
		field := breeze.SearchField(c, s.Search()).Placeholder(locales.FilmSearchLines() + "  " + breeze.ShortcutText(c, ui.Cmd, ui.KeyF)).Label(locales.FilmSearchLines())
		field.Height(30).Background(ui.RGBA(255, 255, 255, 0.1)).FontSize(12)
		if focus := s.SearchFocus(); *focus {
			field.Focus()
			*focus = false
		}
		if ready && subs.Err == "" && len(subs.Cues) > 0 {
			// How many lines the list holds, as the prototype's chip 全部 shows it.
			ui.Row(c).Height(24).Padding(0, 10).Radius(12).Gap(3).AlignItems(ui.Center).AlignSelf(ui.Start).Background(ui.RGBA(255, 255, 255, 0.25)).Children(func() {
				ui.Text(c, locales.FilmAll()).FontSize(11).FontWeight(600)
				ui.Text(c, fmt.Sprint(len(subs.Cues))).FontSize(11).FontWeight(600).Opacity(0.6)
			})
		}
	})
	body := ui.Column(c).Grow(1).MinHeight(0).Padding(0, 8, 12, 8)
	body.Children(func() {
		switch {
		case !ready:
			panelNote(c, locales.FilmLoadingSubtitles())
		case subs.Err != "":
			panelNote(c, subs.Err).TextColor(t.Danger)
		case item.SubtitlePath == "":
			panelNote(c, locales.FilmNoSubtitlesYet())
		case len(subs.Cues) == 0:
			panelNote(c, locales.FilmSubtitlesEmpty())
		default:
			s.cueList(c, subs.Cues)
		}
	})
	ui.Row(c).Padding(8, 14, 12, 14).BorderWidth(1, 0, 0, 0).BorderColor(panelLine).Children(func() {
		ui.Text(c, locales.FilmClickToJump()).FontSize(11).LineHeight(15.0 / 11).Opacity(0.55)
	})
}

// cueList is the subtitle list, narrowed to the cues that hold the search.
func (s screen) cueList(c *ui.Context, cues []core.Segment) {
	query := strings.ToLower(strings.TrimSpace(*s.Search()))
	shown := make([]int, 0, len(cues))
	for i, cue := range cues {
		if query == "" || strings.Contains(strings.ToLower(cue.Text), query) {
			shown = append(shown, i)
		}
	}
	if len(shown) == 0 {
		panelNote(c, locales.FilmNoMatch()).TextAlign(ui.Center)
		return
	}
	current := -1
	if s.Play() != nil {
		current = cueIndex(cues, s.Play().Head()-s.Delay())
	}
	list := s.SubList()
	if follow := s.Follow(); current != *follow {
		*follow = current
		for row, i := range shown {
			if i == current {
				list.ScrollIntoView(row)
			}
		}
	}
	ui.List(c, list, len(shown), func(row int) {
		i := shown[row]
		s.cueRow(c, cues[i], i == current)
	}).Grow(1)
}

// cueRow is one cue of the list: its start, the translation and the original under it. A click plays from its start.
func (s screen) cueRow(c *ui.Context, cue core.Segment, current bool) {
	translation, original := splitCue(cue.Text)
	if translation == "" {
		translation, original = original, ""
	}
	row := ui.ButtonBase(c).Row().Gap(10).Padding(8, 10).Radius(10).AlignItems(ui.Start).Label(cueClock(cue.Start) + " " + translation)
	if current {
		row.Background(panelCurrent)
	} else if row.Hovered() {
		row.Background(panelLine)
	}
	if row.Clicked() {
		s.Seek(cue.Start + s.Delay())
	}
	row.Children(func() {
		ui.Text(c, cueClockTenths(cue.Start)).FontSize(11).FontFeatures("tnum").Opacity(0.7).Width(38).Shrink(0).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			main := ui.Text(c, translation).FontSize(14).LineHeight(19.0 / 14).Wrap()
			if current {
				main.FontWeight(600)
			}
			if original != "" {
				ui.Text(c, original).FontSize(12).LineHeight(16.0 / 12).Opacity(0.7).Wrap()
			}
		})
	})
}

// clipsTab is the film cut into 片段 of equal time; a click plays from one, and the playing one is lit.
func (s screen) clipsTab(c *ui.Context, t *ui.Theme) {
	subs := s.Subs()
	chapters := Chapters(subs.Cues, s.PlayLength())
	ui.Column(c).Grow(1).MinHeight(0).Padding(0, 14, 14, 14).Gap(8).Children(func() {
		switch {
		case !subs.Ready:
			panelNote(c, locales.FilmLoadingSubtitles())
		case subs.Err != "":
			panelNote(c, subs.Err).TextColor(t.Danger)
		case len(chapters) == 0:
			panelNote(c, locales.FilmNoClipsYet())
		default:
			ui.Row(c).MinHeight(30).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.FilmClipsNote()).FontSize(11).LineHeight(15.0 / 11).Opacity(0.7).Wrap()
			})
			ui.List(c, s.ClipList(), len(chapters), func(i int) {
				s.chapterRow(c, i, chapters[i])
			}).Grow(1).Gap(8)
		}
	})
}

func (s screen) chapterRow(c *ui.Context, index int, ch Chapter) {
	active := s.chapterActive(ch)
	row := ui.ButtonBase(c).Row().Gap(12).Padding(8).Radius(12).AlignItems(ui.Center).Label(locales.FilmClip(index + 1))
	if active {
		row.Background(ui.RGBA(255, 255, 255, 0.16))
	} else if row.Hovered() {
		row.Background(panelLine)
	}
	if row.Clicked() {
		s.Seek(ch.Start)
	}
	row.Children(func() {
		thumb := ui.Box(c).Width(84).AspectRatio(16.0/9).Radius(7).Shrink(0).Clip().Background(clipTiles[index%len(clipTiles)]).InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, 0.15))
		thumb.Children(func() {
			if img := s.ClipPoster(ch.Start); img != nil {
				ui.Image(c, img).Absolute().Top(0).Left(0).WidthPercent(100).HeightPercent(100).Fit(ui.Cover)
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(0, 0, 0, 0.18))
			}
			ui.Text(c, cueClock(ch.Start)).FontSize(10).FontWeight(600).FontFeatures("tnum").Absolute().Right(4).Bottom(3)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, locales.FilmClipSpan(index+1, cueClock(ch.Start), cueClock(ch.End))).FontSize(11).FontWeight(600).Opacity(0.7)
			text := ui.Text(c, ch.Title).FontSize(13).FontWeight(500).LineHeight(18.0 / 13).SingleLine()
			if active {
				text.Bold()
			}
			if ch.Detail != "" {
				ui.Text(c, ch.Detail).FontSize(11).LineHeight(15.0 / 11).Opacity(0.6).SingleLine()
			}
		})
	})
}

// clipTiles are the colors of the 片段's tiles, the prototype's stand-ins for a frame of each.
var clipTiles = []ui.Color{ui.Hex("#3E6A5B"), ui.Hex("#6A8EA1"), ui.Hex("#4B7A8C"), ui.Hex("#5D7F63"), ui.Hex("#7C8F6B"), ui.Hex("#2E5B6E")}

func (s screen) chapterActive(ch Chapter) bool {
	if s.Play() == nil {
		return false
	}
	at := s.Play().Head()
	return at >= ch.Start && (ch.End <= ch.Start || at < ch.End)
}

// infoTab shows where the film is: its folder, the files in it and where it came from.
func (s screen) infoTab(c *ui.Context, item films.Film) {
	info := s.Info()
	folder := info.Folder
	if folder == "" && item.VideoPath != "" {
		folder = filepath.Dir(item.VideoPath)
	}
	files := info.Files
	if len(files) == 0 {
		files = knownInfoFiles(item)
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).WidthPercent(100).Padding(0, 14, 16, 14).Gap(12).Children(func() {
			panelGroup(c, func() {
				ui.Column(c).Padding(12).Gap(8).Children(func() {
					title := item.Name
					if title == "" {
						title = item.Title
					}
					ui.Text(c, films.Shorten(title, 52)).FontSize(15).FontWeight(650).LineHeight(19.0 / 15).Wrap()
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						infoChip(c, infoStage(item))
						if item.Language != "" {
							infoChip(c, languageName(item.Language))
						}
					})
				})
			})
			if folder != "" {
				panelHeading(c, locales.FilmFolder())
				panelGroup(c, func() {
					ui.Column(c).Padding(10, 12).Gap(8).Children(func() {
						ui.Text(c, films.DisplayPath(folder)).FontSize(11).LineHeight(16.0 / 11).Font("Menlo").Opacity(0.9).Wrap().Selectable()
						c.Key("film-info-reveal")
						if breeze.Button(c, locales.FilmShowInFinder(breeze.PlatformOf(c).String())).Height(26).AlignSelf(ui.Start).Clicked() {
							s.Reveal(item.VideoPath)
						}
					})
				})
			}
			if len(files) > 0 {
				panelHeading(c, locales.FilmFiles())
				panelGroup(c, func() {
					for i, f := range filesInOrder(files) {
						fileRow(c, f, i > 0, func() {
							if folder != "" {
								s.Reveal(filepath.Join(folder, f.Name))
							}
						})
					}
				})
			}
			panelHeading(c, locales.FilmSource())
			panelGroup(c, func() {
				ui.Column(c).Padding(10, 12).Gap(6).Children(func() {
					source := item.Source
					if source == "" || strings.HasPrefix(source, "file://") {
						ui.Text(c, locales.FilmLocalFile()).FontSize(12).LineHeight(17.0 / 12).Opacity(0.9)
					} else {
						ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
							breeze.Link(c, source, source).FontSize(12).LineHeight(17.0 / 12).Wrap().Grow(1).MinWidth(0)
							copyInfoButton(c, locales.FilmCopy(), source)
						})
					}
					ui.Text(c, locales.FilmJobRecord()).FontSize(11).Opacity(0.55)
					recordPath := s.LibraryPath()
					if recordPath == "" {
						recordPath = "~/Library/Application Support/Kando/library.json"
					}
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						ui.Text(c, films.DisplayPath(recordPath)).FontSize(11).LineHeight(15.0 / 11).Font("Menlo").Opacity(0.65).Wrap().Grow(1).MinWidth(0).Selectable()
						if breeze.FlatButton(c, locales.FilmOpen()).Height(24).Padding(0, 8).Clicked() {
							s.Reveal(recordPath)
						}
						copyInfoButton(c, locales.FilmCopy(), recordPath)
					})
				})
			})
		})
	})
}

func copyInfoButton(c *ui.Context, label, value string) {
	if breeze.FlatButton(c, label).Height(24).Padding(0, 8).Shrink(0).Clicked() {
		c.WriteClipboard(value)
	}
}

// knownInfoFiles keeps the information page useful while the background
// directory scan is still pending, or when a video was opened without a
// readable folder. It only uses paths already present in the film state.
func knownInfoFiles(item films.Film) []File {
	seen := map[string]bool{}
	var out []File
	for _, path := range []string{item.VideoPath, item.SubtitlePath} {
		name := filepath.Base(path)
		if path == "" || name == "." || name == string(filepath.Separator) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, File{Name: name, Size: -1})
	}
	return out
}

func infoChip(c *ui.Context, text string) {
	if text == "" {
		return
	}
	ui.Text(c, text).FontSize(10).FontWeight(600).Opacity(0.75).Background(ui.RGBA(255, 255, 255, 0.12)).Radius(8).Padding(2, 7).SingleLine()
}

func infoStage(item films.Film) string {
	if item.Stage != "" {
		return films.StageLabel(item.Stage)
	}
	if item.VideoPath != "" {
		return locales.FilmLocalVideo()
	}
	return locales.FilmUnprocessed()
}

// languageName is what the info tab shows for the language a film's subtitles were translated into, the value its job saved, such as 简体中文 (subtitle.TargetLanguageName).
func languageName(saved string) string { return subtitle.TargetLanguageName(saved) }

// fileRow is a file of the film's folder: a tile with its kind, its name, its size, and a reveal action.
func fileRow(c *ui.Context, f File, divided bool, reveal func()) {
	row := ui.Row(c).Gap(10).MinHeight(50).Padding(7, 12).AlignItems(ui.Center)
	if divided {
		row.BorderWidth(1, 0, 0, 0).BorderColor(panelLine)
	}
	row.Children(func() {
		tag, color := fileTag(f.Name)
		ui.Box(c).Size(28, 28).Radius(7).Shrink(0).Background(color).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, tag).FontSize(9).FontWeight(800).LetterSpacing(0.3)
		})
		ui.Column(c).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, f.Name).FontSize(12).Font("Menlo").Wrap()
			about := fileSize(f.Size)
			if kind := fileKind(f.Name); kind != "" {
				about = kindLabel(kind) + " · " + about
			}
			ui.Text(c, about).FontSize(11).Opacity(0.65)
		})
		c.Key("film-info-file-" + f.Name)
		if breeze.FlatButton(c, locales.FilmReveal()).Height(24).Padding(0, 8).Shrink(0).Clicked() {
			reveal()
		}
	})
}

// The kinds of files the pipeline writes into a film's folder, as the prototype's 信息 tells them apart (script 3261-3266); kindLabel names them.
const (
	kindVideo     = "video"
	kindCover     = "cover"
	kindOriginal  = "original"
	kindBilingual = "bilingual"
)

// fileKind is what a file the pipeline writes into a film's folder is, one of the kinds above, or "" for another file.
func fileKind(name string) string {
	switch {
	case strings.HasSuffix(name, ".bilingual.srt"):
		return kindBilingual
	case name == "cover.jpg":
		return kindCover
	}
	switch strings.ToUpper(strings.TrimPrefix(fileExt(name), ".")) {
	case "SRT", "VTT", "ASS":
		return kindOriginal
	case "MP4", "MKV", "MOV", "WEBM", "M4V":
		return kindVideo
	}
	return ""
}

// kindLabel is what the info tab calls a kind of file in the interface's language.
func kindLabel(kind string) string {
	switch kind {
	case kindVideo:
		return locales.FilmKindVideo()
	case kindCover:
		return locales.FilmKindCover()
	case kindOriginal:
		return locales.FilmKindOriginal()
	case kindBilingual:
		return locales.FilmKindBilingual()
	}
	return ""
}

// filesInOrder is files as the prototype lists a film's folder: the video, the cover, the subtitles, then the rest, the shorter name first within each, as video.srt before video.bilingual.srt.
func filesInOrder(files []File) []File {
	rank := func(f File) int {
		switch fileKind(f.Name) {
		case kindVideo:
			return 0
		case kindCover:
			return 1
		case kindOriginal, kindBilingual:
			return 2
		}
		return 3
	}
	out := slices.Clone(files)
	slices.SortStableFunc(out, func(a, b File) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(len(a.Name), len(b.Name)), strings.Compare(a.Name, b.Name))
	})
	return out
}

// fileTag is the kind a file's tile shows and its color, after the prototype's (script 3261-3266).
func fileTag(name string) (string, ui.Color) {
	ext := strings.ToUpper(strings.TrimPrefix(fileExt(name), "."))
	switch ext {
	case "SRT", "VTT", "ASS":
		return ext, ui.Hex("#6B5A3A")
	case "JPG", "JPEG", "PNG", "WEBP":
		return "JPG", ui.Hex("#4C6B4C")
	case "":
		return "—", ui.Hex("#4A4A50")
	}
	if len(ext) > 4 {
		ext = ext[:4]
	}
	return ext, ui.Hex("#3A5A8C")
}

func fileExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[i:]
	}
	return ""
}

func fileSize(n int64) string {
	if n < 0 {
		return locales.FilmSizeUnknown()
	}
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return locales.FilmBytes(int(n))
}

func panelHeading(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(11).FontWeight(700).Opacity(0.6).Padding(0, 4)
}

// panelGroup is a card of the panel, a lighter glass holding rows.
func panelGroup(c *ui.Context, fn func()) {
	ui.Column(c).Radius(14).Background(panelCard).Children(fn)
}

func panelNote(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(12).Opacity(0.6).Wrap().Margin(30, 10)
}

// Chapter is one of the 片段 the film is cut into.
type Chapter struct {
	Start, End    time.Duration
	Title, Detail string
}

// Chapters cuts the film into 4 to 12 片段 of equal time, about 30 seconds each, from the start to length, or to the last cue's end when that is later or length is not known. Each is named after the cue playing as it starts, or the first one after it: the translation, with the original as its detail (prototype script 3248-3259).
func Chapters(cues []core.Segment, length time.Duration) []Chapter {
	if len(cues) == 0 {
		return nil
	}
	end := max(length, cues[len(cues)-1].End, cues[len(cues)-1].Start)
	if end <= 0 {
		return nil
	}
	n := int((end + 15*time.Second) / (30 * time.Second))
	n = min(max(n, 4), 12)
	out := make([]Chapter, 0, n)
	for k := range n {
		from := end * time.Duration(k) / time.Duration(n)
		to := end * time.Duration(k+1) / time.Duration(n)
		out = append(out, chapterAt(cues, from, to))
	}
	return out
}

// chapterAt is the 片段 from from to to, named after the cue that started last by from, or the first one.
func chapterAt(cues []core.Segment, from, to time.Duration) Chapter {
	cue := cues[0]
	for _, c := range cues {
		if c.Start > from {
			break
		}
		cue = c
	}
	title, detail := splitCue(cue.Text)
	if title == "" {
		title, detail = detail, ""
	}
	return Chapter{Start: from, End: to, Title: films.Shorten(title, 36), Detail: films.Shorten(detail, 48)}
}
