// Package brand is Kando's own marks the phone app draws in its windows: the app's icon, as a tablet's sidebar heads itself with it. It is not a screen.
package brand

import (
	_ "embed"

	"github.com/nsgo-dev/nsgo/core"
)

// icon is the app's icon (assets/Icon.png, which nsgo.yaml gives the systems) at 132 pixels, enough for 44 points on a 3x screen.
//
//go:embed icon.png
var icon []byte

// Icon is the app's icon as an image's source.
func Icon() core.ImageSource { return core.ImageSource{Data: icon} }
