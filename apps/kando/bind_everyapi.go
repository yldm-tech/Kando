package main

import "kando/apps/kando/settings"

// The EveryAPI sign-in as Settings › AI 服务 and the first-run guide read and use it (settings.App, onboarding.App).

func (a *app) EveryAPIAccount() settings.EveryAPIAccount { return a.everyapiAccount() }
func (a *app) EveryAPISignIn()                           { a.everyapiSignIn() }
func (a *app) EveryAPIUseCode()                          { a.everyapiUseCode() }
func (a *app) EveryAPICancelSignIn()                     { a.everyapiCancelSignIn() }
func (a *app) EveryAPISignOut()                          { a.everyapiSignOut() }
func (a *app) EveryAPITopUp()                            { a.everyapiTopUp() }
