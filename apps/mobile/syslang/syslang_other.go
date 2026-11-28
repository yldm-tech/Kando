//go:build !ios

package syslang

// Keep does nothing where the system keeps no language of the app's own in its defaults for NSGo to write over: Android's per-app language NSGo keeps apart from its own (LocaleManager), and the other platforms have none.
func Keep(string) {}
