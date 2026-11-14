//go:build ignore

// Command arbconv converts Kando's MyGo catalogs (locales/*.json, ARB: "key": message, "@key": {description, placeholders}) into NSGo's (locales/*.yaml: key: message, or a mapping of message, description and placeholders). Messages, descriptions, placeholder order, types and examples are copied as they are, so ICU syntax, plurals, selects and apostrophe quoting keep their meaning; MyGo's "mygo.appName" becomes NSGo's "appName", which the source gains as "Kando". arbconv/supply/<tag>.yaml, NSGo's own words in a language NSGo does not translate to, is appended to that language's file. Every file written is read back and compared with its JSON. Run from apps/kando/locales:
//
//	go run arbconv/main.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

var tags = []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"}

const source = "en"

// appName is the message NSGo names the app by in each language; MyGo's catalogs called it mygo.appName and left the source's out.
const (
	appName     = "appName"
	mygoAppName = "mygo.appName"
	appNameEn   = "Kando"
	appNameDesc = "The app's name in this language, which its packages show: the menu bar, the Dock, Finder and the Start menu."
)

// placeholder is one ARB placeholder, its fields in their order.
type placeholder struct {
	name    string
	fields  []string
	values  []json.RawMessage
}

// message is one key of a catalog.
type message struct {
	key, text, desc string
	placeholders    []placeholder
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "arbconv:", err)
		os.Exit(1)
	}
}

func run() error {
	for _, tag := range tags {
		msgs, err := readARB(tag + ".json")
		if err != nil {
			return err
		}
		if tag == source {
			msgs = append([]message{{key: appName, text: appNameEn, desc: appNameDesc}}, msgs...)
		}
		var b bytes.Buffer
		for _, m := range msgs {
			writeMessage(&b, m, tag == source)
		}
		supply, err := os.ReadFile("arbconv/supply/" + tag + ".yaml")
		switch {
		case err == nil:
			b.WriteString("\n")
			b.Write(supply)
		case !os.IsNotExist(err):
			return err
		}
		if err := os.WriteFile(tag+".yaml", b.Bytes(), 0o644); err != nil {
			return err
		}
		if err := verify(tag+".yaml", msgs); err != nil {
			return err
		}
		fmt.Printf("%s.yaml: %d messages\n", tag, len(msgs))
	}
	return nil
}

// readARB reads an ARB file's messages in their order, with the metadata of "@key" entries.
func readARB(path string) ([]message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := expectDelim(dec, '{'); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var order []string
	texts := map[string]string{}
	meta := map[string]message{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		key := tok.(string)
		switch {
		case key == "@@locale":
			var s string
			if err := dec.Decode(&s); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", path, key, err)
			}
		case strings.HasPrefix(key, "@"):
			m, err := readMeta(dec)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", path, key, err)
			}
			meta[key[1:]] = m
		default:
			var s string
			if err := dec.Decode(&s); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", path, key, err)
			}
			if key == mygoAppName {
				key = appName
			}
			if _, dup := texts[key]; dup {
				return nil, fmt.Errorf("%s: %s twice", path, key)
			}
			order = append(order, key)
			texts[key] = s
		}
	}
	msgs := make([]message, 0, len(order))
	for _, key := range order {
		m := meta[key]
		delete(meta, key)
		m.key, m.text = key, texts[key]
		msgs = append(msgs, m)
	}
	for key := range meta {
		return nil, fmt.Errorf("%s: @%s describes no message", path, key)
	}
	return msgs, nil
}

func readMeta(dec *json.Decoder) (message, error) {
	var m message
	if err := expectDelim(dec, '{'); err != nil {
		return m, err
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return m, err
		}
		switch field := tok.(string); field {
		case "description":
			if err := dec.Decode(&m.desc); err != nil {
				return m, err
			}
		case "placeholders":
			if err := expectDelim(dec, '{'); err != nil {
				return m, err
			}
			for dec.More() {
				tok, err := dec.Token()
				if err != nil {
					return m, err
				}
				p := placeholder{name: tok.(string)}
				if err := expectDelim(dec, '{'); err != nil {
					return m, err
				}
				for dec.More() {
					tok, err := dec.Token()
					if err != nil {
						return m, err
					}
					var v json.RawMessage
					if err := dec.Decode(&v); err != nil {
						return m, err
					}
					p.fields = append(p.fields, tok.(string))
					p.values = append(p.values, v)
				}
				if err := expectDelim(dec, '}'); err != nil {
					return m, err
				}
				m.placeholders = append(m.placeholders, p)
			}
			if err := expectDelim(dec, '}'); err != nil {
				return m, err
			}
		default:
			return m, fmt.Errorf("unknown field %s", field)
		}
	}
	return m, expectDelim(dec, '}')
}

func expectDelim(dec *json.Decoder, d json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != d {
		return fmt.Errorf("want %v, have %v", d, tok)
	}
	return nil
}

// writeMessage writes m as a string, or, in the source with a description or placeholders, as a mapping.
func writeMessage(w io.Writer, m message, src bool) {
	if !src || m.desc == "" && len(m.placeholders) == 0 {
		fmt.Fprintf(w, "%s: %s\n", m.key, quote(m.text))
		return
	}
	fmt.Fprintf(w, "%s:\n  message: %s\n", m.key, quote(m.text))
	if m.desc != "" {
		fmt.Fprintf(w, "  description: %s\n", quote(m.desc))
	}
	if len(m.placeholders) > 0 {
		fmt.Fprintf(w, "  placeholders:\n")
		for _, p := range m.placeholders {
			var fields []string
			for i, f := range p.fields {
				fields = append(fields, f+": "+scalar(p.values[i]))
			}
			fmt.Fprintf(w, "    %s: { %s }\n", p.name, strings.Join(fields, ", "))
		}
	}
}

// scalar writes a JSON value as a YAML flow scalar: a string quoted, else the value as JSON writes it.
func scalar(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return quote(s)
	}
	return string(v)
}

// quote writes s as a YAML double-quoted scalar, escaping what YAML does not take as printable in one.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == utf8.RuneError, r < 0x20, r == 0x7f, r >= 0x80 && r < 0xa0, r == 0x2028, r == 0x2029, r == 0xfeff, r >= 0xd800 && r <= 0xdfff, r == 0xfffe, r == 0xffff:
			if r > 0xffff {
				fmt.Fprintf(&b, `\U%08x`, r)
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// verify reads path back and checks that its messages, descriptions and placeholders are msgs'.
func verify(path string, msgs []message) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	root := doc.Content[0]
	got := map[string]*yaml.Node{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		got[root.Content[i].Value] = root.Content[i+1]
	}
	for _, m := range msgs {
		v := got[m.key]
		if v == nil {
			return fmt.Errorf("%s: %s is missing", path, m.key)
		}
		text, desc := v.Value, ""
		var ph *yaml.Node
		if v.Kind == yaml.MappingNode {
			text = ""
			for i := 0; i+1 < len(v.Content); i += 2 {
				switch v.Content[i].Value {
				case "message":
					text = v.Content[i+1].Value
				case "description":
					desc = v.Content[i+1].Value
				case "placeholders":
					ph = v.Content[i+1]
				}
			}
		}
		if text != m.text {
			return fmt.Errorf("%s: %s reads back as %s, not %s", path, m.key, strconv.Quote(text), strconv.Quote(m.text))
		}
		if v.Kind == yaml.MappingNode && desc != m.desc {
			return fmt.Errorf("%s: %s's description reads back as %q", path, m.key, desc)
		}
		if v.Kind == yaml.MappingNode && len(m.placeholders) > 0 {
			if ph == nil || len(ph.Content) != 2*len(m.placeholders) {
				return fmt.Errorf("%s: %s's placeholders read back wrong", path, m.key)
			}
			for i, p := range m.placeholders {
				spec := ph.Content[2*i+1]
				if ph.Content[2*i].Value != p.name || len(spec.Content) != 2*len(p.fields) {
					return fmt.Errorf("%s: %s's placeholder %s reads back wrong", path, m.key, p.name)
				}
				for j, f := range p.fields {
					var want any
					if err := json.Unmarshal(p.values[j], &want); err != nil {
						return err
					}
					var have any
					if err := spec.Content[2*j+1].Decode(&have); err != nil {
						return err
					}
					if spec.Content[2*j].Value != f || fmt.Sprint(have) != fmt.Sprint(want) {
						return fmt.Errorf("%s: %s's placeholder %s: %s reads back as %v, not %v", path, m.key, p.name, f, have, want)
					}
				}
			}
		}
	}
	return nil
}
