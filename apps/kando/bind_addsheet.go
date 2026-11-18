package main

// Settings › 通用 › 浏览器扩展's 链接到达后立即开始 (settings.App), over the state in state_addsheet.go. The page saves after each edit.

func (a *app) LinkStartAtOnce() bool { return a.addsheet.startAtOnce }

func (a *app) SetLinkStartAtOnce(on bool) { a.addsheet.startAtOnce = on }
