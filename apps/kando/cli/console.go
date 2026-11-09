package cli

// stdio is which of standard input, output and error a process was started with.
type stdio struct{ in, out, err bool }

// needsConsole reports whether a command started with the standard streams in has to borrow its terminal's console to be heard, which only Windows asks, as there the app is a GUI program that starts with none. It does when any of them is missing, except for `kando mcp` started with its input and output piped, as an AI agent starts it: those are its whole conversation, and a console borrowed for standard error would only scribble on the terminal the agent itself may run in.
func needsConsole(args []string, have stdio) bool {
	if have.in && have.out && have.err {
		return false
	}
	if len(args) > 0 && args[0] == "mcp" && have.in && have.out {
		return false
	}
	return true
}
