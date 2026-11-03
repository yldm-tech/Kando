package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const frameMaxWidth = 960

var (
	sizePattern = regexp.MustCompile(`\b(\d{2,5})x(\d{2,5})\b`)
	fpsPattern  = regexp.MustCompile(`([\d.]+) fps`)
)

// FrameReader streams a video's pictures. Next draws every frame into the same image; the caller copies it before the next Next.
type FrameReader struct {
	ffmpeg   string
	path     string
	frame    time.Duration
	width    int
	height   int
	buf      []byte
	pending  *image.RGBA
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	cancel   context.CancelFunc
	closeNow sync.Once
}

// OpenFrames starts ffmpeg and reads the first picture, so the caller learns a bad file before playback.
func OpenFrames(ffmpeg, path string) (*FrameReader, error) {
	if ffmpeg == "" || path == "" {
		return nil, errors.New("video source is missing")
	}
	width, height, frame, err := probePicture(ffmpeg, path)
	if err != nil {
		return nil, err
	}
	reader, err := startFrames(ffmpeg, path, width, height, frame)
	if err != nil {
		return nil, err
	}
	if err := reader.read(); err != nil {
		reader.Close()
		return nil, err
	}
	reader.pending = reader.picture()
	return reader, nil
}

func (r *FrameReader) Next() (image.Image, time.Duration, error) {
	if r.pending != nil {
		img := r.pending
		r.pending = nil
		return img, r.frame, nil
	}
	if err := r.read(); err != nil {
		return nil, 0, err
	}
	return r.picture(), r.frame, nil
}

// Rewind starts the video from its first picture again.
func (r *FrameReader) Rewind() error {
	r.stopProcess()
	next, err := startFrames(r.ffmpeg, r.path, r.width, r.height, r.frame)
	if err != nil {
		return err
	}
	r.cmd = next.cmd
	r.stdout = next.stdout
	r.cancel = next.cancel
	r.buf = next.buf
	if err := r.read(); err != nil {
		return err
	}
	r.pending = r.picture()
	return nil
}

// Close stops ffmpeg. It is safe to call more than once.
func (r *FrameReader) Close() error {
	r.closeNow.Do(r.stopProcess)
	return nil
}

func (r *FrameReader) stopProcess() {
	if r.cancel != nil {
		r.cancel()
	}
	if r.stdout != nil {
		r.stdout.Close()
	}
	if r.cmd != nil {
		r.cmd.Wait()
	}
}

func (r *FrameReader) read() error {
	_, err := io.ReadFull(r.stdout, r.buf)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return io.EOF
	}
	return err
}

func (r *FrameReader) picture() *image.RGBA {
	return &image.RGBA{Pix: r.buf, Stride: r.width * 4, Rect: image.Rect(0, 0, r.width, r.height)}
}

func startFrames(ffmpeg, path string, width, height int, frame time.Duration) (*FrameReader, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-nostdin", "-loglevel", "error",
		"-i", path,
		"-map", "0:v:0", "-an",
		"-vf", "scale="+strconv.Itoa(width)+":"+strconv.Itoa(height),
		"-f", "rawvideo", "-pix_fmt", "rgba",
		"pipe:1",
	)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return &FrameReader{
		ffmpeg: ffmpeg, path: path, frame: frame, width: width, height: height,
		buf: make([]byte, width*height*4),
		cmd: cmd, stdout: stdout, cancel: cancel,
	}, nil
}

func probePicture(ffmpeg, path string) (width, height int, frame time.Duration, err error) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-nostdin", "-i", path)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	_ = cmd.Run()
	text := stderr.String()
	var video string
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "Video:") {
			video = line
			break
		}
	}
	if video == "" {
		return 0, 0, 0, errors.New("video has no picture")
	}
	size := sizePattern.FindStringSubmatch(video)
	if size == nil {
		return 0, 0, 0, errors.New("video size is missing")
	}
	width, _ = strconv.Atoi(size[1])
	height, _ = strconv.Atoi(size[2])
	if width < 2 || height < 2 {
		return 0, 0, 0, errors.New("video size is missing")
	}
	width, height = fitFrame(width, height)
	fps := 25.0
	if match := fpsPattern.FindStringSubmatch(video); match != nil {
		if parsed, convErr := strconv.ParseFloat(match[1], 64); convErr == nil && parsed > 0 {
			fps = parsed
		}
	}
	return width, height, time.Duration(float64(time.Second) / fps), nil
}

func fitFrame(width, height int) (int, int) {
	if width <= frameMaxWidth {
		if height%2 != 0 {
			height--
		}
		return width, height
	}
	height = height * frameMaxWidth / width
	if height%2 != 0 {
		height--
	}
	if height < 2 {
		height = 2
	}
	return frameMaxWidth, height
}
