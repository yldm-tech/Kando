package main

import (
	"errors"
	"strings"

	"kando/apps/kando/clitool"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
)

// cliToolUI is the state of Settings › 通用 › 命令行与 MCP: where the kando command stands (clitool/), which the app looks up off the UI thread when the 通用 page opens and on 重新检测, never while the page is drawn, and its 安装 under way.
type cliToolUI struct {
	// env is the computer the card looks at; tests replace it.
	env func() clitool.Env
	// status is what the last look found; known reports whether one has finished, gen counts the looks so only the newest one lands.
	status clitool.Status
	known  bool
	gen    int
	// installing is an install under way; err is why the last one failed, until the next one starts.
	installing bool
	err        string
	// copied is the text the card last put on the clipboard, so its button reads 已拷贝 while the text stays the same.
	copied string
}

func newCLIToolUI() cliToolUI { return cliToolUI{env: clitool.EnvHere} }

// cliToolCheck looks for the kando command again off the UI thread, keeping what the last look found on the page until it lands.
func (a *app) cliToolCheck() {
	a.cliTool.gen++
	gen, env, update := a.cliTool.gen, a.cliTool.env, a.update
	go func() {
		status := clitool.Look(env())
		update(func() {
			if gen != a.cliTool.gen {
				return
			}
			a.cliTool.status, a.cliTool.known = status, true
		})
	}()
}

// cliToolStartInstall links kando to this app's executable off the UI thread (clitool.Install), then looks again, so the card says where the command went and whether a terminal finds it.
func (a *app) cliToolStartInstall() {
	if a.cliTool.installing {
		return
	}
	a.cliTool.installing, a.cliTool.err = true, ""
	a.cliTool.gen++
	gen, env, update := a.cliTool.gen, a.cliTool.env, a.update
	go func() {
		here := env()
		exe := clitool.Look(here).Exe
		_, err := clitool.Install(here, exe)
		status := clitool.Look(here)
		update(func() {
			a.cliTool.installing = false
			if err != nil {
				a.cliTool.err = cliToolErrorText(err)
			}
			if gen == a.cliTool.gen {
				a.cliTool.status, a.cliTool.known = status, true
			}
		})
	}()
}

// cliToolErrorText is why an install failed in the app's language, for the ones clitool.Install refuses itself; any other error, as the system's permission denied, is shown as it is. It reads the locale, so it runs on the UI thread.
func cliToolErrorText(err error) string {
	var blocked *clitool.BlockedError
	switch {
	case errors.Is(err, clitool.ErrTemporary):
		return locales.SettingsGeneralCLIMoveFirst()
	case errors.As(err, &blocked):
		paths := make([]string, len(blocked.Paths))
		for i, path := range blocked.Paths {
			paths[i] = films.DisplayPath(path)
		}
		return locales.SettingsGeneralCLIBlocked(strings.Join(paths, ", "))
	}
	return err.Error()
}
