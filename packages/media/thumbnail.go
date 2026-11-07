package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// maxThumbnail bounds a thumbnail's download; a site's thumbnails are well under a megabyte.
const maxThumbnail = 8 << 20

// FetchThumbnail downloads the picture at address, a video's thumbnail as its site names it, to dest. It writes dest only once the whole picture has arrived.
func FetchThumbnail(ctx context.Context, address, dest string) error {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("thumbnail address %q is not a web address", address)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("thumbnail: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxThumbnail+1))
	if err != nil {
		return err
	}
	if len(data) > maxThumbnail {
		return errors.New("thumbnail is too large")
	}
	if len(data) == 0 {
		return errors.New("thumbnail is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
