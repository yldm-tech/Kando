package film

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// glyphButton is a button over the picture showing icon at iw×ih in a w×h box, in white, named label: breeze's FlatButton, whose face shows only under the pointer, holding the prototype's glyph at the prototype's size, which FlatIconButton fixes at 16.
func glyphButton(c *ui.Context, icon *ui.SVG, label string, w, h, iw, ih float32) ui.Element {
	b := breeze.FlatButton(c, "").Label(label).Tooltip(label).Size(w, h).Padding(0).Gap(0).TextColor(onGlass)
	b.Children(func() { ui.Icon(c, icon).Size(iw, ih).Shrink(0) })
	return b
}

// glyphToggle is glyphButton as breeze's FlatToggle, which stays lit while *on.
func glyphToggle(c *ui.Context, on *bool, icon *ui.SVG, label string, w, h, iw, ih float32) ui.Element {
	b := breeze.FlatToggle(c, on, nil, "").Label(label).Tooltip(label).Size(w, h).Padding(0).Gap(0).TextColor(onGlass)
	b.Children(func() { ui.Icon(c, icon).Size(iw, ih).Shrink(0) })
	return b
}
