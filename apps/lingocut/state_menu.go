package main

// menuUI is the state of the app's menu bar.
type menuUI struct {
	// menuBar is set once the app installs its own menu bar. From then on the menu's accelerators run the ⌘ shortcuts and wireKeys leaves them alone, so a shortcut does not run twice.
	menuBar bool
}
