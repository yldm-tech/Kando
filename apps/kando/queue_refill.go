package main

import (
	"kando/apps/kando/films"
	"kando/packages/core"
)

// Refilling: a film left untranslated from where the AI service was gone for a while, a network down, a service over its capacity or requests that timed out (core.Job.TranslationErr), has its lines left translated (core.RedoMissing) once the service is seen answering again: its 测试 passed, or a film was just translated. The user asked for the film translated already, and only the lines left are sent, so it costs what they would have; a film the service refused, as one whose key or credit ran out, waits for the user, whom the library offers to translate it once the service's settings change (libraryOfferTranslate).

// queueRefill queues, behind the jobs waiting, the done films of the open library left untranslated from where the service was gone for a while (films.Refillable), once each since launch, to translate their lines left quietly: neither the page nor the selected card moves, and one that fails again says nothing, its film as it was.
func (a *app) queueRefill() {
	if a.untranslatable() {
		return
	}
	for _, job := range a.library.Jobs {
		if !films.Refillable(films.FromJob(job, "", "")) || job.ID == a.queue.active || a.queue.refilled[job.ID] {
			continue
		}
		ready, err := a.queueRetranslatable(job, core.RedoMissing, "")
		if err != nil {
			continue
		}
		if a.queue.refilled == nil {
			a.queue.refilled, a.queue.refills = map[string]bool{}, map[string]bool{}
		}
		a.queue.refilled[job.ID], a.queue.refills[job.ID] = true, true
		delete(a.lib.quotes, job.SubtitlePath)
		a.queueAdd(ready)
	}
}
