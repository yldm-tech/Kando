package main

import "kando/apps/kando/settings"

// The 隐私 page's part of settings.App (state_privacy.go).

func (a *app) Offline() bool                   { return a.privacy.offline }
func (a *app) SetOffline(on bool)              { a.privacySetOffline(on) }
func (a *app) AILocal() bool                   { return a.privacyLocalAI() }
func (a *app) ClearCache()                     { a.privacyClearCache() }
func (a *app) CacheClear() settings.CacheClear { return a.privacy.clear }
