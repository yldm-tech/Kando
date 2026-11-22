package films

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// textButton is b, a button of ui's, saying text in the language the app shows: text is read in the button's binding, so it follows the language, and whatever else it reads, without building the button again.
func textButton(text func(cx *core.Scope) string, b ui.Button) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		b.TextFunc = func() string { return text(cx) }
		return b
	})
}

// frosted is the prototype's backdrop-filter glass: what is under the view blurred by blur points, under tint.
func frosted(blur core.Points, tint core.Color) core.Style {
	return core.Style{BackdropFilters: []core.Filter{core.Blur(blur)}, Background: tint}
}

// cover is a picture filling its parent, as the posters and thumbnails lie under what is drawn over them. It shows nothing while source is the zero ImageSource.
func cover(source func() core.ImageSource) core.Node {
	return core.Image{SourceFunc: source, ResizeMode: core.Cover,
		Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Width: core.Pct(100), Height: core.Pct(100)}}
}

// hasImage reports whether source names a picture.
func hasImage(source core.ImageSource) bool { return source.URI != "" || len(source.Data) > 0 }

// overlay is a view over the whole of its parent.
var overlay = core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}

// clip is a view over the whole of its parent, rounded by radius as the parent is, that cuts children off at its edges. It is a view of its own, as core.Style's Overflow advises and ui.Dialog's Bare does, because some views that clip their children clip their own outer shadow too (Apple's images whose corners differ, Android's views), so a card with a shadow keeps the shadow and clips here.
func clip(radius core.Points, children ...core.Node) core.Node {
	return core.View{Style: overlay.Merge(core.Style{Overflow: core.Hidden, CornerRadius: radius}), Children: children}
}

// heading is text read as a heading of level, from 1, by assistive technology: core.Text takes no level, so a view around it carries the role.
func heading(level int, text core.Node) core.Node {
	return core.View{Role: core.RoleHeader, AccessibilityLevel: func() int { return level }, Children: []core.Node{text}}
}
