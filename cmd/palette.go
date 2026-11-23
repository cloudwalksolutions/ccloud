package cmd

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/cloudwalksolutions/ccloud/internal/app"
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

func terminal(stream any) bool {
	if os.Getenv("CCLOUD_FORCE_TTY") != "" {
		return true
	}
	file, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (p palette) paint(code, text string) string {
	if !p {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (p palette) bold(text string) string   { return p.paint("01", text) }
func (p palette) red(text string) string    { return p.paint("31", text) }
func (p palette) green(text string) string  { return p.paint("32", text) }
func (p palette) yellow(text string) string { return p.paint("33", text) }
func (p palette) cyan(text string) string   { return p.paint("36", text) }
func (p palette) dim(text string) string    { return p.paint("90", text) }

func (p palette) status(phase string) string {
	switch symbol := symbol(phase); symbol {
	case "✓":
		return p.green(symbol)
	case "X":
		return p.red(symbol)
	case "*":
		return p.yellow(symbol)
	default:
		return p.dim(symbol)
	}
}

func (p palette) severe(s app.Severity, text string) string {
	return map[app.Severity]func(string) string{app.Hint: p.dim, app.Warn: p.yellow, app.Bad: p.red}[s](text)
}

func (p palette) issues(issues []app.Issue) string {
	seen := map[string]bool{}
	var painted []string
	for _, i := range issues {
		if !seen[i.Text] {
			seen[i.Text] = true
			painted = append(painted, p.severe(i.Severity, i.Text))
		}
	}
	return strings.Join(painted, ", ")
}

func symbol(phase string) string {
	switch {
	case phase == "Succeeded":
		return "✓"
	case failed(phase):
		return "X"
	case phase == "Skipped", phase == "Omitted":
		return "-"
	default:
		return "*"
	}
}
