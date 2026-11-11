package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/cloudwalksolutions/ccloud/internal/logs"
)

type logPrinter struct {
	out    io.Writer
	format string
	colour palette
}

func (p logPrinter) print(entry logs.Entry) error {
	switch p.format {
	case "raw":
		_, err := fmt.Fprintln(p.out, entry.Line)
		return err
	case "json":
		return json.NewEncoder(p.out).Encode(entry)
	default:
		_, err := fmt.Fprintf(p.out, "%s %s %s\n", p.colour.paint("90", entry.Time.Local().Format("15:04:05")), entry.Pod, p.readable(entry.Line))
		return err
	}
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
		return p.colour.paint("31", level)
	case "WARN", "WARNING":
		return p.colour.paint("33", level)
	default:
		return p.colour.paint("36", level)
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
