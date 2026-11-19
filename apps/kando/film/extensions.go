package film

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// SideExtensions is the FilmSide of the 扩展 tab (docs/plugin-system.md E15), which shows the panels the script plugins running add, one at a time.
const SideExtensions = "extensions"

// Extension is a panel a script plugin running adds to the 扩展 tab: ID names it for the app (SetExtension, ExtensionView), Run tells one load of its plugin from the next, so its view is built again when its plugin is loaded again, and Icon is one of plugins.PanelIcons or "" for none. Title and Plugin are its title and its plugin's name in the interface language.
type Extension struct {
	ID     string
	Title  string
	Plugin string
	Icon   string
	Run    int
}

// Extender is the 扩展 tab's: the panels of the plugins running and the one chosen.
type Extender interface {
	// Extensions are the panels the script plugins running add, in the order the plugins are listed, their titles in the language the interface shows under cx; none hides the tab.
	Extensions(cx *ns.Scope) []Extension
	// Extension is the ID of the panel chosen last, whichever film is open, "" before one was; SetExtension chooses another.
	Extension() string
	SetExtension(id string)
	// ExtensionView builds the view of the panel id, the plugin's own, under cx, and failed in its place while it fails, retry building it again.
	ExtensionView(cx *ns.Scope, id string, failed func(cx *ns.Scope, retry func()) ns.Node) ns.Node
}

// extensionIcons are the icons of plugins.PanelIcons by name.
var extensionIcons = map[string]func() *ns.SVG{
	"book-a": lucide.BookA, "book-open": lucide.BookOpen, "brain": lucide.Brain, "captions": lucide.Captions, "chart-bar": lucide.ChartBar,
	"graduation-cap": lucide.GraduationCap, "highlighter": lucide.Highlighter, "languages": lucide.Languages, "lightbulb": lucide.Lightbulb,
	"list-checks": lucide.ListChecks, "message-square": lucide.MessageSquare, "notebook-pen": lucide.NotebookPen, "puzzle": lucide.Puzzle,
	"search": lucide.Search, "sparkles": lucide.Sparkles, "star": lucide.Star,
}

// extensionChosen is the panel of list shown: the one chosen last while a plugin running adds it, else the first; the zero Extension for none.
func extensionChosen(list []Extension, id string) Extension {
	for _, e := range list {
		if e.ID == id {
			return e
		}
	}
	if len(list) > 0 {
		return list[0]
	}
	return Extension{}
}

// extensionsTab is the 扩展 tab: the panel shown, the plugin's own view, under a heading with its title, or a drop-down of the panels when there are more than one, and the name of the plugin that draws it, so a panel never passes for one of Kando's. The view is built again only when another panel is chosen or its plugin is loaded again, never as the app's state changes, so what it holds, its focus and scroll included, stays.
func (s *screen) extensionsTab(cx *ns.Scope) ns.Node {
	a := s.a
	list := live.Func(a, func() []Extension { return a.Extensions(cx) })
	chosen := func() Extension { return extensionChosen(list(), a.Extension()) }
	// The view's key: the panel and its plugin's load, not its title, which follows the language.
	key := ns.NewMemo(cx, func() Extension {
		e := chosen()
		return Extension{ID: e.ID, Run: e.Run}
	}).Get
	// options are the drop-down's label and the panels for it, each its id, title and icon, joined, so the drop-down is built again only when they change, or the language does, as ui.Select's Label is read once.
	options := ns.NewMemo(cx, func() string {
		out := []string{locales.FilmExtensionPick(cx)}
		for _, e := range list() {
			out = append(out, e.ID+"\x00"+e.Title+"\x00"+e.Icon)
		}
		return strings.Join(out, "\x01")
	}).Get
	muted := ns.RGBA(0xFFFFFF, 0.6)
	heading := ns.Show{When: func() bool { return strings.Count(options(), "\x01") < 2 }, Then: func(cx *ns.Scope) ns.Node {
		return ns.View{Style: ns.Style{Direction: ns.Row, AlignItems: ns.AlignCenter, Gap: 6}, Children: []ns.Node{
			ns.Show{When: func() bool { return extensionIcons[chosen().Icon] != nil }, Then: func(cx *ns.Scope) ns.Node {
				return ns.Icon{SVGFunc: func() *ns.SVG {
					if icon := extensionIcons[chosen().Icon]; icon != nil {
						return icon()
					}
					return nil
				}, Style: ns.Style{Width: ns.Pt(14), Height: ns.Pt(14), FlexShrink: 0, Color: onGlass}}
			}},
			ns.Text{TestID: "film-extension-title", ContentFunc: func() string { return chosen().Title }, Role: ns.RoleHeader, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
				Style: ns.Style{FontSize: 15, LineHeight: 21, FontWeight: ns.FontWeightBold, Color: onGlass, FlexShrink: 1}},
		}}
	}, Else: func(cx *ns.Scope) ns.Node {
		// TODO(nsgo#848): give ui.Select its options as a binding, so a list that changes does not build the drop-down again and take its focus; it is built again only when the panels of the plugins running change, which is done on another page https://github.com/nsgo-dev/nsgo/issues/848
		return ns.Dynamic(func(cx *ns.Scope) ns.Node { return s.extensionSelect(cx, options()) })
	}}
	head := ns.View{Style: ns.Style{Padding: ns.Edges{Right: 14, Bottom: 10, Left: 14}, Gap: 4, Borders: ns.Edges{Bottom: 0.5}, BorderColor: panelLine}, Children: []ns.Node{
		heading,
		ns.Text{TestID: "film-extension-plugin", ContentFunc: func() string { return locales.FilmExtensionFrom(cx, chosen().Plugin) }, MaxLines: 1, EllipsizeMode: ns.EllipsizeTail,
			Style: ns.Style{FontSize: 11, LineHeight: 15, Color: muted}},
	}}
	body := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		id := key().ID
		if id == "" {
			return ns.View{}
		}
		return a.ExtensionView(cx, id, func(cx *ns.Scope, retry func()) ns.Node {
			return ns.View{TestID: "film-extension-failed", Style: ns.Style{Padding: ns.Symmetric(12, 14), Gap: 8, AlignItems: ns.AlignStart}, Children: []ns.Node{
				ns.Text{ContentFunc: func() string { return locales.FilmExtensionFailed(cx, chosen().Plugin) }, LiveRegion: ns.LiveRegionPolite, Style: ns.Style{FontSize: 13, LineHeight: 19, Color: muted}},
				explainPill(cx, "film-extension-retry", func() string { return locales.FilmExtensionRetry(cx) }, retry),
			}}
		})
	})
	return ns.View{TestID: "film-extensions", Role: ns.RoleGroup, LabelFunc: func() string { return locales.FilmTabExtensions(cx) },
		Style:    ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)},
		Children: []ns.Node{head, ns.View{TestID: "film-extension-body", Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{body}}}}
}

// extensionSelect is the drop-down of the panels joined lists after its label, each its id, title and icon.
func (s *screen) extensionSelect(cx *ns.Scope, joined string) ns.Node {
	a := s.a
	items := strings.Split(joined, "\x01")
	var options []ui.Option[string]
	for _, item := range items[1:] {
		parts := strings.SplitN(item, "\x00", 3)
		if len(parts) < 3 {
			continue
		}
		o := ui.Option[string]{Value: parts[0], Label: parts[1], TestID: "film-extension-option-" + parts[0]}
		if icon := extensionIcons[parts[2]]; icon != nil {
			o.Icon = icon
		}
		options = append(options, o)
	}
	return ui.Select[string]{Options: options, Label: items[0], TestID: "film-extension-pick",
		Value:    live.Func(a, func() string { return extensionChosen(a.Extensions(cx), a.Extension()).ID }),
		OnChange: live.On(a, a.SetExtension), Style: ns.Style{AlignSelf: ns.AlignStretch}}
}
