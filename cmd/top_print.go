package cmd

import (
	"fmt"
	"math"
	"strings"

	"github.com/cloudwalksolutions/ccloud/internal/app"
)

func (p palette) severe(s app.Severity, text string) string {
	return p.paint(map[app.Severity]string{app.Hint: "2", app.Warn: "33", app.Bad: "31"}[s], text)
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

func percent(fraction float64) string { return fmt.Sprintf("%.0f%%", math.Round(fraction*100)) }

func memory(bytes float64) string {
	if bytes >= 1<<30 {
		return fmt.Sprintf("%.1fGi", bytes/(1<<30))
	}
	return fmt.Sprintf("%.0fMi", bytes/(1<<20))
}
