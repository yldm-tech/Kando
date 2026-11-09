package main

import (
	"errors"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
)

// cliToolUI is the state of Settings › 通用 › 命令行与 MCP: where the kando command stands (cli_install.go), which the app looks up off the UI thread when the 通用 page opens and on 重新检测, never while the page is drawn, and its 安装 under way.
type cliToolUI struct {
	// env is the computer the card looks at; tests replace it.
	env func() cliToolEnv
	// status is what the last look found; known reports whether one has finished, gen counts the looks so only the newest one lands.
	status cliToolStatus
	known  bool
	gen    int
	// installing is an install under way; err is why the last one failed, until the next one starts.
	installing bool
	err        string
	// copied is the text the card last put on the clipboard, so its button reads 已拷贝 while the text stays the same.
	copied string
}

func newCLIToolUI() cliToolUI { return cliToolUI{env: cliToolEnvHere} }

// cliToolCheck looks for the kando command again off the UI thread, keeping what the last look found on the page until it lands.
func (a *app) cliToolCheck() {
	a.cliTool.gen++
	gen, env, update := a.cliTool.gen, a.cliTool.env, a.update
	go func() {
		status := cliToolLook(env())
		update(func() {
			if gen != a.cliTool.gen {
				return
			}
			a.cliTool.status, a.cliTool.known = status, true
		})
	}()
}

// cliToolStartInstall links kando to this app's executable off the UI thread (cliToolInstall), then looks again, so the card says where the command went and whether a terminal finds it.
func (a *app) cliToolStartInstall() {
	if a.cliTool.installing {
		return
	}
	a.cliTool.installing, a.cliTool.err = true, ""
	a.cliTool.gen++
	gen, env, update := a.cliTool.gen, a.cliTool.env, a.update
	go func() {
		here := env()
		exe := cliToolLook(here).exe
		_, err := cliToolInstall(here, exe)
		status := cliToolLook(here)
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

// cliToolErrorText is why an install failed in the app's language, for the ones cliToolInstall refuses itself; any other error, as the system's permission denied, is shown as it is. It reads the locale, so it runs on the UI thread.
func cliToolErrorText(err error) string {
	var blocked *cliToolBlockedError
	switch {
	case errors.Is(err, errCLIToolTemporary):
		return locales.SettingsGeneralCLIMoveFirst()
	case errors.As(err, &blocked):
		paths := make([]string, len(blocked.paths))
		for i, path := range blocked.paths {
			paths[i] = films.DisplayPath(path)
		}
		return locales.SettingsGeneralCLIBlocked(strings.Join(paths, ", "))
	}
	return err.Error()
}

// cliToolCommand is what an MCP client should run: the kando a terminal finds when it is this app, else the link the card installed, else the executable itself, each a full path, as MCP clients start their servers without a shell's PATH. It is "" while Kando runs from a place that goes away (cliToolTemporary), where every one of them leads to a path that soon stops working, so the card asks to move Kando first instead of giving out a configuration that breaks.
func (s cliToolStatus) cliToolCommand() string {
	switch {
	case s.temporary:
		return ""
	case s.ours:
		return s.command
	case s.link != "":
		return s.link
	}
	return s.exe
}
