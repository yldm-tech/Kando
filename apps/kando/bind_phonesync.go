package main

import "kando/apps/kando/settings"

// Settings › 手机同步's reads and actions (settings.App), from state_phonesync.go. The page reads them in its bindings and calls the actions from its handlers, which tell the revision, so none of them calls Changed itself.

func (a *app) PhoneSync() settings.PhoneSync { return a.phoneSyncStatus() }

func (a *app) SetPhoneSync(on bool) { a.phoneSyncSet(on) }

func (a *app) NewPhoneCode() { a.phoneSyncNewCode() }

func (a *app) RevokePhone(id string) { a.phoneSyncRevoke(id) }

func (a *app) SetPhoneReceive(on bool) { a.phoneSyncSetReceive(on) }

func (a *app) SetPhoneAskFirst(ask bool) { a.phoneSyncSetAsk(ask) }
