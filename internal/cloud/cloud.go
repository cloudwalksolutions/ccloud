package cloud

import (
	"fmt"
	"strings"
)

type Cloud struct {
	Name         string
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       []string
	APIServer    string
	Regions      []Region
}

type Region struct {
	Name      string
	Host      string
	Port      int
	Authority []byte
	Logs      string
}

func (r Region) Context() string { return "ccloud-" + r.Name }

func (c Cloud) Names() []string {
	names := make([]string, len(c.Regions))
	for i, region := range c.Regions {
		names[i] = region.Name
	}
	return names
}

func (c Cloud) Find(name string) (Region, error) {
	for _, region := range c.Regions {
		if region.Name == name {
			return region, nil
		}
	}
	return Region{}, fmt.Errorf("no region %q; %s has %s", name, c.Name, strings.Join(c.Names(), ", "))
}
