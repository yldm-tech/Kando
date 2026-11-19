package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
)

// previewTextMax is the most characters of one text of a pack the preview writes, so a pack cannot make the comment longer than a pull request takes.
const previewTextMax = 300

// writePreview writes each pack of results as Markdown for a comment on a pull request: a heading with its id and whether it passed, a table of what it is (kind, name and summary in each of its languages, version, size, license, the languages it is for, its signature), what it adds to Kando and, for a script plugin, what it asks for and a line flagging that it runs code, with the combination the install confirmation flags too (plugins.Risky). Everything a pack says, and the path it came from, is written inside code spans, so its text cannot add links, images, HTML, mentions or references to the comment.
func writePreview(w io.Writer, results []checked) error {
	var b strings.Builder
	passed := 0
	for _, c := range results {
		if c.err == nil {
			passed++
		}
	}
	fmt.Fprintf(&b, "## Kando plugin check\n\n%d of %d packs passed.\n", passed, len(results))
	for _, c := range results {
		b.WriteString("\n")
		previewPack(&b, c)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// previewPack writes the Markdown of the pack c to b.
func previewPack(b *strings.Builder, c checked) {
	if c.err != nil {
		fmt.Fprintf(b, "### Failed: %s\n\n", code(filepath.Base(c.path)))
		fmt.Fprintf(b, "| | |\n|---|---|\n| Path | %s |\n| Validation | **failed** (%s): %s |\n", cell(c.path), errorCode(c.err), cell(c.err.Error()))
		return
	}
	p := c.Pack
	entry := index.FromPack(p, "", c.Size, c.SHA256, c.Source)
	fmt.Fprintf(b, "### %s\n\n", code(p.ID))
	b.WriteString("| | |\n|---|---|\n")
	row := func(name, value string) { fmt.Fprintf(b, "| %s | %s |\n", name, value) }
	row("Path", cell(c.path))
	row("Kind", cell(string(p.Kind)))
	row("Name", texts(p.Name))
	if len(p.Description) > 0 {
		row("Summary", texts(p.Description))
	}
	row("Version", cell(p.Version))
	if c.File {
		row("Size", byteSize(c.Size))
		row("SHA-256", cell(entry.SHA256))
	} else {
		row("Size", "a folder, not a pack file: the directory lists pack files only")
	}
	row("License", cell(p.License))
	if p.Author != "" {
		row("Author", cell(p.Author))
	}
	if p.Homepage != "" {
		row("Homepage", cell(p.Homepage))
	}
	if len(entry.Languages) > 0 {
		row("Languages", cells(entry.Languages))
	}
	if entry.MinKando != "" {
		row("Needs Kando", cell(entry.MinKando)+" or newer")
	}
	switch {
	case p.Kind == plugins.KindScript:
		row("Signature", cell(string(c.Source))+", release key "+cell(c.Signer))
	default:
		row("Signature", "none: a data pack is not signed; the index lists its SHA-256")
	}
	row("Validation", "passed")
	if adds := previewAdds(p.Manifest); len(adds) > 0 {
		b.WriteString("\n**Adds**\n\n")
		for _, a := range adds {
			fmt.Fprintf(b, "- %s\n", a)
		}
	}
	if p.Kind != plugins.KindScript {
		return
	}
	b.WriteString("\n**Asks for**\n\n")
	asks := previewAsks(p.Manifest)
	if len(asks) == 0 {
		b.WriteString("- nothing\n")
	}
	for _, a := range asks {
		fmt.Fprintf(b, "- %s\n", a)
	}
	b.WriteString("\n> [!WARNING]\n> A script plugin runs code in Kando's sandbox: review what it asks for, and its network rules, before merging.")
	if plugins.Risky(p.Manifest, p.Declared()) {
		b.WriteString(" It reads text someone else may have written (subtitle lines or AI replies) and can send data out of Kando, the combination Kando flags in its install confirmation.")
	}
	b.WriteString("\n")
}

// previewAdds is what the pack m adds to Kando, a line each, in English.
func previewAdds(m plugins.Manifest) []string {
	var out []string
	switch {
	case m.Glossary != nil:
		out = append(out, fmt.Sprintf("a glossary of %d terms, %s into %s", len(m.Glossary.Terms), code(m.Glossary.From), code(m.Glossary.To)))
	case m.Dictionary != nil:
		out = append(out, fmt.Sprintf("a dictionary (%s, %d files), %s into %s", code(m.Dictionary.Format), len(m.Dictionary.Files), codes(m.Dictionary.From), codes(m.Dictionary.To)))
	case m.Wordlist != nil:
		out = append(out, fmt.Sprintf("a word list of %s in %d levels", code(m.Wordlist.Language), len(m.Wordlist.Tiers)))
	case m.Service != nil:
		s := fmt.Sprintf("an AI service at %s (%s)", code(m.Service.BaseURL), code(m.Service.Group))
		if m.Service.Model != "" {
			s += ", model " + code(m.Service.Model)
		}
		out = append(out, s+"; the user gives it their own key")
	case m.Site != nil:
		out = append(out, "video sites for the built-in browser's download button: "+codes(m.Site.Domains())+"; each works only once the user allows it")
	case m.Task != nil:
		out = append(out, fmt.Sprintf("a task for the user's AI service on %s, shown as %s, with %d reply fields; it spends the user's tokens, so it is installed off", code(m.Task.Scope), code(m.Task.Render), len(m.Task.Reply.Fields)))
	case m.Style != nil:
		out = append(out, fmt.Sprintf("a translation style preset (a prompt of %d characters)", len([]rune(m.Style.Prompt))))
	case m.Template != nil:
		out = append(out, fmt.Sprintf("an export template writing %s with %d columns", code(m.Template.Format), len(m.Template.Columns)))
	}
	if c := m.Contributes; c != nil {
		for _, part := range []struct {
			n    int
			what string
		}{
			{len(c.Commands), "commands"},
			{len(c.Lookups), "word lookup sources"},
			{len(c.SubtitleFormats), "subtitle formats"},
			{len(c.Exporters), "export targets"},
			{len(c.Annotators), "line annotators"},
		} {
			if part.n > 0 {
				out = append(out, fmt.Sprintf("%d %s", part.n, part.what))
			}
		}
	}
	if len(m.Settings) > 0 {
		out = append(out, fmt.Sprintf("%d settings", len(m.Settings)))
	}
	if len(m.Events) > 0 {
		out = append(out, "listens to "+codes(m.Events))
	}
	return out
}

// previewAsks is what the script plugin m asks for, a line each: its permissions, those it needs and those it can do without, its network rules and the credentials Kando sends for it.
func previewAsks(m plugins.Manifest) []string {
	var out []string
	if m.Capabilities != nil && len(m.Capabilities.Host) > 0 {
		out = append(out, "permissions: "+codes(m.Capabilities.Host))
	}
	if len(m.Optional) > 0 {
		var names []string
		for _, p := range m.Optional {
			names = append(names, string(p))
		}
		out = append(out, "optional permissions: "+codes(names))
	}
	if m.Capabilities != nil && m.Capabilities.Network != nil && len(m.Capabilities.Network.HTTP) > 0 {
		out = append(out, "network: "+code(plugins.RulesText(m.Capabilities.Network.HTTP)))
	}
	for _, cred := range m.Credentials {
		out = append(out, fmt.Sprintf("credential %s, sent as the %s header only to %s", code(cred.ID), code(cred.Header), codes(cred.Hosts)))
	}
	return out
}

// texts is the text t in each of its languages, English first, then by tag, for a table cell.
func texts(t plugins.Text) string {
	tags := make([]string, 0, len(t))
	for tag := range t {
		tags = append(tags, tag)
	}
	slices.SortFunc(tags, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "en":
			return -1
		case b == "en":
			return 1
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, len(tags))
	for i, tag := range tags {
		parts[i] = cell(tag) + " " + cell(t[tag])
	}
	return strings.Join(parts, "<br>")
}

// byteSize is n bytes for people.
func byteSize(n int64) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MiB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + " KiB"
	}
	return strconv.FormatInt(n, 10) + " bytes"
}

// code is s as a Markdown code span outside a table: on one line, at most previewTextMax characters, fenced with one backtick more than the longest run of them in s, so nothing in it is read as Markdown or HTML, and mentions and references in it link nowhere.
func code(s string) string {
	s = oneLine(s)
	if r := []rune(s); len(r) > previewTextMax {
		s = string(r[:previewTextMax]) + "…"
	}
	if s == "" {
		return "(empty)"
	}
	run, longest := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	// A space on each side keeps a backtick at either end out of the fence; the space is dropped when the span is read.
	return fence + " " + s + " " + fence
}

// cell is s as a code span inside a table cell, whose pipes are escaped, as GitHub reads a pipe in a cell as its end even inside a code span.
func cell(s string) string { return strings.ReplaceAll(code(s), "|", `\|`) }

// codes is each of ss as a code span, comma-separated.
func codes(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = code(s)
	}
	return strings.Join(out, ", ")
}

// cells is codes for a table cell.
func cells(ss []string) string { return strings.ReplaceAll(codes(ss), "|", `\|`) }
