package desktop

import (
	"os"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
)

// hostname is the system's host name, os.Hostname; tests replace it.
var hostname = os.Hostname

// Name is this computer's name as Kando gives it to others, as phones list it and EveryAPI's list of keys shows it: the name the user gave the computer (core.Env.DeviceName), such as "Ada's MacBook Pro", or where the system tells none, its host name without the .local the Mac adds; "" when it has neither. cx may be nil, before the app's scope exists, which gives the host name. A binding or effect that reads it follows the computer being renamed.
func Name(cx *core.Scope) string {
	if cx != nil {
		if name := strings.TrimSpace(cx.Env().DeviceName()); name != "" {
			return name
		}
	}
	name, err := hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(name), ".local")
}
