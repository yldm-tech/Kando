package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"kando/packages/core"
)

// translationCheckpointPath is where the translation of the video at videoPath keeps the batches it has done until the job exports (Run): a hidden file beside the video, such as ".video.translation.json". It is "", which keeps nothing, for a job that has no video path.
func translationCheckpointPath(videoPath string) string {
	if videoPath == "" {
		return ""
	}
	return hiddenBeside(videoPath, ".translation.json")
}

type checkpointKey struct{}

// withCheckpoint returns ctx carrying path, the file a translator that honors it (ChatTranslator) keeps the batches it has translated in, so a translation that stops partway, cancelled, quit or failed, resumes on the next run from the first batch it had not done rather than paying for the others again.
func withCheckpoint(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, checkpointKey{}, path)
}

// savedTranslation is what a checkpoint file holds: the target language and a hash of the lines sent, which say which translation it is part of, and each line's translation where Done says its batch was translated.
type savedTranslation struct {
	Language string   `json:"language"`
	Lines    string   `json:"lines"`
	Text     []string `json:"text"`
	Done     []bool   `json:"done"`
}

// checkpoint is the translation of segments into a language kept so far, read from and written to path. A nil checkpoint keeps nothing.
type checkpoint struct {
	path  string
	saved savedTranslation
}

// openCheckpoint is the checkpoint of translating segments into language that ctx names the file of (withCheckpoint), with the batches an earlier run of the same translation kept there. A file kept for other lines or another language is not this translation's, and the first batch done replaces it. It is nil when ctx names no file.
func openCheckpoint(ctx context.Context, segments []core.Segment, language string) *checkpoint {
	path, _ := ctx.Value(checkpointKey{}).(string)
	if path == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(language + "\x00" + formatSRT(segments)))
	c := &checkpoint{path: path, saved: savedTranslation{
		Language: language,
		Lines:    hex.EncodeToString(sum[:]),
		Text:     make([]string, len(segments)),
		Done:     make([]bool, len(segments)),
	}}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var saved savedTranslation
	if json.Unmarshal(data, &saved) != nil || saved.Language != c.saved.Language || saved.Lines != c.saved.Lines || len(saved.Text) != len(segments) || len(saved.Done) != len(segments) {
		return c
	}
	c.saved = saved
	return c
}

// batch is the translation of the n lines from start that an earlier run kept, and whether it kept all of them.
func (c *checkpoint) batch(start, n int) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	for _, done := range c.saved.Done[start : start+n] {
		if !done {
			return nil, false
		}
	}
	return c.saved.Text[start : start+n], true
}

// keep records lines as the translation of the lines from start and writes the checkpoint, replacing the file at once (writeFileAtomic). A checkpoint that cannot be written only means a retry translates those lines again.
func (c *checkpoint) keep(start int, lines []string) {
	if c == nil {
		return
	}
	copy(c.saved.Text[start:], lines)
	for i := range lines {
		c.saved.Done[start+i] = true
	}
	data, err := json.Marshal(c.saved)
	if err != nil {
		return
	}
	_ = writeFileAtomic(c.path, data)
}
