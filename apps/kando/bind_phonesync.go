package main

import (
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/settings"
)

// Settings › 手机同步's reads and actions (settings.App), from state_phonesync.go. The page reads them in its bindings and calls the actions from its handlers, which tell the revision, so none of them calls Changed itself.

func (a *app) PhoneSync() settings.PhoneSync { return a.phoneSyncStatus() }

func (a *app) SetPhoneSync(on bool) { a.phoneSyncSet(on) }

func (a *app) NewPhoneCode() { a.phoneSyncNewCode() }

func (a *app) RevokePhone(id string) { a.phoneSyncRevoke(id) }

func (a *app) SetPhoneReceive(on bool) { a.phoneSyncSetReceive(on) }

func (a *app) SetPhoneAskFirst(ask bool) { a.phoneSyncSetAsk(ask) }

func (a *app) SetPhoneReadOnly(id string, on bool) { a.phoneSyncSetReadOnly(id, on) }

func (a *app) SetPhoneRelay(on bool) { a.phoneRelaySet(on) }

func (a *app) SetPhoneRelayHosted(on bool) { a.phoneHostSet(on) }

func (a *app) SubscribePhoneRelay() { a.phoneHostSubscribe() }

func (a *app) SetPhoneRelaySubtitlesOnly(on bool) { a.phoneFilmsSetSubtitlesOnly(on) }

func (a *app) SetPhoneRelayClean(days int) { a.phoneFilmsSetClean(days) }

func (a *app) CleanPhoneVideos(watched []settings.PhoneWatched) { a.phoneFilmsClean(watched) }

func (a *app) MovePhoneStorage(kind string) { a.phoneFilmsMove(kind) }

func (a *app) ResolvePhoneConflict(id string, keep settings.PhoneKeep) { a.phoneSyncResolve(id, keep) }

func (a *app) PhoneNow() time.Time { return a.phoneStateNow() }

// The library page's part of Phone Sync (films.App), from state_phonesync_linked.go: where a phone last left a film, offered to continue here, and the phone each film was left on.

func (a *app) Handoff() films.Handoff { return a.phoneHandoff() }

func (a *app) CloseHandoff() { a.phoneHandoffClose() }

func (a *app) LeftOn(path string) string { return a.phoneLeftOn(path) }
