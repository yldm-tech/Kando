package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// The final proofread (终校, core.Proof): once a film is translated, a pipeline that is a Reviewer reads the whole translation through again, with the original lines and the film's glossary, and suggests changes to lines that write a name, a form of address or a term otherwise than the rest of the film, read unnaturally, or say something the original does not. Make asks for it, a block of the film at a time, when the job's context says so (WithReview): the changes of the kinds the user lets apply by themselves (core.ProofAuto) go into the film's lines at once, and every change is kept beside the video (ProofsPath) with what it was, so the user can accept the others, turn any down or roll the whole pass back (DecideProofs).

// Reviewer is a Pipeline that can proofread a film's translation as a whole, as Tools does through its translator.
type Reviewer interface {
	// Review reads translated, the translation of original line for line into language, and returns the changes it suggests, each keyed by the span of its line, title naming the video for context. The changes have no status; Make gives them one (making.final). A batch it could not read keeps its lines as they are.
	Review(ctx context.Context, original, translated []core.Segment, title, language string) ([]core.Proof, error)
}

type reviewKey struct{}

// WithReview returns ctx asking Make to proofread the translation it makes as a whole, a block at a time, the changes of the kinds auto names applying by themselves.
func WithReview(ctx context.Context, auto core.ProofAuto) context.Context {
	return context.WithValue(ctx, reviewKey{}, core.CleanProofAuto(auto))
}

// reviewOf is the level WithReview put in ctx, and whether it put one.
func reviewOf(ctx context.Context) (core.ProofAuto, bool) {
	auto, ok := ctx.Value(reviewKey{}).(core.ProofAuto)
	return auto, ok
}

// reviewBatch bounds how many lines go into one request of the final proofread. A reply names only the lines it changes, so a batch can hold more of the film than a translation's, which lets the model see how the film writes a name.
const reviewBatch = 100

// reviewPrompt is the instruction a request of the final proofread is sent with; {{title}} becomes the video's title and {{language}} the language it was translated into.
const reviewPrompt = `You proofread the {{language}} subtitles of a video titled "{{title}}", translated line by line by a machine. Read the whole batch, then suggest a change only where a line clearly needs one, of one of three kinds:
- "consistency": the line writes a name, a form of address or a term otherwise than the other lines do. Make it match how most lines, and the glossary, write it.
- "fluency": the line is correct but reads unnaturally in {{language}}. Make it read naturally without changing what it says.
- "meaning": the line says something the original does not. Correct it to what the original says.
Keep each line one subtitle line, as short as it is, in {{language}}. Do not change a line that is already fine, and do not merge or split lines.

The user sends a JSON array of the lines, each {"id": its number, "original": the line as spoken, "translation": its translation}. Reply with only a JSON array with one element for each line you change, and none for the others: {"id": the line's number, "translation": the translation exactly as it was sent, "revised": the new translation, "kind": "consistency", "fluency" or "meaning", "why": the reason in a short phrase in {{language}}}. Reply [] when no line needs a change.`

// reviewChange is a change a reply of the final proofread suggests.
type reviewChange struct {
	ID          int    `json:"id"`
	Translation string `json:"translation"`
	Revised     string `json:"revised"`
	Kind        string `json:"kind"`
	Why         string `json:"why"`
}

// Review proofreads translated, the translation of original into language, in batches of reviewBatch lines, through the same endpoint and model translations use. Lines left untranslated are not sent. A change is kept only when its line is the one its number names, as it was sent, and it is of a kind the final proofread knows (core.Proof.Valid); a locked term's translation is kept as the glossary gives it, so a change that would write it otherwise is dropped. A batch whose reply cannot be used after it was sent again suggests nothing, and the rest of the film is still read; a key, model or address the API rejects, or an account out of credit, fails the pass, which Make tries again, then leaves the block a draft.
func (t ChatTranslator) Review(ctx context.Context, original, translated []core.Segment, title, language string) ([]core.Proof, error) {
	if len(original) != len(translated) {
		return nil, fmt.Errorf("%d translated lines for %d original lines", len(translated), len(original))
	}
	if err := t.offline(); err != nil {
		return nil, err
	}
	endpoint := Endpoint{BaseURL: t.BaseURL, APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return nil, err
		}
	}
	terms := glossaryOf(ctx)
	system := strings.NewReplacer("{{title}}", strings.TrimSpace(title), "{{language}}", strings.TrimSpace(language)).Replace(reviewPrompt)
	if glossary := glossaryPrompt(terms); glossary != "" {
		system += "\n\n" + glossary
	}
	// Only translated lines are read, by their index in the film.
	var at []int
	for i, s := range translated {
		if strings.TrimSpace(s.Text) != "" {
			at = append(at, i)
		}
	}
	renewed := false
	var out []core.Proof
	batches := (len(at) + reviewBatch - 1) / reviewBatch
	for start := 0; start < len(at); start += reviewBatch {
		batch := at[start:min(start+reviewBatch, len(at))]
		progress := core.Fraction(float64(start) / float64(len(at)))
		progress.Reviewing = true
		progress.Batch, progress.Batches, progress.BatchSize = start/reviewBatch+1, batches, reviewBatch
		progress.Lines, progress.Of = start, len(at)
		reportProgress(ctx, progress)
		changes, err := t.reviewBatch(ctx, endpoint, system, original, translated, batch, progress)
		var status *statusError
		if t.Credential != nil && !renewed && errors.As(err, &status) && status.code == http.StatusUnauthorized {
			renewed = true
			if endpoint, err = t.Credential(ctx, true); err != nil {
				return nil, err
			}
			changes, err = t.reviewBatch(ctx, endpoint, system, original, translated, batch, progress)
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if !skippableBatch(err) {
				return nil, err
			}
			logf(withLogRange(ctx, original[batch[0]].Start, original[batch[len(batch)-1]].End, len(batch)), "final proofread: batch %d of %d not proofread: %v", start/reviewBatch+1, batches, err)
			continue
		}
		for _, p := range changes {
			if !breaksLockedTerm(p, original, terms) {
				out = append(out, p)
			}
		}
	}
	done := core.Fraction(1)
	done.Reviewing = true
	reportProgress(ctx, done)
	return out, nil
}

// reviewBatch sends the lines of the film at the indexes batch, again while the reply is not a JSON array of changes, up to maxBatchSends times, and returns the changes it keeps, keyed by their lines' spans.
func (t ChatTranslator) reviewBatch(ctx context.Context, endpoint Endpoint, system string, original, translated []core.Segment, batch []int, progress core.Progress) ([]core.Proof, error) {
	type numbered struct {
		ID          int    `json:"id"`
		Original    string `json:"original"`
		Translation string `json:"translation"`
	}
	lines := make([]numbered, len(batch))
	for k, i := range batch {
		lines[k] = numbered{ID: k + 1, Original: original[i].Text, Translation: strings.TrimSpace(translated[i].Text)}
	}
	input, err := json.Marshal(lines)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(chatRequest{
		Model:    t.Model,
		Messages: []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: string(input)}},
	})
	if err != nil {
		return nil, err
	}
	for send := 1; ; send++ {
		reply, err := t.complete(ctx, endpoint, body, progress)
		if err != nil {
			return nil, err
		}
		var changes []reviewChange
		if len(reply.Choices) == 0 {
			err = errors.New("proofreading API returned no choices")
		} else if err = json.Unmarshal([]byte(stripCodeFence(reply.Choices[0].Message.Content)), &changes); err != nil {
			err = fmt.Errorf("the final proofread is not a JSON array of changes: %w", err)
		}
		if err == nil {
			var out []core.Proof
			seen := map[int]bool{}
			for _, c := range changes {
				k := c.ID - 1
				if k < 0 || k >= len(batch) || seen[k] || strings.TrimSpace(c.Translation) != lines[k].Translation {
					continue
				}
				i := batch[k]
				p := core.Proof{Span: core.Span{Start: original[i].Start, End: original[i].End}, Kind: core.ProofKind(strings.ToLower(strings.TrimSpace(c.Kind))),
					Old: lines[k].Translation, New: strings.TrimSpace(c.Revised), Why: strings.TrimSpace(c.Why)}
				if p.Valid() {
					seen[k] = true
					out = append(out, p)
				}
			}
			return out, nil
		}
		if send == maxBatchSends || ctx.Err() != nil {
			return nil, err
		}
	}
}

// breaksLockedTerm reports whether p would write a locked term of the glossary otherwise than the glossary gives it: its line's original holds the term (core.TermsIn), its translation holds the term's translation, and the change does not.
func breaksLockedTerm(p core.Proof, original []core.Segment, terms []core.Term) bool {
	var line string
	for _, s := range original {
		if s.Start == p.Span.Start && s.End == p.Span.End {
			line = s.Text
			break
		}
	}
	for _, t := range core.TermsIn(terms, []string{line}) {
		if t.Locked && strings.Contains(p.Old, t.Target) && !strings.Contains(p.New, t.Target) {
			return true
		}
	}
	return false
}

// Review proofreads the translation as a whole through the translator (ChatTranslator.Review).
func (t Tools) Review(ctx context.Context, original, translated []core.Segment, title, language string) ([]core.Proof, error) {
	return t.withOffline().Review(ctx, original, translated, title, language)
}

// The changes of the final proofread are kept in a hidden file beside the video, "<video>.proofs.json" with a dot before it (ProofsPath). The job that makes or translates the film again writes it, or removes it when it proofread nothing; accepting or turning a change down rewrites it with the subtitles (DecideProofs). It moves and goes to the Trash with the video (Companions).

// proofsVersion is the version of the proofs file this Kando writes. A file of a later version is read for the fields this one knows and is not written over (ErrProofsNewer).
const proofsVersion = 1

// ErrProofsNewer is SaveProofs refusing to write over a proofs file a newer Kando wrote, whose fields this one would drop.
var ErrProofsNewer = errors.New("the final proofread was written by a newer Kando")

type proofsFile struct {
	Version int          `json:"version"`
	Changes []proofEntry `json:"changes"`
}

type proofEntry struct {
	StartMS int64  `json:"startMs"`
	EndMS   int64  `json:"endMs"`
	Kind    string `json:"kind"`
	Old     string `json:"old"`
	New     string `json:"new"`
	Why     string `json:"why,omitempty"`
	Status  string `json:"status"`
}

// ProofsPath is the proofs file of the film whose video, or whose subtitles Kando reads or writes by the video's name, is at path: ".<video>.proofs.json" beside it, as CorrectionsPath names the corrections.
func ProofsPath(path string) string {
	edits := CorrectionsPath(path)
	return filepath.Join(filepath.Dir(edits), strings.TrimSuffix(filepath.Base(edits), ".edits.json")+".proofs.json")
}

// LoadProofs reads the proofs file at path (ProofsPath). A file that is not there holds none; a change of a kind or status this Kando does not know is left out.
func LoadProofs(path string) ([]core.Proof, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f proofsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	out := make([]core.Proof, 0, len(f.Changes))
	for _, e := range f.Changes {
		p := core.Proof{Span: core.Span{Start: time.Duration(e.StartMS) * time.Millisecond, End: time.Duration(e.EndMS) * time.Millisecond},
			Kind: core.ProofKind(e.Kind), Old: e.Old, New: e.New, Why: e.Why, Status: core.ProofStatus(e.Status)}
		switch p.Status {
		case core.ProofApplied, core.ProofPending, core.ProofAccepted, core.ProofRejected:
		default:
			continue
		}
		if p.Valid() {
			out = append(out, p)
		}
	}
	return out, nil
}

// SaveProofs writes proofs to the proofs file at path (ProofsPath), all of it or nothing (writeFileAtomic); with none it removes the file. A file a newer Kando wrote is left as it is (ErrProofsNewer).
func SaveProofs(path string, proofs []core.Proof) error {
	if data, err := os.ReadFile(path); err == nil {
		var f proofsFile
		if json.Unmarshal(data, &f) == nil && f.Version > proofsVersion {
			return ErrProofsNewer
		}
	}
	if len(proofs) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	f := proofsFile{Version: proofsVersion, Changes: make([]proofEntry, len(proofs))}
	for i, p := range proofs {
		f.Changes[i] = proofEntry{StartMS: p.Span.Start.Milliseconds(), EndMS: p.Span.End.Milliseconds(), Kind: string(p.Kind), Old: p.Old, New: p.New, Why: p.Why, Status: string(p.Status)}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// DecideProofs keeps after, the changes of the film whose video is at video and whose bilingual subtitles are at subtitles as the user decided them, in place of before, as the proofs file holds them, and makes the subtitles match: a change that became live (core.Proof.Live) puts its New in place of its Old as its line's translation, and one that no longer is puts Old back. A film made into its subtitle document (track) keeps each decision there, as the user's, as device's, and its subtitles are written out from it again, so a making still running writes them out too; any other film has its subtitles rewritten in place (core.SwapProofs).
func DecideProofs(video, subtitles, device string, before, after []core.Proof) error {
	if video != "" && len(track.Logs(video)) > 0 {
		return decideTrackProofs(video, subtitles, device, before, after)
	}
	data, err := os.ReadFile(subtitles)
	if err != nil {
		return err
	}
	cues, err := ParseSRT(data)
	if err != nil {
		return err
	}
	path := ProofsPath(subtitles)
	if err := SaveProofs(path, after); err != nil {
		return err
	}
	if !core.SwapProofs(cues, before, after) {
		return nil
	}
	if err := writeFileAtomic(subtitles, []byte(formatSRT(cues))); err != nil {
		_ = SaveProofs(path, before)
		return err
	}
	return nil
}

// decideTrackProofs is DecideProofs of a film made into its subtitle document.
func decideTrackProofs(video, subtitles, device string, before, after []core.Proof) error {
	doc, err := track.Read(video)
	if err != nil {
		return err
	}
	if doc.Newer {
		return ErrProofsNewer
	}
	// The subtitles may have been written since by something other than the document, as an older Kando: what they hold is kept in the document first, so the decision is laid over it, not over what the document held before.
	files, err := trackFiles(doc, subtitles, TranscriptOf(video))
	if err != nil {
		return err
	}
	if len(files) > 0 {
		written, err := track.Append(video, device, files...)
		if err != nil {
			return err
		}
		doc.Events = append(doc.Events, written...)
	}
	var events []track.Event
	lines := doc.View()
	for i := range after {
		if i < len(before) && before[i].Live() == after[i].Live() {
			continue
		}
		text := after[i].New
		if !after[i].Live() {
			text = after[i].Old
		}
		for _, l := range lines {
			if l.Start == after[i].Span.Start && l.End == after[i].Span.End {
				events = append(events, track.Event{Kind: track.KindRev, Line: l.ID, Side: track.Translation, Layer: track.LayerUser, Text: text, Source: "review"})
				break
			}
		}
	}
	path := ProofsPath(subtitles)
	if err := SaveProofs(path, after); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	written, err := track.Append(video, device, events...)
	if err != nil {
		_ = SaveProofs(path, before)
		return err
	}
	doc.Events = append(doc.Events, written...)
	projected, err := projectTrack(doc, doc.View(), TranscriptOf(video), subtitles)
	if err != nil {
		return err
	}
	_, err = track.Append(video, device, projected...)
	return err
}
