package desktop

import (
	"errors"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeHostname makes the host name name, or err, for the rest of t.
func fakeHostname(t *testing.T, name string, err error) {
	t.Helper()
	saved := hostname
	hostname = func() (string, error) { return name, err }
	t.Cleanup(func() { hostname = saved })
}

// startNamed runs an app whose one text shows Name, in an environment that names the device device.
func startNamed(t *testing.T, device string) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: func() string { return "name:" + Name(cx) }}
	}, nsgotest.Size(400, 200), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US", DeviceName: device}))
}

// TestNameIsTheDevicesName checks that Name is the name the user gave the computer rather than its host name, and follows the computer being renamed.
func TestNameIsTheDevicesName(t *testing.T) {
	fakeHostname(t, "842f574b78da.local", nil)
	app := startNamed(t, "Ada's MacBook Pro")
	if got, want := app.Texts(), []string{"name:Ada's MacBook Pro"}; !slices.Equal(got, want) {
		t.Fatalf("texts %q, want %q", got, want)
	}
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US", DeviceName: "Ada's Studio"})
	app.Settle()
	if got, want := app.Texts(), []string{"name:Ada's Studio"}; !slices.Equal(got, want) {
		t.Fatalf("after renaming, texts %q, want %q", got, want)
	}
}

// TestNameFallsBackToTheHostName checks that Name is the host name, without .local, where the system tells no device name or before the app's scope exists, and "" when there is no host name either.
func TestNameFallsBackToTheHostName(t *testing.T) {
	fakeHostname(t, "842f574b78da.local", nil)
	if got := Name(nil); got != "842f574b78da" {
		t.Fatalf("Name(nil) = %q, want the host name", got)
	}
	app := startNamed(t, "")
	if got, want := app.Texts(), []string{"name:842f574b78da"}; !slices.Equal(got, want) {
		t.Fatalf("texts %q, want %q", got, want)
	}
	fakeHostname(t, "", errors.New("no host name"))
	if got := Name(nil); got != "" {
		t.Fatalf("Name(nil) without a host name = %q, want \"\"", got)
	}
}
