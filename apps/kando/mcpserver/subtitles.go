package mcpserver

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kando/apps/kando/remote"
)

// The languages kando_get_subtitles can return.
const (
	whichBoth        = "both"
	whichOriginal    = "original"
	whichTranslation = "translation"
)

// The formats kando_get_subtitles can return its text in.
const (
	formatText = "text"
	formatSRT  = "srt"
)

// subtitlesInput is kando_get_subtitles's input. Its schema is subtitlesInputSchema, written out rather than inferred so which and format list the values they take and default to them.
type subtitlesInput struct {
	ID      string `json:"id"`
	Which   string `json:"which,omitempty"`
	Format  string `json:"format,omitempty"`
	MaxCues int    `json:"max_cues,omitempty"`
}

var subtitlesInputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"id": map[string]any{
			"type":        "string",
			"description": "The film's id, as kando_list_films reports it.",
		},
		"which": map[string]any{
			"type":        "string",
			"enum":        []string{whichBoth, whichOriginal, whichTranslation},
			"default":     whichBoth,
			"description": "Which subtitles to return: both the translation and the original speech, the original alone, or the translation alone (lines the translation left out are skipped).",
		},
		"format": map[string]any{
			"type":        "string",
			"enum":        []string{formatText, formatSRT},
			"default":     formatText,
			"description": "text is one line per subtitle, \"[hh:mm:ss] translation | original\", the most compact to read; srt is a SubRip file with start and end times, to save or hand to a player.",
		},
		"max_cues": map[string]any{
			"type":        "integer",
			"minimum":     0,
			"description": "Return at most this many subtitle lines, from the start of the video. Leave it out, or 0, for all of them.",
		},
	},
	"required":             []string{"id"},
	"additionalProperties": false,
}

// subtitlesOutput is kando_get_subtitles's structured result.
type subtitlesOutput struct {
	ID    string       `json:"id" jsonschema:"The film's id."`
	Which string       `json:"which" jsonschema:"Which subtitles were returned: both, original or translation."`
	Total int          `json:"total" jsonschema:"How many subtitle lines the film has in the returned language or languages."`
	Shown int          `json:"shown" jsonschema:"How many of them are returned, fewer than total when max_cues cut them short."`
	Cut   bool         `json:"truncated" jsonschema:"Whether max_cues left lines out."`
	Cues  []remote.Cue `json:"cues" jsonschema:"The returned lines: start_ms and end_ms in milliseconds from the start of the video, text the original speech, translation its translation. With which original, translation is left out; with which translation, text is empty."`
}

// subtitlesResult is kando_get_subtitles's answer for the film's cues: the text in the asked format as its content, a note after it when max_cues cut it short, and the returned cues as its structured output.
func subtitlesResult(in subtitlesInput, cues []remote.Cue) (*mcp.CallToolResult, subtitlesOutput, error) {
	which := in.Which
	if which == "" {
		which = whichBoth
	}
	format := in.Format
	if format == "" {
		format = formatText
	}
	if which != whichBoth && which != whichOriginal && which != whichTranslation {
		return nil, subtitlesOutput{}, fmt.Errorf("which is %q; it takes both, original or translation", which)
	}
	if format != formatText && format != formatSRT {
		return nil, subtitlesOutput{}, fmt.Errorf("format is %q; it takes text or srt", format)
	}

	picked := make([]remote.Cue, 0, len(cues))
	for _, cue := range cues {
		switch which {
		case whichOriginal:
			cue.Translation = ""
		case whichTranslation:
			if cue.Translation == "" {
				continue
			}
			cue.Text = ""
		}
		picked = append(picked, cue)
	}
	shown := picked
	if in.MaxCues > 0 && len(shown) > in.MaxCues {
		shown = shown[:in.MaxCues]
	}

	var text string
	switch {
	case len(shown) == 0:
		text = "This film has no subtitle lines."
		if which == whichTranslation && len(cues) > 0 {
			text = "This film's subtitles have no translated lines."
		}
	case format == formatSRT:
		text = subtitlesSRT(shown)
	default:
		text = subtitlesText(shown)
	}
	content := []mcp.Content{&mcp.TextContent{Text: text}}
	if len(shown) < len(picked) {
		last := shown[len(shown)-1]
		content = append(content, &mcp.TextContent{Text: fmt.Sprintf("Truncated: these are the first %d of %d subtitle lines, up to %s. Call kando_get_subtitles again with a larger max_cues, or without it, for the rest.", len(shown), len(picked), clock(last.EndMS))})
	}
	out := subtitlesOutput{ID: in.ID, Which: which, Total: len(picked), Shown: len(shown), Cut: len(shown) < len(picked), Cues: shown}
	return &mcp.CallToolResult{Content: content}, out, nil
}

// subtitlesText is one line per cue, "[hh:mm:ss] translation | original", with only the part a cue has when it has one.
func subtitlesText(cues []remote.Cue) string {
	var b strings.Builder
	for _, cue := range cues {
		b.WriteString("[" + clock(cue.StartMS) + "] ")
		switch {
		case cue.Translation != "" && cue.Text != "":
			b.WriteString(oneLine(cue.Translation) + " | " + oneLine(cue.Text))
		case cue.Translation != "":
			b.WriteString(oneLine(cue.Translation))
		default:
			b.WriteString(oneLine(cue.Text))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// subtitlesSRT is the cues as a SubRip file, numbered from 1, the translation above the original as Kando's bilingual .srt has them.
func subtitlesSRT(cues []remote.Cue) string {
	var b strings.Builder
	for i, cue := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n", i+1, srtTime(cue.StartMS), srtTime(cue.EndMS))
		for _, part := range []string{cue.Translation, cue.Text} {
			if part != "" {
				b.WriteString(part + "\n")
			}
		}
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// oneLine joins the lines of a cue's text with spaces, so each cue stays on one line of the text format.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// clock is ms as hh:mm:ss.
func clock(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}

// srtTime is ms as SubRip's hh:mm:ss,mmm.
func srtTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%s,%03d", clock(ms), ms%1000)
}
