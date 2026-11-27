package media

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Kit is a program Kando downloads for itself when the computer does not have it: yt-dlp from its own releases, ffmpeg and whisper-cli from the Tools workflow's release (.github/workflows/tools.yml), and Deno, the JavaScript runtime yt-dlp solves YouTube's challenges with (Tools.Deno), from Deno's own releases. Each lives in a folder of its own under OwnToolsDir, named by ID.
type Kit struct {
	// ID is the kit's folder: "yt-dlp", "ffmpeg", "whisper" or "deno".
	ID string
	// Program is the program Kando runs from the folder, as lookTool names it.
	Program string
	Version string
	URL     string
	// SHA256 is the checksum of what URL serves; a download that does not match it is thrown away.
	SHA256 string
	// Size is how many bytes URL serves, for the progress shown before the server says.
	Size int64
	// Archive is set for a .tar.gz unpacked into the folder, and Zip for a .zip, as Deno publishes its builds; with neither URL serves the program itself.
	Archive bool
	Zip     bool
	// Fallback is set for a kit Kando runs only when the computer has no copy of the program that works, and downloads only then. Without it the kit is Kando's own copy of choice: downloaded even when the computer has one, and run before it (Candidates), so a package manager that upgrades what the computer's copy is built on cannot break Kando. On Linux, ffmpeg and whisper-cli are fallbacks: the distribution's packages keep their libraries in step, the computer's ffmpeg usually encodes H.264 (libx264) where the kit's LGPL build may not, and its whisper-cli may run on the graphics card where the kit's runs on the processor.
	Fallback bool
	// Updates is set for a kit whose program updates itself in place, as the standalone yt-dlp does with -U: a later Version of the kit does not replace it, as it may be older than what the program updated itself to.
	Updates bool
}

// OwnToolsDir is the folder that holds the kits Kando downloaded. main sets it before the tools are looked up; lookTool prefers what is there to what is on PATH. Empty, as in tests, Kando has no folder of its own.
var OwnToolsDir string

// toolsRelease is where the Tools workflow publishes the ffmpeg and whisper-cli archives. Its tag is TOOLS_RELEASE of .github/tools.env.
const toolsRelease = "https://github.com/yldm-tech/Kando/releases/download/tools-1/"

const (
	ytdlpVersion   = "2026.11.23"
	ffmpegVersion  = "9.0.2"
	whisperVersion = "1.9.5"
	denoVersion    = "2.9.8"
)

// ytdlpRelease is the yt-dlp release the kit starts from. Once downloaded it is the standalone build, which 更新并重试 updates in place with -U (FindYTDLPInstall).
const ytdlpRelease = "https://github.com/yt-dlp/yt-dlp/releases/download/" + ytdlpVersion + "/"

// denoRelease is the Deno release the deno kit is. YouTube serves its videos' addresses only to a client that solves a challenge in JavaScript, which yt-dlp hands to a JavaScript runtime with the solver scripts its standalone build carries (https://github.com/yt-dlp/yt-dlp/wiki/EJS); a signed-in download, which yt-dlp asks of YouTube's web clients, finds no formats without it. Deno is the runtime yt-dlp enables by default. The kit is a fallback: the computer's Deno, as Homebrew's, is run when there is one.
const denoRelease = "https://github.com/denoland/deno/releases/download/v" + denoVersion + "/"

// kits is every kit by platform, as GOOS-GOARCH. A platform not listed, such as Windows, gets none, and its user installs the programs.
var kits = map[string][]Kit{
	"darwin-arm64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_macos", SHA256: "57654854886d0398a798895aadf5173e2749fbb5e249b9f3728022ab376e11fb", Size: 37478576, Updates: true},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-darwin-arm64.tar.gz", SHA256: "4f5b8adcee835fc19730d0825bfb08d0571275d64299794096c6a62fea481b02", Size: 21843581, Archive: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-darwin-arm64.tar.gz", SHA256: "68c198fe26d55f9e896b3954b44331a09217eae0688e9c476344735f86ffde5b", Size: 2076700, Archive: true},
		{ID: "deno", Program: "deno", Version: denoVersion, URL: denoRelease + "deno-aarch64-apple-darwin.zip", SHA256: "9f0bf660393da2eaff044cdc7444cb08c73f235849b61d224d825a194ce7c98e", Size: 38770795, Zip: true, Fallback: true},
	},
	"darwin-amd64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_macos", SHA256: "57654854886d0398a798895aadf5173e2749fbb5e249b9f3728022ab376e11fb", Size: 37478576, Updates: true},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-darwin-amd64.tar.gz", SHA256: "a4039acf7c791771ea69833f210e81206741e866be194b5af8b482e5e61215a9", Size: 24730159, Archive: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-darwin-amd64.tar.gz", SHA256: "7fbb244309fa704bd9f9b4d60d9544372545ea5faa3d1bf05220593432912ae0", Size: 2367533, Archive: true},
		{ID: "deno", Program: "deno", Version: denoVersion, URL: denoRelease + "deno-x86_64-apple-darwin.zip", SHA256: "4b2f5edd2639b7470f034238b321d7002d8dfb4726d8558f52c17afac370eb2c", Size: 42797031, Zip: true, Fallback: true},
	},
	"linux-amd64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_linux", SHA256: "249ddb058234276bc449b9cd9492aad9f133bda5923c61a9e39983ae114ef17d", Size: 41184064, Updates: true},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-linux-amd64.tar.gz", SHA256: "c8c7c08510b22177854931c685a83fe24188b6f0eaff831c3aaddd3fef8743a3", Size: 58572283, Archive: true, Fallback: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-linux-amd64.tar.gz", SHA256: "852ec0ed30acda5ab8be6f9e821d3a49f9a5dd1387de9d63a9d0c3a7ecf4a7b3", Size: 7272399, Archive: true, Fallback: true},
		{ID: "deno", Program: "deno", Version: denoVersion, URL: denoRelease + "deno-x86_64-unknown-linux-gnu.zip", SHA256: "e890975bc2549807bbcc0b584ffafda72cc7c93349dd99c80483dd784f6c4609", Size: 41963064, Zip: true, Fallback: true},
	},
	"linux-arm64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_linux_aarch64", SHA256: "fb847a2e4a9ddc8cf5fcfb4bb7f42ba8969ecf72068def0278074f9d300bd13c", Size: 40807480, Updates: true},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-linux-arm64.tar.gz", SHA256: "2a5a384c8e79e97e0f35b7915ce7d81295bf3168230aaab26dd3946655f68c79", Size: 53741884, Archive: true, Fallback: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-linux-arm64.tar.gz", SHA256: "7273286ed3ba373477e394609a9856482a67c588829f09a349888bec2fb8ac71", Size: 1172279, Archive: true, Fallback: true},
		{ID: "deno", Program: "deno", Version: denoVersion, URL: denoRelease + "deno-aarch64-unknown-linux-gnu.zip", SHA256: "d49596c20b9788cf8f5d8ddf644103d4bde56ed239753f0bdb92ba9361d51905", Size: 40120596, Zip: true, Fallback: true},
	},
}

// KitFor is the kit that provides program on this computer's platform.
func KitFor(program string) (Kit, bool) {
	return kitOn(runtime.GOOS+"-"+runtime.GOARCH, program)
}

func kitOn(platform, program string) (Kit, bool) {
	for _, kit := range kits[platform] {
		if kit.Program == program {
			return kit, true
		}
	}
	return Kit{}, false
}

// Source is where the user gets a program that Kando downloads no kit of on this platform, as on Windows (KitFor).
type Source struct {
	// Page is the program's own page of builds for the platform, or of how to install it.
	Page string
	// Scoop is the name Scoop's main bucket installs it by (scoop install <name>), on Windows; "" elsewhere.
	Scoop string
}

// SourceFor is where the user gets program on this computer's platform; false for a program Kando names no source of.
func SourceFor(program string) (Source, bool) { return sourceOn(runtime.GOOS, program) }

func sourceOn(goos, program string) (Source, bool) {
	if goos == "windows" {
		// yt-dlp.exe is on its releases page; ffmpeg.org sends Windows users to gyan.dev's builds; whisper.cpp publishes whisper-bin-x64.zip and the others in its numbered releases, not the one its latest version is tagged with.
		switch program {
		case "yt-dlp":
			return Source{Page: "https://github.com/yt-dlp/yt-dlp/releases/latest", Scoop: "yt-dlp"}, true
		case "ffmpeg":
			return Source{Page: "https://www.gyan.dev/ffmpeg/builds/", Scoop: "ffmpeg"}, true
		case "whisper-cli":
			return Source{Page: "https://github.com/ggml-org/whisper.cpp/releases", Scoop: "whisper-cpp"}, true
		}
		return Source{}, false
	}
	switch program {
	case "yt-dlp":
		return Source{Page: "https://github.com/yt-dlp/yt-dlp/wiki/Installation"}, true
	case "ffmpeg":
		return Source{Page: "https://ffmpeg.org/download.html"}, true
	case "whisper-cli":
		return Source{Page: "https://github.com/ggml-org/whisper.cpp"}, true
	}
	return Source{}, false
}

// OwnTool is program in the folder of the kit that provides it, or "" when Kando has not downloaded it.
func OwnTool(program string) string {
	if OwnToolsDir == "" {
		return ""
	}
	kit, ok := KitFor(program)
	if !ok {
		return ""
	}
	path := filepath.Join(OwnToolsDir, kit.ID, program)
	if !isExecutable(path) {
		return ""
	}
	return path
}

// kitVersionFile is the file in a kit's folder that says which Version of the kit InstallKit put there.
const kitVersionFile = ".kit-version"

// KitStale reports whether the copy of kit Kando downloaded is of an earlier Version than this Kando pins, so it downloads it again. A copy downloaded before Kando wrote the version beside it is taken for the one pinned, as the pins have not changed since; one of a kit that Updates itself is never stale.
func KitStale(kit Kit) bool {
	if OwnToolsDir == "" || kit.Updates {
		return false
	}
	data, err := os.ReadFile(filepath.Join(OwnToolsDir, kit.ID, kitVersionFile))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) != kit.Version
}

// InstallKit downloads kit into its folder under dir, checks it against its checksum and returns the program's path. The folder is replaced only once the download is complete and checked, so a canceled or failed one leaves what was there.
func InstallKit(ctx context.Context, kit Kit, dir string, progress func(written, total int64)) (string, error) {
	if kit.SHA256 == "" {
		return "", fmt.Errorf("%s: no checksum pinned", kit.ID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(dir, "."+kit.ID+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	download := filepath.Join(staging, ".download")
	if err := fetchChecked(ctx, kit, download, progress); err != nil {
		return "", err
	}
	unpacked := filepath.Join(staging, kit.ID)
	switch {
	case kit.Archive:
		if err := untarGz(download, unpacked); err != nil {
			return "", fmt.Errorf("%s: %w", kit.ID, err)
		}
	case kit.Zip:
		if err := unzipTo(download, unpacked); err != nil {
			return "", fmt.Errorf("%s: %w", kit.ID, err)
		}
	default:
		if err := os.Mkdir(unpacked, 0o755); err != nil {
			return "", err
		}
		if err := os.Rename(download, filepath.Join(unpacked, kit.Program)); err != nil {
			return "", err
		}
	}
	program := filepath.Join(unpacked, kit.Program)
	if err := os.Chmod(program, 0o755); err != nil {
		return "", fmt.Errorf("%s: %s is not in the download", kit.ID, kit.Program)
	}
	if err := os.WriteFile(filepath.Join(unpacked, kitVersionFile), []byte(kit.Version+"\n"), 0o644); err != nil {
		return "", err
	}
	final := filepath.Join(dir, kit.ID)
	if err := os.RemoveAll(final); err != nil {
		return "", err
	}
	if err := os.Rename(unpacked, final); err != nil {
		return "", err
	}
	return filepath.Join(final, kit.Program), nil
}

// fetchChecked writes what kit.URL serves to dest and fails unless its SHA-256 is kit.SHA256. A server that stops sending fails it with ErrStalled (downloadGet); a kit is small enough to download again whole, so nothing is resumed.
func fetchChecked(ctx context.Context, kit Kit, dest string, progress func(written, total int64)) error {
	resp, err := downloadGet(ctx, kit.URL, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &downloadStatusError{name: kit.ID, code: resp.StatusCode}
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	total := resp.ContentLength
	if total <= 0 {
		total = kit.Size
	}
	sum := sha256.New()
	if _, err := copyProgress(io.MultiWriter(out, sum), resp.Body, 0, total, progress); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != kit.SHA256 {
		return fmt.Errorf("download %s: checksum %s, want %s", kit.ID, got, kit.SHA256)
	}
	return nil
}

// errUnsafeEntry is the unpackers' error for an entry that is not a plain file or folder inside the archive.
var errUnsafeEntry = errors.New("archive entry outside its folder")

// untarGz unpacks the regular files and folders of the .tar.gz at src into dir, keeping whether each file may run.
func untarGz(src, dir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, skip, err := entryTarget(dir, h.Name)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(target, tr, h.Mode&0o111 != 0); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: %s", errUnsafeEntry, h.Name)
		}
	}
}

// unzipTo unpacks the regular files and folders of the .zip at src into dir, keeping whether each file may run, as untarGz does a .tar.gz.
func unzipTo(src, dir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		target, skip, err := entryTarget(dir, f.Name)
		if err != nil {
			return err
		}
		if skip {
			continue
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case mode.IsRegular():
			r, err := f.Open()
			if err != nil {
				return err
			}
			err = writeEntry(target, r, mode&0o111 != 0)
			r.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: %s", errUnsafeEntry, f.Name)
		}
	}
	return nil
}

// entryTarget is where an archive's entry called name goes under dir; skip is set for the archive's own top folder ("." or "./"). A name that would go outside dir is errUnsafeEntry.
func entryTarget(dir, name string) (target string, skip bool, err error) {
	clean := path.Clean(strings.TrimPrefix(name, "./"))
	if clean == "." {
		return "", true, nil
	}
	// filepath.IsLocal judges the name as this system reads paths. On Windows a backslash is a separator too, so ..\evil, which a check of slashes alone let through and filepath.Join put beside dir, is refused, as are names with a drive letter or a reserved device name such as NUL.
	local := filepath.FromSlash(clean)
	if !filepath.IsLocal(local) {
		return "", false, fmt.Errorf("%w: %s", errUnsafeEntry, name)
	}
	return filepath.Join(dir, local), false, nil
}

// writeEntry writes a regular file of an archive to target, which must not be there yet, as one that may run when executable is set.
func writeEntry(target string, r io.Reader, executable bool) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if executable {
		mode = 0o755
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
