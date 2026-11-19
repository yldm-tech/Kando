package main

import (
	"slices"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings"
)

// TestPluginReservedNames checks that plugins.ReservedNames, which the plugins package keeps itself as it imports no interface package, holds Kando's name and the name of every built-in AI service in every language of the interface, so a pack can take none of them.
func TestPluginReservedNames(t *testing.T) {
	for _, tag := range locales.Interface {
		m := locales.In(tag)
		names := []string{m.AppName()}
		for _, preset := range settings.Presets(m) {
			names = append(names, preset.Name)
		}
		for _, name := range names {
			if !slices.Contains(plugins.ReservedNames, name) {
				t.Errorf("%s: %q is not in plugins.ReservedNames", tag, name)
			}
		}
	}
}
