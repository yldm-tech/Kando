// Command official is what .github/scripts/official-plugins.sh, plugin-revocations.sh and the Release workflow run around nsgo's signing (docs/plugin-system.md 15, P3c). It never sees a secret key: nsgo publish and nsgo revoke sign, with keys only the owner's environment holds.
//
//	manifest -src <plugin folder> -out <file>
//	wrap -src <plugin folder> -package <file.nsgopkg> -dir <folder> [-root <key>]
//	revocations -list <file> [-previous <file>] [-root <key>] [-warn-days <n>]
//	revocations -list <file> -next
//
// manifest writes into -out what the package of the plugin in -src signs of its manifest (signature.SignedManifest), for nsgo publish -extra, and prints -out, when no Kando its kando-plugin.json says it runs on refuses it (signature.SignsManifest); otherwise it writes and prints nothing. wrap checks that the package nsgo publish signed of the plugin in -src chains to the plugin root and is the plugin its kando-plugin.json says, and writes <id>-<version>.kandoplugin into -dir (official.Wrap), refusing to replace a file already there. revocations checks a revocation list before a release publishes it (official.CheckRevocations), warning when it expires within -warn-days; with -next it prints, a line each, the sequence the next list takes and the release keys and packages the list revokes now, comma-separated, which the next one must revoke again, as a newer list replaces the older one whole (1 and two empty lines when there is no list yet). -root is the plugin root's public key, signature.OfficialRoot unless a test gives its own.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/official"
	"kando/apps/kando/plugins/script/signature"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "manifest":
		err = manifest(os.Args[2:])
	case "wrap":
		err = wrap(os.Args[2:])
	case "revocations":
		err = revocations(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "official:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: official manifest -src <plugin folder> -out <file>\n       official wrap -src <plugin folder> -package <file.nsgopkg> -dir <folder> [-root <key>]\n       official revocations -list <file> [-previous <file>] [-root <key>] [-warn-days <n>] [-next]")
	os.Exit(2)
}

func manifest(args []string) error {
	flags := flag.NewFlagSet("manifest", flag.ExitOnError)
	src := flags.String("src", "", "the plugin's source `folder`, with its kando-plugin.json")
	out := flags.String("out", "", "the `file` to write what the package signs of the manifest into")
	flags.Parse(args)
	if *src == "" || *out == "" {
		usage()
	}
	data, err := os.ReadFile(filepath.Join(*src, "kando-plugin.json"))
	if err != nil {
		return err
	}
	m, err := plugins.Parse(data)
	if err != nil {
		return err
	}
	if !signature.SignsManifest(m) {
		return nil
	}
	signed, err := signature.SignedManifest(m)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, signed, 0o644); err != nil {
		return err
	}
	fmt.Println(*out)
	return nil
}

func wrap(args []string) error {
	flags := flag.NewFlagSet("wrap", flag.ExitOnError)
	src := flags.String("src", "", "the plugin's source `folder`, with its kando-plugin.json")
	pkgFile := flags.String("package", "", "the `file` nsgo publish signed of it")
	dir := flags.String("dir", "", "the `folder` to write <id>-<version>.kandoplugin into")
	rootKey := flags.String("root", signature.OfficialRoot, "the plugin root's public `key`")
	flags.Parse(args)
	if *src == "" || *pkgFile == "" || *dir == "" {
		usage()
	}
	root, err := pkg.DecodePublicKey(*rootKey)
	if err != nil {
		return fmt.Errorf("-root: %w", err)
	}
	manifest, err := os.ReadFile(filepath.Join(*src, "kando-plugin.json"))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*pkgFile)
	if err != nil {
		return err
	}
	if _, err := official.VerifyPackage(data, root); err != nil {
		return fmt.Errorf("%s does not verify against the plugin root: %w", *pkgFile, err)
	}
	out, id, version, err := official.Wrap(manifest, filepath.Base(*pkgFile), data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(*dir, official.FileName(id, version))
	// A published file is never replaced: a Kando that cached an older index checks it by its SHA-256. A new version has a new name.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s is there already: publish a new version of the plugin rather than replacing it", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(out); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	fmt.Println(path)
	return nil
}

func revocations(args []string) error {
	flags := flag.NewFlagSet("revocations", flag.ExitOnError)
	list := flags.String("list", "", "the revocation list `file`")
	previous := flags.String("previous", "", "the list the latest release published, `file`")
	rootKey := flags.String("root", signature.OfficialRoot, "the plugin root's public `key`")
	warnDays := flags.Int("warn-days", 30, "warn when the list expires within this many `days`")
	next := flags.Bool("next", false, "print the next list's sequence and what the list revokes")
	flags.Parse(args)
	if *list == "" {
		usage()
	}
	root, err := pkg.DecodePublicKey(*rootKey)
	if err != nil {
		return fmt.Errorf("-root: %w", err)
	}
	data, err := os.ReadFile(*list)
	if *next && errors.Is(err, fs.ErrNotExist) {
		fmt.Print("1\n\n\n")
		return nil
	}
	if err != nil {
		return err
	}
	var prev []byte
	if *previous != "" {
		if prev, err = os.ReadFile(*previous); err != nil {
			return err
		}
	}
	now := time.Now()
	info, err := official.CheckRevocations(data, prev, root, now)
	if *next {
		// The next list may follow one that expired: renewing it is what it is for.
		if err != nil && !errors.Is(err, official.ErrExpired) {
			return err
		}
		fmt.Printf("%d\n%s\n%s\n", info.Sequence+1, strings.Join(info.Keys, ","), strings.Join(info.Packages, ","))
		return nil
	}
	if err != nil {
		return err
	}
	expires := "never expires"
	if !info.NotAfter.IsZero() {
		expires = "expires " + info.NotAfter.UTC().Format(time.DateOnly)
		if info.NotAfter.Sub(now) < time.Duration(*warnDays)*24*time.Hour {
			// A GitHub Actions annotation where the Release workflow runs it, a plain line elsewhere.
			fmt.Printf("::warning::The plugin revocation list %d expires %s: sign a newer one (.github/scripts/plugin-revocations.sh) before then, or Kando pauses the plugins of other publishers that reach the network.\n", info.Sequence, info.NotAfter.UTC().Format(time.DateOnly))
		}
	}
	fmt.Printf("revocation list %d, %d keys and %d packages revoked, %s\n", info.Sequence, len(info.Keys), len(info.Packages), expires)
	return nil
}
