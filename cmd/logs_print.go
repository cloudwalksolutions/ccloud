package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

type logPrinter struct {
	out    io.Writer
	format string
	color  palette
}

func (p logPrinter) print(entry gateway.Entry) {
	switch p.format {
	case "raw":
		fmt.Fprintln(p.out, entry.Line)
	case "json":
		_ = json.NewEncoder(p.out).Encode(entry)
	default:
		line := strings.ReplaceAll(strings.TrimRight(p.readable(entry.Line), "\n"), "\n", "\n    ")
		fmt.Fprintf(p.out, "%s %s %s %s\n", p.color.paint("90", stamp(entry.Time)), p.color.paint(hue(entry.Container), entry.Container), entry.Pod, line)
	}
}

func stamp(at time.Time) string {
	local := at.Local()
	if local.Format(time.DateOnly) == time.Now().Format(time.DateOnly) {
		return local.Format("15:04:05.000")
	}
	return local.Format("Jan 02 15:04:05.000")
}

func hue(container string) string {
	hues := []string{"32", "34", "35", "36", "92", "94"}
	sum := 0
	for _, r := range container {
		sum += int(r)
	}
	return hues[sum%len(hues)]
}

func (p logPrinter) readable(line string) string {
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return line
	}
	var parts []string
	if level, ok := fields["level"].(string); ok {
		parts = append(parts, p.badge(strings.ToUpper(level)))
	}
	if message, ok := fields["msg"].(string); ok {
		parts = append(parts, message)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if key != "level" && key != "msg" {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+value(fields[key]))
	}
	return strings.Join(parts, " ")
}

func (p logPrinter) badge(level string) string {
	switch level {
	case "ERROR", "FATAL", "PANIC":
		return p.color.paint("31", level)
	case "WARN", "WARNING":
		return p.color.paint("33", level)
	default:
		return p.color.paint("36", level)
	}
}

func value(v any) string {
	if text, ok := v.(string); ok {
		return text
	}
	var encoded bytes.Buffer
	_ = json.NewEncoder(&encoded).Encode(v)
	return strings.TrimSpace(encoded.String())
}
