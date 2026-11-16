package mcpserver

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kando/apps/kando/remote"
)

// The languages kando_get_subtitles can return (remote.PickCues).
const (
	whichBoth        = remote.WhichBoth
	whichOriginal    = remote.WhichOriginal
	whichTranslation = remote.WhichTranslation
)

// The formats kando_get_subtitles can return its text in.
const (
	formatText = "text"
	formatSRT  = "srt"
)

// subtitlesInput is kando_get_subtitles's input. Its schema is subtitlesInputSchema, written out rather than inferred so which, format and until list the values they take and default to them.
type subtitlesInput struct {
	ID      string `json:"id"`
	Which   string `json:"which,omitempty"`
	Format  string `json:"format,omitempty"`
	MaxCues int    `json:"max_cues,omitempty"`
	FromMS  int64  `json:"from_ms,omitempty"`
	ToMS    int64  `json:"to_ms,omitempty"`
	Until   string `json:"until,omitempty"`
}

// untilSchema is the until property of the tools that read subtitles.
var untilSchema = map[string]any{
	"type":        "string",
	"enum":        []string{remote.UntilWatched},
	"description": "watched: only the lines up to the furthest point the user has played the film to (its watched_ms), none of a film not played yet, so nothing after it is given away. Use it to quiz, recap or answer questions about what the user has seen. Leave it out for every line.",
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
			"description": "Return at most this many subtitle lines, from from_ms on. Leave it out, or 0, for all of them. When it cuts the lines short, the result gives next_from_ms to read the next ones.",
		},
		"from_ms": map[string]any{
			"type":        "integer",
			"minimum":     0,
			"description": "Only the lines that start at this time or later, in milliseconds from the start of the video. Leave it out to start at the beginning.",
		},
		"to_ms": map[string]any{
			"type":        "integer",
			"minimum":     0,
			"description": "Only the lines that start before this time, in milliseconds. Leave it out, or 0, to read to the end.",
		},
		"until": untilSchema,
	},
	"required":             []string{"id"},
	"additionalProperties": false,
}

// subtitlesOutput is kando_get_subtitles's structured result.
type subtitlesOutput struct {
	ID    string       `json:"id" jsonschema:"The film's id."`
	Which string       `json:"which" jsonschema:"Which subtitles were returned: both, original or translation."`
	Total int          `json:"total" jsonschema:"How many subtitle lines the film has in the returned language or languages, within from_ms, to_ms and until."`
	Shown int          `json:"shown" jsonschema:"How many of them are returned, fewer than total when max_cues cut them short."`
	Cut   bool         `json:"truncated" jsonschema:"Whether max_cues left lines out."`
	Next  int64        `json:"next_from_ms,omitempty" jsonschema:"When max_cues left lines out, the from_ms that reads on from the first of them."`
	Cues  []remote.Cue `json:"cues" jsonschema:"The returned lines: start_ms and end_ms in milliseconds from the start of the video, text the original speech, translation its translation. With which original, translation is left out; with which translation, text is empty."`
}

// subtitlesResult is kando_get_subtitles's answer for the film's cues, which the app already kept to from_ms, to_ms and until: the text in the asked format as its content, a note after it when max_cues cut it short, and the returned cues as its structured output.
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

	picked := remote.PickCues(cues, which)
	shown := picked
	if in.MaxCues > 0 && len(shown) > in.MaxCues {
		shown = shown[:in.MaxCues]
	}

	var text string
	switch {
	case len(shown) == 0 && in.Until == remote.UntilWatched && len(cues) == 0:
		text = "No subtitle lines have been watched yet in this part of the film."
	case len(shown) == 0 && (in.FromMS > 0 || in.ToMS > 0):
		text = "This film has no subtitle lines in this part."
	case len(shown) == 0:
		text = "This film has no subtitle lines."
		if which == whichTranslation && len(cues) > 0 {
			text = "This film's subtitles have no translated lines."
		}
	case format == formatSRT:
		text = strings.TrimSuffix(remote.CuesSRT(shown), "\n")
	default:
		text = strings.TrimSuffix(remote.CuesText(shown), "\n")
	}
	content := []mcp.Content{&mcp.TextContent{Text: text}}
	out := subtitlesOutput{ID: in.ID, Which: which, Total: len(picked), Shown: len(shown), Cut: len(shown) < len(picked), Cues: shown}
	if out.Cut {
		last, next := shown[len(shown)-1], picked[len(shown)]
		out.Next = next.StartMS
		content = append(content, &mcp.TextContent{Text: fmt.Sprintf("Truncated: these are the first %d of %d subtitle lines, up to %s. Call kando_get_subtitles again with from_ms %d for the next ones.", len(shown), len(picked), remote.Clock(last.EndMS), next.StartMS)})
	}
	return &mcp.CallToolResult{Content: content}, out, nil
}
