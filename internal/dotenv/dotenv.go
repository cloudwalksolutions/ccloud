package dotenv

import (
	"strconv"
	"strings"
)

type Var struct {
	Name, Value string
}

type Changes struct {
	Added, Changed, Unchanged int
	Differ                    []Difference
}

type Difference struct {
	Name, Was, Is string
}

func Merge(existing string, vars []Var, force bool) (string, Changes) {
	var changes Changes
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	lines := strings.SplitAfter(existing, "\n")
	defined := map[string]int{}
	for i, line := range lines {
		name, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && !strings.HasPrefix(name, "#") {
			defined[strings.TrimSpace(name)] = i
		}
	}
	for _, v := range vars {
		line := v.Name + "=" + quote(v.Value) + "\n"
		at, ok := defined[v.Name]
		if !ok {
			lines = append(lines, line)
			changes.Added++
			continue
		}
		_, was, _ := strings.Cut(lines[at], "=")
		was = unquote(strings.TrimSpace(was))
		switch {
		case was == v.Value:
			changes.Unchanged++
		case force:
			lines[at] = line
			changes.Changed++
		default:
			changes.Differ = append(changes.Differ, Difference{Name: v.Name, Was: was, Is: v.Value})
		}
	}
	return strings.Join(lines, ""), changes
}

func quote(value string) string {
	if strings.ContainsAny(value, " #\"'\\\n\t$") {
		return strconv.Quote(value)
	}
	return value
}

func unquote(value string) string {
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return value
}
