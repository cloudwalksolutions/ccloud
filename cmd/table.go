package cmd

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

type table struct {
	out  io.Writer
	rows [][]string
}

var colorCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func newTable(out io.Writer, header ...string) *table {
	t := &table{out: out}
	if len(header) > 0 {
		bold := make([]string, len(header))
		for i, cell := range header {
			bold[i] = paletteFor(out).paint("01", cell)
		}
		t.rows = append(t.rows, bold)
	}
	return t
}

func (t *table) row(cells ...string) {
	t.rows = append(t.rows, cells)
}

func (t *table) print() error {
	var widths []int
	for _, row := range t.rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], visibleWidth(cell))
		}
	}
	var lines strings.Builder
	for _, row := range t.rows {
		var line strings.Builder
		for i, cell := range row {
			line.WriteString(cell)
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-visibleWidth(cell)+2))
			}
		}
		lines.WriteString(strings.TrimRight(line.String(), " ") + "\n")
	}
	_, err := fmt.Fprint(t.out, lines.String())
	return err
}

func visibleWidth(cell string) int {
	return utf8.RuneCountInString(colorCodes.ReplaceAllString(cell, ""))
}
