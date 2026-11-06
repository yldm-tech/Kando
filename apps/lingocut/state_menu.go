package main

// menuUI is the state of the app's menu bar.
type menuUI struct {
	// menuBar is set once the app installs its own menu bar. From then on the menu's accelerators run the ⌘ shortcuts and wireKeys leaves them alone, so a shortcut does not run twice.
	menuBar bool
}

func newMenuUI() menuUI { return menuUI{} }

// savedMenu is the menu bar's part of settings.json.
type savedMenu struct{}

func (s *savedMenu) clean() {}

func (a *app) menuHooks() areaHooks { return areaHooks{} }

// menuKeys are the menu bar's own shortcuts.
func (a *app) menuKeys() []shortcut { return nil }
