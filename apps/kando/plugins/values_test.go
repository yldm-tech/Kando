package plugins

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A plugin's settings that are being read while they are saved are replaced once the reader is done with them. Windows refuses to replace a file another handle holds open, as Go's os.ReadFile does without letting it be deleted, so on Windows the save waits for the reader rather than failing, which Kando, saving a setting off the UI thread, would not see, and the setting the user changed would be lost (TestPluginScriptSettingsOfAPluginNotRunning, which reads the file while it waits for the save, timed out on Windows so); elsewhere it replaces the file at once.
func TestValuesBeingReadAreReplacedOnceTheReaderIsDone(t *testing.T) {
	root := t.TempDir()
	if err := SaveValues(root, "acme.anki", Values{"deck": "Mining"}); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(filepath.Join(root, valuesDir, "acme.anki.json"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(100 * time.Millisecond)
		reader.Close()
	}()
	if err := SaveValues(root, "acme.anki", Values{"deck": "Mining", "audio": false}); err != nil {
		t.Fatalf("the save failed while the settings were read: %v", err)
	}
	<-done
	if v, err := LoadValues(root, "acme.anki"); err != nil || v["deck"] != "Mining" || v["audio"] != false {
		t.Fatalf("read back %v, %v", v, err)
	}
}
