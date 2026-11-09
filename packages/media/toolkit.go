package media

import (
	"archive/tar"
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

// Kit is a program Kando downloads for itself when the computer does not have it: yt-dlp from its own releases, ffmpeg and whisper-cli from the Tools workflow's release (.github/workflows/tools.yml). Each lives in a folder of its own under OwnToolsDir, named by ID.
type Kit struct {
	// ID is the kit's folder: "yt-dlp", "ffmpeg" or "whisper".
	ID string
	// Program is the program Kando runs from the folder, as lookTool names it.
	Program string
	Version string
	URL     string
	// SHA256 is the checksum of what URL serves; a download that does not match it is thrown away.
	SHA256 string
	// Size is how many bytes URL serves, for the progress shown before the server says.
	Size int64
	// Archive is set for a .tar.gz unpacked into the folder; otherwise URL serves the program itself.
	Archive bool
}

// OwnToolsDir is the folder that holds the kits Kando downloaded. main sets it before the tools are looked up; lookTool prefers what is there to what is on PATH. Empty, as in tests, Kando has no folder of its own.
var OwnToolsDir string

// toolsRelease is where the Tools workflow publishes the ffmpeg and whisper-cli archives. Its tag is TOOLS_RELEASE of .github/tools.env.
const toolsRelease = "https://github.com/yldm-tech/Kando/releases/download/tools-1/"

const (
	ytdlpVersion   = "2026.10.26"
	ffmpegVersion  = "9.0.2"
	whisperVersion = "1.9.5"
)

// ytdlpRelease is the yt-dlp release the kit starts from. Once downloaded it is the standalone build, which 更新并重试 updates in place with -U (FindYTDLPInstall).
const ytdlpRelease = "https://github.com/yt-dlp/yt-dlp/releases/download/" + ytdlpVersion + "/"

// kits is every kit by platform, as GOOS-GOARCH. A platform not listed, such as Windows, gets none, and its user installs the programs.
var kits = map[string][]Kit{
	"darwin-arm64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_macos", SHA256: "0f922c15ccc0b83eefa1c8bc9104e434318c4bf16e408e1ee93b7745a6efe0aa", Size: 37466304},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-darwin-arm64.tar.gz", Archive: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-darwin-arm64.tar.gz", Archive: true},
	},
	"darwin-amd64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_macos", SHA256: "0f922c15ccc0b83eefa1c8bc9104e434318c4bf16e408e1ee93b7745a6efe0aa", Size: 37466304},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-darwin-amd64.tar.gz", Archive: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-darwin-amd64.tar.gz", Archive: true},
	},
	"linux-amd64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_linux", SHA256: "767ba9dce7052aa196f8b12652f433d76de128ba168966fa84b6394d5e6c6795", Size: 41166968},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-linux-amd64.tar.gz", Archive: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-linux-amd64.tar.gz", Archive: true},
	},
	"linux-arm64": {
		{ID: "yt-dlp", Program: "yt-dlp", Version: ytdlpVersion, URL: ytdlpRelease + "yt-dlp_linux_aarch64", SHA256: "e87e9533e89fe5889abaa21f5114dfe7756d6aa6219b83551add04d48b18e483", Size: 40789792},
		{ID: "ffmpeg", Program: "ffmpeg", Version: ffmpegVersion, URL: toolsRelease + "ffmpeg-" + ffmpegVersion + "-linux-arm64.tar.gz", Archive: true},
		{ID: "whisper", Program: "whisper-cli", Version: whisperVersion, URL: toolsRelease + "whisper-" + whisperVersion + "-linux-arm64.tar.gz", Archive: true},
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

// ownTool is program in the folder of the kit that provides it, or "" when Kando has not downloaded it.
func ownTool(program string) string {
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
	if kit.Archive {
		if err := untarGz(download, unpacked); err != nil {
			return "", fmt.Errorf("%s: %w", kit.ID, err)
		}
	} else {
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
	final := filepath.Join(dir, kit.ID)
	if err := os.RemoveAll(final); err != nil {
		return "", err
	}
	if err := os.Rename(unpacked, final); err != nil {
		return "", err
	}
	return filepath.Join(final, kit.Program), nil
}

// fetchChecked writes what kit.URL serves to dest and fails unless its SHA-256 is kit.SHA256.
func fetchChecked(ctx context.Context, kit Kit, dest string, progress func(written, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kit.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Kando")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", kit.ID, resp.StatusCode)
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
	buf := make([]byte, 256*1024)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
			sum.Write(buf[:n])
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != kit.SHA256 {
		return fmt.Errorf("download %s: checksum %s, want %s", kit.ID, got, kit.SHA256)
	}
	return nil
}

// errUnsafeEntry is untarGz's error for an entry that is not a plain file or folder inside the archive.
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
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("%w: %s", errUnsafeEntry, h.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: %s", errUnsafeEntry, h.Name)
		}
	}
}
