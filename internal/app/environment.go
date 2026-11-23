package app

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

var lasting = []string{"prod", "stage", "ci"}

type Environment struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	URL       string `json:"url,omitempty"`
	Region    string `json:"-"`
}

func (e Environment) Lasting() bool { return slices.Contains(lasting, e.Name) }

func (e Environment) Compare(other Environment) int {
	return cmp.Or(cmp.Compare(e.rank(), other.rank()), byNumber(e.Name, other.Name))
}

func (e Environment) rank() int {
	if i := slices.Index(lasting, e.Name); i >= 0 {
		return i
	}
	return len(lasting)
}

func byNumber(a, b string) int {
	nameA, numberA := trailingNumber(a)
	nameB, numberB := trailingNumber(b)
	return cmp.Or(strings.Compare(nameA, nameB), cmp.Compare(numberA, numberB))
}

func trailingNumber(name string) (string, int) {
	prefix := strings.TrimRight(name, "0123456789")
	number, _ := strconv.Atoi(name[len(prefix):])
	return prefix, number
}

type Environments struct {
	App string
	All []Environment
}

func (e Environments) Named(name string) (Environment, error) {
	var found []Environment
	var has []string
	for _, environment := range e.All {
		has = append(has, fmt.Sprintf("%s (%s)", environment.Name, environment.Region))
		if environment.Name == name {
			found = append(found, environment)
		}
	}
	switch len(found) {
	case 0:
		return Environment{}, fmt.Errorf("no region hosts %s's %s; it has %s", e.App, name, strings.Join(has, ", "))
	case 1:
		return found[0], nil
	}
	return Environment{}, fmt.Errorf("%s both host %s's %s; name one with --region", And(regionsOf(found)), e.App, name)
}

func (e Environments) Feature() (Environment, error) {
	features := slices.DeleteFunc(slices.Clone(e.All), Environment.Lasting)
	switch len(features) {
	case 0:
		return Environment{}, fmt.Errorf("no feature environment of %s is running to copy; open a pull request, or name one with -e", e.App)
	case 1:
		return features[0], nil
	}
	names := make([]string, len(features))
	for i, f := range features {
		names[i] = f.Name
	}
	return Environment{}, fmt.Errorf("%s has %d feature environments running, %s; name the one to copy with -e", e.App, len(features), And(names))
}

func (e Environments) Regions() []string { return slices.Compact(regionsOf(e.All)) }

func regionsOf(environments []Environment) []string {
	regions := make([]string, len(environments))
	for i, environment := range environments {
		regions[i] = environment.Region
	}
	return regions
}

func And(names []string) string {
	last := len(names) - 1
	if last == 0 {
		return names[0]
	}
	return strings.Join(names[:last], ", ") + " and " + names[last]
}
