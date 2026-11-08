package regions

import (
	"embed"
	"fmt"
	"slices"
	"strings"
)

//go:embed ca/*.crt
var authorities embed.FS

type Region struct {
	Name string
	Host string
	Port int
}

func (r Region) Context() string { return "ccloud-" + r.Name }

func (r Region) Authority() []byte {
	pem, err := authorities.ReadFile("ca/" + r.Name + ".crt")
	if err != nil {
		panic(err)
	}
	return pem
}

var all = map[string]Region{
	"us-central1": {Name: "us-central1", Host: "k8s.us-central1.cloudwalksolutions.net", Port: 16444},
	"us-east1":    {Name: "us-east1", Host: "k8s.us-east1.cloudwalksolutions.net", Port: 16443},
}

func Find(name string) (Region, error) {
	region, ok := all[name]
	if !ok {
		names := make([]string, 0, len(all))
		for n := range all {
			names = append(names, n)
		}
		slices.Sort(names)
		return Region{}, fmt.Errorf("no region %q; the datacenter has %s", name, strings.Join(names, ", "))
	}
	return region, nil
}
