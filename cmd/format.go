package cmd

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

func percent(fraction float64) string { return fmt.Sprintf("%.0f%%", math.Round(fraction*100)) }

func memory(bytes float64) string {
	if bytes >= 1<<30 {
		return fmt.Sprintf("%.1fGi", bytes/(1<<30))
	}
	return fmt.Sprintf("%.0fMi", bytes/(1<<20))
}

func quantity(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}

func hours(h float64) string { return strconv.FormatFloat(h, 'f', -1, 64) + "h" }

func truncated(secret string) string {
	return secret[:min(4, len(secret))] + "…"
}

func short(sha string) string {
	return sha[:min(len(sha), 7)]
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
