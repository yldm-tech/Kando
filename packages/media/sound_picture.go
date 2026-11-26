package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// The covers WideCover reads are a site's or a file's own: JPEG or PNG.
	_ "image/jpeg"
	_ "image/png"
)

// A film of sound alone has no picture to stand for it, so it is drawn from its sound: the waveform of soundSpan of it in Kando's blue on a dark ground, 16:9 as a video's poster is, which the cards, the player's stage and the phone show as any poster. Its ground is bright enough not to count as a black frame (minPosterLuma), so a kept poster is not grabbed again.
const (
	soundSpan   = 30 * time.Second
	soundGround = "0x1C2333"
	soundInk    = "0x6CB4FF"
)

// soundPicture is a picture of the sound of the media at path from at, soundSpan of it, as a JPEG width wide in 16:9.
func soundPicture(ctx context.Context, ffmpeg, path string, at time.Duration, width int) ([]byte, error) {
	height := width * 9 / 16
	wave := height * 5 / 9
	size := func(w, h int) string { return strconv.Itoa(w) + "x" + strconv.Itoa(h) }
	args := []string{"-nostdin", "-loglevel", "error", "-y"}
	if at > 0 {
		args = append(args, "-ss", ffmpegTime(at))
	}
	args = append(args, "-t", ffmpegTime(soundSpan), "-i", path,
		"-filter_complex", "[0:a:0]aformat=channel_layouts=mono,showwavespic=s="+size(width, wave)+":scale=sqrt:draw=full:filter=peak:colors="+soundInk+"[wave];"+
			"color=c="+soundGround+":s="+size(width, height)+"[ground];[ground][wave]overlay=(W-w)/2:(H-h)/2:format=auto",
		"-frames:v", "1", "-f", "image2", "-c:v", "mjpeg", "-q:v", "3", "pipe:1")
	out, err := run(ctx, ffmpeg, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, errors.New("sound picture is empty")
	}
	return []byte(out), nil
}

// Header is what the header of a media file says as ffmpeg -i reads it: how long it plays and whether it has a picture, a video stream that is not an attached picture as an mp3's cover art is. Read is false when ffmpeg could not read the file.
type Header struct {
	Length  time.Duration
	Picture bool
	Read    bool
}

// ReadHeader is the Header of the media file at path, from its header alone, which takes ffmpeg a moment whatever the file's length.
func ReadHeader(ffmpeg, path string) Header {
	return ReadHeaderContext(context.Background(), ffmpeg, path)
}

// ReadHeaderContext is ReadHeader bounded by ctx, as when probing a video served over HTTP.
func ReadHeaderContext(ctx context.Context, ffmpeg, path string) Header {
	if ffmpeg == "" || path == "" {
		return Header{}
	}
	return parseHeader(ffmpegStderr(ctx, ffmpeg, "-hide_banner", "-nostdin", "-i", path))
}

// parseHeader reads what ffmpeg -i writes of a file: "Duration: 00:01:05.00, …" and a line per stream, as "Stream #0:1: Video: mjpeg …, 300x300 … (attached pic)".
func parseHeader(text string) Header {
	h := Header{Length: parseDuration(text)}
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "Stream #") {
			continue
		}
		h.Read = true
		if strings.Contains(line, "Video:") && !strings.Contains(line, "(attached pic)") {
			h.Picture = true
		}
	}
	return h
}

// wideCover is how far a cover's width over its height may be from 16:9, as 16:10 and 2:1 are, before WideCover makes it 16:9.
const (
	wideCoverMin = 1.6
	wideCoverMax = 1.9
)

// WideCover is cover, a picture as JPEG or PNG, as a 16:9 JPEG for a film of sound alone, whose cards and stage show it in place of a picture: a podcast's or an album's art is square, and a card filled with it would cut off its top and bottom, often its title. The whole of it goes in the middle, over a blurred and darkened copy that fills the rest, as players show such art. A picture near 16:9 already is returned as it is.
func WideCover(ctx context.Context, ffmpeg string, cover []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(cover))
	if err != nil {
		return nil, err
	}
	if cfg.Height > 0 {
		if ratio := float64(cfg.Width) / float64(cfg.Height); ratio >= wideCoverMin && ratio <= wideCoverMax {
			return cover, nil
		}
	}
	if ffmpeg == "" {
		return cover, nil
	}
	dir, err := os.MkdirTemp("", "kando-cover-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "cover")
	if err := os.WriteFile(in, cover, 0o644); err != nil {
		return nil, err
	}
	width := soundPosterWidth
	size := strconv.Itoa(width) + ":" + strconv.Itoa(width*9/16)
	out, err := run(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-i", in,
		"-filter_complex", "[0:v]split[a][b];"+
			"[a]scale="+size+":force_original_aspect_ratio=increase,crop="+size+",boxblur=40:2,eq=brightness=-0.25[ground];"+
			"[b]scale="+size+":force_original_aspect_ratio=decrease[art];[ground][art]overlay=(W-w)/2:(H-h)/2",
		"-frames:v", "1", "-f", "image2", "-c:v", "mjpeg", "-q:v", "3", "pipe:1")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, errors.New("wide cover is empty")
	}
	return []byte(out), nil
}
