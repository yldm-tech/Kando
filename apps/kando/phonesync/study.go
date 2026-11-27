package phonesync

import (
	"errors"
	"strings"
	"time"

	"kando/apps/kando/lines"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// Line is a line a phone sends, as Explain Line and the recap read lines.
func Line(c psync.Cue) lines.Line {
	return lines.New(time.Duration(c.StartMS)*time.Millisecond, strings.TrimSpace(c.Translation), strings.TrimSpace(c.Original))
}

// ExplainLines are the line req explains and the lines before it that go with it, at most before of them; the lines after it are left out, so what the AI service is given gives nothing away of what comes after. req has been checked (psync.LineExplain.Check).
func ExplainLines(req psync.LineExplain, before int) ([]lines.Line, lines.Line) {
	var out []lines.Line
	for _, c := range req.Lines[max(req.Index-before, 0):req.Index] {
		out = append(out, Line(c))
	}
	return out, Line(req.Lines[req.Index])
}

// The phrases StudyErr tells a phone with, in English for its logs.
const (
	// StudyNoAI is that no AI service set in the settings can answer.
	StudyNoAI = "no AI service set in Settings › AI Service can answer"
	// studyRefused is the AI service set refusing the desktop's account: out of credit or over its quota, or its key rejected.
	studyRefused = "the AI service set in Settings › AI Service cannot answer for now"
	// studyCapped is the monthly limit of tokens set on the desktop refusing the request before it was sent: neutral, as the desktop's words for it name its settings.
	studyCapped = "the AI service cannot answer for now"
	// studyFailed is any other failure of the AI service, or of a dictionary or plugin asked.
	studyFailed = "the AI service did not answer"
)

// StudyErr is err, why a lookup, an explanation, a follow-up question or a recap failed, as a phone is told it: a protocol code with a fixed phrase of this package's, never err's own text, which holds what the AI service answered, as its prices or a link to buy credit, that a phone must never show. A *psync.CallError the app gave is told as it is; the AI service refusing the account, as one out of credit (media.TranslateQuota) or whose key it rejects (media.TranslateAuth), is psync.CodeNoAI, which a phone tells as an AI service that cannot answer, and so is the monthly limit of tokens refusing the request (media.TranslateCap), with a phrase that does not name the desktop's settings; anything else is psync.CodeFailed, with the kind of failure media.ClassifyTranslate reads, as a timeout, for the phone's logs.
func StudyErr(err error) error {
	var e *psync.CallError
	if err == nil || errors.As(err, &e) {
		return err
	}
	kind := media.ClassifyTranslate(err.Error())
	switch kind {
	case media.TranslateQuota, media.TranslateAuth:
		return psync.Errorf(psync.CodeNoAI, "%s", studyRefused)
	case media.TranslateCap:
		return psync.Errorf(psync.CodeNoAI, "%s", studyCapped)
	case "":
		return psync.Errorf(psync.CodeFailed, "%s", studyFailed)
	}
	return psync.Errorf(psync.CodeFailed, "%s (%s)", studyFailed, kind)
}
