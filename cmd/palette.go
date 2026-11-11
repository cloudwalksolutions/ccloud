package cmd

import (
	"io"
	"os"

	"golang.org/x/term"
)

type palette bool

func paletteFor(out io.Writer) palette {
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return palette(terminal(out))
}

func terminal(out io.Writer) bool {
	if os.Getenv("CCLOUD_FORCE_TTY") != "" {
		return true
	}
	file, ok := out.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (p palette) paint(code, text string) string {
	if !p {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (p palette) status(phase string) string {
	switch symbol := symbol(phase); symbol {
	case "✓":
		return p.paint("32", symbol)
	case "X":
		return p.paint("31", symbol)
	case "*":
		return p.paint("33", symbol)
	default:
		return p.paint("90", symbol)
	}
}
