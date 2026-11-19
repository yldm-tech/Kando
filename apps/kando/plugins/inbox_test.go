package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDropAndInbox checks that Drop copies a pack file into the inbox under a name of its own, which Stage then installs from, and that Inbox lists the drops oldest first, leaving out a drop still being written and a folder.
func TestDropAndInbox(t *testing.T) {
	root := t.TempDir()
	if files, err := Inbox(root); err != nil || len(files) != 0 {
		t.Fatalf("an inbox never made: %q, %v", files, err)
	}
	src := makeZip(t, []entry{{name: ManifestName, content: glossaryManifest}})
	first, err := Drop(root, src)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Drop(root, src)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Dir(first) != filepath.Join(root, InboxName) || !strings.HasSuffix(first, "-pack.zip") {
		t.Fatalf("drops %q and %q", first, second)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(second, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, InboxName, ".drop-123"), []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, InboxName, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := Inbox(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != second || files[1] != first {
		t.Fatalf("inbox %q, want %q then %q", files, second, first)
	}
	staged, p, err := Stage(root, files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
	if packs, issues := Scan(root); len(packs) != 1 || len(issues) != 0 {
		t.Fatalf("scan after installing from the inbox: %d packs, issues %v", len(packs), issues)
	}
}

// TestDropRefusesWhatIsNoFile checks that Drop takes a regular file alone: a folder or a missing file is refused and leaves the inbox empty.
func TestDropRefusesWhatIsNoFile(t *testing.T) {
	root := t.TempDir()
	if _, err := Drop(root, t.TempDir()); err == nil {
		t.Error("a folder was dropped")
	}
	if _, err := Drop(root, filepath.Join(t.TempDir(), "missing.zip")); err == nil {
		t.Error("a missing file was dropped")
	}
	if files, _ := Inbox(root); len(files) != 0 {
		t.Errorf("inbox %q after refused drops", files)
	}
}

// TestInboxBase checks the name a drop keeps of its file: its base name in plain ASCII.
func TestInboxBase(t *testing.T) {
	for src, want := range map[string]string{
		"/x/pack.zip":      "pack.zip",
		"/x/動画 terms.zip":  "___terms.zip",
		"/x/a;b$(c).kando": "a_b__c_.kando",
	} {
		if got := inboxBase(src); got != want {
			t.Errorf("inboxBase(%q) = %q, want %q", src, got, want)
		}
	}
}
