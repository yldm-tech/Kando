// mygo-bridge.go make|verify
//
// Kando's MyGo builds (0.12.0 and older) update themselves from update-<target>.json in the latest GitHub release, which NSGo releases do not publish: they publish Sparkle appcasts. This tool makes, for each macOS and Windows target of a release, what a MyGo build's updater reads, so that copies installed before Kando moved to NSGo update to the NSGo build once, after which the NSGo build updates itself through its appcast. It writes kando-<version>-<mygo target>.tar.gz, the NSGo app as MyGo's updater unpacks it (the Kando.app bundle on macOS; Kando.exe, WinSparkle.dll and icon.png on Windows, the files of the folder MyGo's installer used, %LOCALAPPDATA%\Programs\Kando, where NSGo's installer installs too), and update-<mygo target>.json, the manifest naming it, with the Ed25519 signature of the archive's SHA-256 digest by the MyGo updater's key (github.com/egoist/mygo v0.9.0, internal/update). Linux is left out: the MyGo builds ran on GTK 3, the NSGo build needs GTK 4 and GStreamer, which a machine set up for the MyGo build may lack, so Linux copies are reinstalled with install.sh instead of being replaced by one that may not start.
//
// Run it with GOWORK=off go run .github/scripts/mygo-bridge.go, as it is a file of its own outside the workspace's modules:
//
//	make -version 0.13.1 -repo yldm-tech/Kando -packages <nsgo build -output> -targets macos_arm64,macos_x64 -notes notes.md -public <nsgo.yaml updates.key>
//	verify -dir <releases folder> -public <nsgo.yaml updates.key>
//
// make takes the 64-byte private key, base64, from the MYGO_UPDATER_PRIVATE_KEY environment variable, refuses one whose public key is not -public, and writes into <packages>/releases, beside the installers nsgo build made there. verify checks every update-*.json in a folder the way the MyGo updater does before it installs: the archive the manifest names is there with its size and signature, and it unpacks to what the target's updater expects.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// manifest is MyGo's update.Manifest, the fields a MyGo build's updater reads; a bridge has no deltas or previous archives.
type manifest struct {
	Version   string `json:"version"`
	Notes     string `json:"notes,omitempty"`
	Date      string `json:"date,omitempty"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

// mygoTargets maps nsgo build's targets to MyGo's, which name the MyGo builds' manifests and archives.
var mygoTargets = map[string]string{
	"macos_arm64":   "darwin-arm64",
	"macos_x64":     "darwin-amd64",
	"windows_x64":   "windows-amd64",
	"windows_arm64": "windows-arm64",
}

// windowsFiles are the files of the NSGo build's folder on Windows that its installer installs: nsgo build's folder also holds the installer's script and icons, which stay out.
var windowsFiles = []string{"Kando.exe", "WinSparkle.dll", "icon.png"}

func main() {
	if len(os.Args) < 2 {
		fail(errors.New("usage: mygo-bridge.go make|verify [flags]"))
	}
	var err error
	switch os.Args[1] {
	case "make":
		err = runMake(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q: make or verify", os.Args[1])
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mygo-bridge:", err)
	os.Exit(1)
}

func runMake(args []string) error {
	flags := flag.NewFlagSet("make", flag.ExitOnError)
	version := flags.String("version", "", "the release's version, without v")
	repo := flags.String("repo", "", "the GitHub repository the release is in, owner/name")
	packages := flags.String("packages", "", "nsgo build's -output folder")
	targets := flags.String("targets", "", "nsgo build's targets to bridge, comma-separated")
	notes := flags.String("notes", "", "a Markdown file of the release notes the MyGo build's update window shows")
	public := flags.String("public", "", "the updates key in nsgo.yaml, the public key the MyGo builds check updates with")
	date := flags.String("date", "", "the build's date, RFC 3339; now by default")
	flags.Parse(args)
	if *version == "" || *repo == "" || *packages == "" || *targets == "" || *public == "" {
		return errors.New("make needs -version, -repo, -packages, -targets and -public")
	}
	key, err := privateKey(os.Getenv("MYGO_UPDATER_PRIVATE_KEY"), *public)
	if err != nil {
		return err
	}
	var text string
	if *notes != "" {
		b, err := os.ReadFile(*notes)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(string(b))
	}
	when := *date
	if when == "" {
		when = time.Now().UTC().Format(time.RFC3339)
	}
	out := filepath.Join(*packages, "releases")
	for _, target := range strings.Split(*targets, ",") {
		mt, ok := mygoTargets[target]
		if !ok {
			return fmt.Errorf("no MyGo build of target %s to bridge", target)
		}
		dir := filepath.Join(*packages, "Kando-"+*version+"-"+target)
		var archive []byte
		if strings.HasPrefix(target, "macos_") {
			archive, err = tarGz(dir, []string{"Kando.app"})
		} else {
			archive, err = tarGz(dir, windowsFiles)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
		name := "kando-" + *version + "-" + mt + ".tar.gz"
		if err := os.WriteFile(filepath.Join(out, name), archive, 0o644); err != nil {
			return err
		}
		digest := sha256.Sum256(archive)
		m := manifest{
			Version:   *version,
			Notes:     text,
			Date:      when,
			URL:       "https://github.com/" + *repo + "/releases/download/v" + *version + "/" + name,
			Size:      int64(len(archive)),
			Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest[:])),
		}
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "update-"+mt+".json"), append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s: %s (%d bytes) and update-%s.json\n", target, name, len(archive), mt)
	}
	return nil
}

// privateKey decodes the MyGo updater's 64-byte private key and checks that its public key is the one the MyGo builds check updates with.
func privateKey(secret, public string) (ed25519.PrivateKey, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("MYGO_UPDATER_PRIVATE_KEY is not set")
	}
	b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(secret), ""))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("MYGO_UPDATER_PRIVATE_KEY is not a base64 Ed25519 private key of 64 bytes")
	}
	key := ed25519.PrivateKey(b)
	pub, err := publicKey(public)
	if err != nil {
		return nil, err
	}
	if !pub.Equal(key.Public()) {
		return nil, errors.New("MYGO_UPDATER_PRIVATE_KEY is not the key of the public key given, so the MyGo builds would refuse its updates")
	}
	return key, nil
}

func publicKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("-public is not a base64 Ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

// tarGz archives the entries names of dir, and everything under them, as MyGo's updater unpacks them: directories, regular files with their permissions and relative symbolic links, with paths relative to dir.
func tarGz(dir string, names []string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		root := filepath.Join(dir, name)
		if _, err := os.Lstat(root); err != nil {
			return nil, err
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			info, err := os.Lstat(p)
			if err != nil {
				return err
			}
			hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: int64(info.Mode().Perm()), ModTime: info.ModTime()}
			switch {
			case info.Mode()&fs.ModeSymlink != 0:
				link, err := os.Readlink(p)
				if err != nil {
					return err
				}
				if path.IsAbs(filepath.ToSlash(link)) {
					return fmt.Errorf("%s links outside the app, to %s", rel, link)
				}
				hdr.Typeflag, hdr.Linkname = tar.TypeSymlink, filepath.ToSlash(link)
			case info.IsDir():
				hdr.Typeflag, hdr.Name = tar.TypeDir, hdr.Name+"/"
			case info.Mode().IsRegular():
				hdr.Typeflag, hdr.Size = tar.TypeReg, info.Size()
			default:
				return fmt.Errorf("%s is neither a file, a folder nor a link", rel)
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if hdr.Typeflag != tar.TypeReg {
				return nil
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func runVerify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ExitOnError)
	dir := flags.String("dir", "", "the folder of the release's files")
	public := flags.String("public", "", "the updates key in nsgo.yaml")
	flags.Parse(args)
	pub, err := publicKey(*public)
	if err != nil {
		return err
	}
	manifests, err := filepath.Glob(filepath.Join(*dir, "update-*.json"))
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		return fmt.Errorf("%s holds no update-*.json", *dir)
	}
	for _, p := range manifests {
		if err := verify(p, pub); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		fmt.Println(filepath.Base(p), "ok")
	}
	return nil
}

// verify checks a manifest as a MyGo build's updater does before it installs it (checkUpdate, download and installUpdate in github.com/egoist/mygo v0.9.0): a whole manifest, an archive of its size whose digest its signature signs, and an archive that unpacks to what the target's update replaces.
func verify(p string, pub ed25519.PublicKey) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if m.Version == "" || m.URL == "" || m.Signature == "" || m.Size <= 0 {
		return errors.New("incomplete manifest")
	}
	if _, err := time.Parse(time.RFC3339, m.Date); err != nil {
		return fmt.Errorf("date %q is not RFC 3339", m.Date)
	}
	archive, err := os.ReadFile(filepath.Join(filepath.Dir(p), path.Base(m.URL)))
	if err != nil {
		return err
	}
	if int64(len(archive)) != m.Size {
		return fmt.Errorf("the archive is %d bytes, the manifest says %d", len(archive), m.Size)
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	digest := sha256.Sum256(archive)
	if err != nil || !ed25519.Verify(pub, digest[:], sig) {
		return errors.New("the archive is not signed with the MyGo builds' key")
	}
	roots, err := archiveRoots(archive)
	if err != nil {
		return err
	}
	target := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "update-"), ".json")
	switch {
	case strings.HasPrefix(target, "darwin-"):
		// stageBundle: the update holds one entry, an app bundle.
		if len(roots) != 1 || roots[0] != "Kando.app" {
			return fmt.Errorf("the archive holds %q, not the Kando.app bundle alone", roots)
		}
	case strings.HasPrefix(target, "windows-"):
		// swapFiles keeps the executable the app runs from, Kando.exe, and its shortcuts start it.
		if !slices.Equal(roots, windowsFiles) {
			return fmt.Errorf("the archive holds %q, not %q", roots, windowsFiles)
		}
	default:
		return fmt.Errorf("no bridge for target %s", target)
	}
	return nil
}

// archiveRoots returns the top-level entries of a .tar.gz, in its order, checking that no entry leaves the folder it unpacks in, as MyGo's ExtractArchive refuses.
func archiveRoots(archive []byte) ([]string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	var roots []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return roots, nil
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(hdr.Name, "/")
		if !fs.ValidPath(name) {
			return nil, fmt.Errorf("entry %q leaves the app", hdr.Name)
		}
		if hdr.Typeflag == tar.TypeSymlink && (path.IsAbs(hdr.Linkname) || !fs.ValidPath(path.Join(path.Dir(name), hdr.Linkname))) {
			return nil, fmt.Errorf("link %q points outside the app", hdr.Name)
		}
		root, _, _ := strings.Cut(name, "/")
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
}
