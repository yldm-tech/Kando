package main

import (
	"slices"

	"kando/apps/kando/account"
)

// The 账号与上传 page's 存储位置 card's reads and writes (account.Locations). The page reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself; what they say is in the interface's language. The state and what the actions do are in state_locations.go.

func (a *app) Locations() []account.Location { return slices.Clone(a.locations.list) }

func (a *app) LocationStatus(id string) account.Status {
	t := a.locations.tests[id]
	s := account.Status{Busy: t.busy, OK: t.ok}
	if t.err != nil {
		s.Err = t.err(a.messages())
	}
	return s
}

func (a *app) TestLocation(id string) { a.testLocation(id) }

func (a *app) AddLocation() { a.openLocationDialog("", account.DraftOf(account.Location{})) }

func (a *app) EditLocation(id string) {
	if i := a.locationIndex(id); i >= 0 {
		a.openLocationDialog(id, account.DraftOf(a.locations.list[i]))
	}
}

func (a *app) AskRemoveLocation(id string) {
	if a.locationIndex(id) >= 0 {
		a.askLocation(locationAlert{kind: account.AlertRemove, target: id})
	}
}

func (a *app) LocationEditing() bool { return a.locations.editing }

func (a *app) LocationIsNew() bool { return a.locations.editID == "" }

func (a *app) LocationDraft() *account.Draft { return &a.locations.draft }

func (a *app) LocationProblem() string {
	if a.locations.problem == nil {
		return ""
	}
	return a.locations.problem(a.messages())
}

func (a *app) SaveLocation() { a.saveLocationDraft() }

func (a *app) CloseLocation() { a.closeLocationDialog() }

func (a *app) LocationAlert() account.Alert { return a.locationAlertView() }

func (a *app) AnswerLocationAlert(yes bool) { a.answerLocation(yes) }
