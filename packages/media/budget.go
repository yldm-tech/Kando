package media

import "errors"

// ErrMonthlyCap is what a request to the AI service returns, before it is sent, while the tokens counted this month and the request's would go over the monthly limit the user set (ChatTranslator.Budget). Every request is refused alike until the limit is raised or turned off, or the month ends, so a translation stops at it rather than leave batch after batch untranslated. A failed job keeps its text, so the app tells it from other failures by its words (ClassifyTranslate's TranslateCap).
var ErrMonthlyCap = errors.New("the monthly AI token limit is reached")
