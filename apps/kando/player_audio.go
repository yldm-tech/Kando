package main

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/audio"
)

// The sound of a film plays through mygo/audio, which sends it to the system's audio output on every platform Kando runs on: an Audio Queue on macOS, PulseAudio (or PipeWire serving it) or else ALSA on Linux, WASAPI on Windows. ffmpeg, the same one that decodes the picture, decodes the film's sound track into 48 kHz stereo float samples on a pipe, and the player reads them about a second ahead of what is heard.
//
// ffmpeg ends with the app however the app ends, a crash or a force quit included: the app holds the only read end of the pipe, the system closes it when the app's process ends, and ffmpeg's next write, or the one it is blocked in while the sound is paused, fails with a broken pipe, which ends it. So no process of its own watches over it, as one had to over the ffplay that played the sound before.

const (
	// playerAudioRate and playerAudioChannels are the format ffmpeg writes the sound in, the output's own, so that nothing is resampled on the way.
	playerAudioRate     = 48000
	playerAudioChannels = 2
	// playerAudioFrameBytes is one frame on the pipe: a little-endian float32 for each channel.
	playerAudioFrameBytes = playerAudioChannels * 4
)

// playerAudioOutputLogged keeps the reason the system's audio output could not open to one line in the log, however many films play unheard.
var playerAudioOutputLogged sync.Once

// pcmAudio is the sound of the film open on the film page, from where it was opened on. Start and Play play it, Pause holds it where it is, setVolume changes how loud it plays while it plays, and Close ends it for good. A nil pcmAudio is a film without sound and does nothing.
type pcmAudio struct {
	player  *audio.Player
	decoder *playerAudioDecoder
	// ended is set once the sound track played to its end. A Player that ended plays again from its start, which is where the sound was opened, so Play leaves it ended instead.
	ended atomic.Bool
	once  sync.Once
}

// openPCM starts decoding the sound of path from at, to play at volume from 0 (silent) to 1 (as recorded). It holds the sound until Start; ffmpeg decodes ahead meanwhile, so the sound starts with the picture.
func openPCM(ffmpeg, path string, at time.Duration, volume float64) (*pcmAudio, error) {
	if ffmpeg == "" || path == "" {
		return nil, errors.New("no ffmpeg or no video to play the sound of")
	}
	dec := &playerAudioDecoder{ffmpeg: ffmpeg, path: path, from: max(at, 0)}
	if err := dec.start(0); err != nil {
		return nil, err
	}
	a := &pcmAudio{player: audio.NewPlayer(dec), decoder: dec}
	a.player.SetVolume(playerAudioGain(volume))
	a.player.OnEnd(func() { a.ended.Store(true) })
	return a, nil
}

// playerAudioGain is volume held within 0 and 1.
func playerAudioGain(volume float64) float64 { return min(max(volume, 0), 1) }

// Start plays the sound from where it is. It does nothing to a sound that plays already, such as the one a film coming back from the background hands over.
func (a *pcmAudio) Start() { a.Play() }

// Play plays the sound on from where Pause held it.
func (a *pcmAudio) Play() {
	if a == nil || a.ended.Load() {
		return
	}
	a.player.Play()
}

// Pause holds the sound where it is. ffmpeg waits on the full pipe meanwhile.
func (a *pcmAudio) Pause() {
	if a == nil {
		return
	}
	a.player.Pause()
}

// setVolume plays the sound at volume from 0 to 1 from now on, as it plays.
func (a *pcmAudio) setVolume(volume float64) {
	if a == nil {
		return
	}
	a.player.SetVolume(playerAudioGain(volume))
}

// Close stops the sound and ends its ffmpeg. It is safe to call more than once.
func (a *pcmAudio) Close() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		a.player.Close()
		// The player closes its decoder only once a read in progress returns, which waits on ffmpeg; ending ffmpeg here returns that read at once.
		a.decoder.Close()
		if err := audio.Err(); err != nil {
			playerAudioOutputLogged.Do(func() { log.Printf("player: films play without sound: %v", err) })
		}
	})
}

// playerAudioDecoder is the sound of a film as an audio.Decoder: what ffmpeg decodes of path from from on. Its frames count from from, the start of the player's own sound. The player reads it on a goroutine of its own, while Close may come from the UI thread.
type playerAudioDecoder struct {
	ffmpeg, path string
	from         time.Duration
	// raw holds the bytes read from the pipe, of which the first partial are a frame the last read cut short. Only Read and SetPosition, on the player's goroutine, use them.
	raw     []byte
	partial int

	mu     sync.Mutex
	proc   *playerAudioProcess
	closed bool
}

// playerAudioProcess is one ffmpeg decoding the sound, and the read end of its pipe.
type playerAudioProcess struct {
	cmd  *exec.Cmd
	out  *os.File
	once sync.Once
}

func (d *playerAudioDecoder) SampleRate() int { return playerAudioRate }

func (d *playerAudioDecoder) Channels() int { return playerAudioChannels }

// Len is not known: the sound ends where ffmpeg stops writing.
func (d *playerAudioDecoder) Len() int64 { return -1 }

// playerAudioArgs are ffmpeg's arguments to write the film's first sound track from at on as 48 kHz stereo f32le. The seek comes before the input, as the picture's does (media.OpenFramesAt), so the two start at the same point.
func playerAudioArgs(path string, at time.Duration) []string {
	args := []string{"-nostdin", "-loglevel", "error"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64))
	}
	return append(args,
		"-i", path,
		"-map", "0:a:0", "-vn",
		"-f", "f32le", "-ac", strconv.Itoa(playerAudioChannels), "-ar", strconv.Itoa(playerAudioRate),
		"pipe:1",
	)
}

// start starts ffmpeg at frame, counted from from, in place of any ffmpeg before it.
func (d *playerAudioDecoder) start(frame int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return os.ErrClosed
	}
	if d.proc != nil {
		d.proc.stop()
		d.proc = nil
	}
	d.partial = 0
	at := d.from + time.Duration(frame)*time.Second/playerAudioRate
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.Command(d.ffmpeg, playerAudioArgs(d.path, at)...)
	cmd.Stdout = w
	err = cmd.Start()
	// ffmpeg has its own copy of the write end; with this one closed, ffmpeg ending ends the pipe, and the app ending breaks it.
	w.Close()
	if err != nil {
		r.Close()
		return err
	}
	d.proc = &playerAudioProcess{cmd: cmd, out: r}
	return nil
}

// Read reads whole frames of the sound into buf. It returns io.EOF once ffmpeg stopped writing, at the end of the track, on a file without sound or after Close.
func (d *playerAudioDecoder) Read(buf []float32) (int, error) {
	d.mu.Lock()
	proc := d.proc
	d.mu.Unlock()
	want := len(buf) / playerAudioChannels * playerAudioFrameBytes
	if proc == nil {
		return 0, io.EOF
	}
	if want == 0 {
		return 0, nil
	}
	if len(d.raw) < want {
		raw := make([]byte, want)
		copy(raw, d.raw[:d.partial])
		d.raw = raw
	}
	raw := d.raw[:want]
	// At least one whole frame: the player takes a read of none for the end.
	n, err := io.ReadAtLeast(proc.out, raw[d.partial:], playerAudioFrameBytes-d.partial)
	got := d.partial + n
	whole := got - got%playerAudioFrameBytes
	for i := 0; i < whole; i += 4 {
		buf[i/4] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i:]))
	}
	d.partial = copy(raw, raw[whole:got])
	if err != nil {
		// The end of the track, or a pipe closed under the read: either way there is no more sound from here.
		proc.wait()
		return whole / 4, io.EOF
	}
	return whole / 4, nil
}

// SetPosition ends ffmpeg and starts it again at frame, counted from from.
func (d *playerAudioDecoder) SetPosition(frame int64) error {
	return d.start(max(frame, 0))
}

// Close ends ffmpeg. It is safe to call more than once, and while Read waits on the pipe.
func (d *playerAudioDecoder) Close() error {
	d.mu.Lock()
	proc := d.proc
	d.proc = nil
	d.closed = true
	d.mu.Unlock()
	if proc != nil {
		proc.stop()
	}
	return nil
}

// stop ends ffmpeg and closes its pipe, returning any read waiting on it.
func (p *playerAudioProcess) stop() {
	_ = p.cmd.Process.Kill()
	p.out.Close()
	p.wait()
}

// wait reaps ffmpeg once it ended, whoever sees it end first.
func (p *playerAudioProcess) wait() {
	p.once.Do(func() { _ = p.cmd.Wait() })
}
