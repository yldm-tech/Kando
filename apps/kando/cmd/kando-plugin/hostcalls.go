package main

import (
	"fmt"
	"slices"
	"strings"

	"kando/apps/kando/plugins"
)

// waitingCalls are the host calls Kando answers only once something outside the engine did (docs/plugin-system.md 5.3), and gives up on after two minutes with the code timeout. host.call waits 30 s unless told otherwise, so a plugin passes {timeout: 150000} to them, to hear Kando's answer rather than its own timeout (docs/plugin-system.md 3.3).
var waitingCalls = []plugins.Perm{plugins.PermLLMChat, plugins.PermSoundRead, plugins.PermCredFetch}

// hostCall is a call host.call("kando.…", …) the code of a script plugin makes with the permission's name written out: the name, and whether it passes host.call's options (a third argument), which is where a timeout is.
type hostCall struct {
	name    string
	options bool
}

// hostNotes is what the preview tells the reviewer of how the code bundle of the script plugin m uses what m declares (docs/plugin-system.md 4.3), a line each: the permissions it calls host.call with but does not declare, which Kando refuses (denied); those it declares but never calls host.call with, which it asks the user for without needing them; and the calls of waitingCalls without host.call's options, which give up after 30 s. It reads the bundle as text and sees only a call with the name written out in it, not one through a variable or a name built at run time: these are notes, not reasons to refuse the pack.
func hostNotes(m plugins.Manifest, bundle string) []string {
	var declared []string
	if m.Capabilities != nil {
		declared = append(declared, m.Capabilities.Host...)
	}
	for _, p := range m.Optional {
		declared = append(declared, string(p))
	}
	calls := jsHostCalls(bundle)
	var out []string
	var undeclared, untimed []string
	for _, c := range calls {
		if !slices.Contains(declared, c.name) && !slices.Contains(undeclared, c.name) {
			undeclared = append(undeclared, c.name)
		}
		if !c.options && slices.Contains(waitingCalls, plugins.Perm(c.name)) && !slices.Contains(untimed, c.name) {
			untimed = append(untimed, c.name)
		}
	}
	if len(undeclared) > 0 {
		out = append(out, "calls "+codes(undeclared)+" without declaring it: Kando refuses the call with denied")
	}
	var unused []string
	for _, d := range declared {
		if !slices.ContainsFunc(calls, func(c hostCall) bool { return c.name == d }) {
			unused = append(unused, d)
		}
	}
	if len(unused) > 0 {
		out = append(out, "declares "+codes(unused)+" but its code never calls it: the user is asked for a permission the plugin may not need")
	}
	if len(untimed) > 0 {
		out = append(out, fmt.Sprintf("calls %s without host.call's options: it gives up after 30 s, before Kando's own 120 s; pass {timeout: 150000}", codes(untimed)))
	}
	return out
}

// jsHostCalls reads the JavaScript src as text and gives the calls .call("kando.…", …) in it, each with whether it has a third argument. It skips comments, and the text of a template literal, and counts the arguments of a call by the commas outside brackets and literals; a regular expression literal holding a bracket or a quote can mislead it, which only makes a note wrong.
func jsHostCalls(src string) []hostCall {
	var calls []hostCall
	for i := 0; i < len(src); {
		switch c := src[i]; {
		case c == '/' && i+1 < len(src) && (src[i+1] == '/' || src[i+1] == '*'):
			i = jsSkipComment(src, i)
		case c == '"' || c == '\'' || c == '`':
			end, value := jsString(src, i)
			if c != '`' && strings.HasPrefix(value, "kando.") && strings.HasSuffix(strings.TrimRight(src[:i], " \t\r\n"), ".call(") {
				calls = append(calls, hostCall{name: value, options: jsArgs(src, end) >= 3})
			}
			i = end
		default:
			i++
		}
	}
	return calls
}

// jsSkipComment is the index just past the comment that starts at i.
func jsSkipComment(src string, i int) int {
	if src[i+1] == '/' {
		if n := strings.IndexByte(src[i:], '\n'); n >= 0 {
			return i + n + 1
		}
		return len(src)
	}
	if n := strings.Index(src[i+2:], "*/"); n >= 0 {
		return i + 2 + n + 2
	}
	return len(src)
}

// jsString is the index just past the string or template literal that starts at i, and its text with escapes kept as they are.
func jsString(src string, i int) (int, string) {
	quote := src[i]
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case quote:
			return j + 1, src[i+1 : j]
		case '\n':
			if quote != '`' {
				return j, src[i+1 : j]
			}
		}
	}
	return len(src), src[i+1:]
}

// jsArgs is the number of arguments of the call whose first one ends at i: one, and one more for each comma before the call's closing parenthesis that is outside brackets, literals and comments.
func jsArgs(src string, i int) int {
	args, depth := 1, 0
	for i < len(src) {
		switch c := src[i]; {
		case c == '/' && i+1 < len(src) && (src[i+1] == '/' || src[i+1] == '*'):
			i = jsSkipComment(src, i)
			continue
		case c == '"' || c == '\'' || c == '`':
			i, _ = jsString(src, i)
			continue
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			if depth == 0 {
				return args
			}
			depth--
		case c == ',' && depth == 0:
			args++
		}
		i++
	}
	return args
}
