package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"kando/apps/kando/plugins"
)

// pluginsRoot is the plugins folder `kando plugin list` reads, plugins.DefaultRoot; tests point it at a folder of their own.
var pluginsRoot = plugins.DefaultRoot

// pluginInfo is a pack as `kando plugin` prints it with --json. Enabled and Dir are only for an installed pack, as list prints it.
type pluginInfo struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Version string `json:"version"`
	Name    string `json:"name"`
	Hash    string `json:"hash"`
	Enabled *bool  `json:"enabled,omitempty"`
	Dir     string `json:"dir,omitempty"`
}

func newPluginInfo(p plugins.Pack) pluginInfo {
	return pluginInfo{ID: p.ID, Kind: string(p.Kind), Version: p.Version, Name: p.Name.In("en"), Hash: p.Hash}
}

// pluginIssue is a folder of the plugins folder that is not a valid pack, as `kando plugin list --json` prints it.
type pluginIssue struct {
	Dir   string `json:"dir"`
	Error string `json:"error"`
}

// runPlugin is `kando plugin`, which works without the app: validate checks a pack, a zip or a folder, with the checks the app installs it with (plugins.Stage and plugins.Validate), copying it into a folder of its own that it removes afterwards; list lists the packs in the plugins folder (plugins.Scan) and whether each is turned on (plugins.State).
func runPlugin(e *env, args []string) error {
	fs := e.flags()
	fs.BoolVar(&e.json, "json", false, "print the result as JSON")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) == 0 {
		return usagef("needs a subcommand: validate or list")
	}
	verb, positional := positional[0], positional[1:]
	switch verb {
	case "validate":
		if len(positional) != 1 {
			return usagef("validate takes one pack, a .zip file or a folder, got %d arguments", len(positional))
		}
		return pluginValidate(e, positional[0])
	case "list":
		if len(positional) > 0 {
			return usagef("list takes no arguments, got %d", len(positional))
		}
		return pluginList(e)
	}
	return usagef("unknown subcommand %q: use validate or list", verb)
}

func pluginValidate(e *env, src string) error {
	root, err := os.MkdirTemp("", "kando-plugin-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	_, p, err := plugins.Stage(root, src)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	info := newPluginInfo(p)
	if e.json {
		return pluginJSON(e, info)
	}
	return table(e.stdout, [][]string{{"id", info.ID}, {"kind", info.Kind}, {"version", info.Version}, {"name", info.Name}, {"hash", info.Hash}})
}

func pluginList(e *env) error {
	root := pluginsRoot()
	packs, issues := plugins.Scan(root)
	state, err := plugins.LoadState(root)
	if err != nil {
		// Every pack then reads as turned off, as the app reads it.
		fmt.Fprintf(e.stderr, "kando plugin: %s\n", printable(err.Error()))
	}
	infos := make([]pluginInfo, 0, len(packs))
	for _, p := range packs {
		info := newPluginInfo(p)
		enabled := state.Enabled(p.ID)
		info.Enabled, info.Dir = &enabled, p.Dir
		infos = append(infos, info)
	}
	if e.json {
		out := struct {
			Plugins []pluginInfo  `json:"plugins"`
			Issues  []pluginIssue `json:"issues"`
		}{Plugins: infos, Issues: []pluginIssue{}}
		for _, issue := range issues {
			out.Issues = append(out.Issues, pluginIssue{Dir: issue.Dir, Error: issue.Err.Error()})
		}
		return pluginJSON(e, out)
	}
	for _, issue := range issues {
		fmt.Fprintf(e.stderr, "kando plugin: %s: %s\n", printable(issue.Dir), printable(issue.Err.Error()))
	}
	if len(infos) == 0 {
		fmt.Fprintln(e.stderr, "No plugins.")
		return nil
	}
	rows := [][]string{{"ID", "KIND", "VERSION", "STATE", "NAME"}}
	for _, info := range infos {
		on := "on"
		if !*info.Enabled {
			on = "off"
		}
		rows = append(rows, []string{info.ID, info.Kind, info.Version, on, info.Name})
	}
	return table(e.stdout, rows)
}

// pluginJSON writes v as indented JSON to the command's standard output.
func pluginJSON(e *env, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeJSON(e.stdout, data)
}
