// Command extgen writes the browser extension's files that come from the page buttons' code and data (browse.ExtensionFiles) into apps/kando/resources/browser-extension, for the version of apps/kando/nsgo.yaml. go generate runs it in apps/kando/browse.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"kando/apps/kando/browse"
	"kando/apps/kando/browserext"
)

func main() {
	if err := run(".."); err != nil {
		fmt.Fprintln(os.Stderr, "extgen:", err)
		os.Exit(1)
	}
}

// run writes the files under app, the folder of nsgo.yaml.
func run(app string) error {
	data, err := os.ReadFile(filepath.Join(app, "nsgo.yaml"))
	if err != nil {
		return err
	}
	version, err := browse.AppVersion(data)
	if err != nil {
		return err
	}
	dir := filepath.Join(app, "resources", browserext.FolderName)
	for name, data := range browse.ExtensionFiles(version) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
