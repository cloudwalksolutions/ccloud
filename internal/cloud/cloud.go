package cloud

import (
	"fmt"
	"strings"
	"time"
)

type Cloud struct {
	Name           string
	Issuer         string
	ClientID       string
	ClientSecret   string
	Scopes         []string
	APIServer      string
	Regions        []Region
	RepoDir        string
	Releases       string
	Database       Database
	Env            Env
	GatewayTimeout time.Duration
}

type Database struct {
	Primary   string
	Port      int
	Secret    string
	Reset     string
	Backup    string
	Fetch     string
	Fetched   string
	Restore   string
	Container string
	Offsite   Offsite
}

type Offsite struct {
	Namespace string
	CronJob   string
}

type Env struct {
	Deployment    string
	Container     string
	File          string
	SecretManager string
}

func (c Cloud) AppDir() string {
	if c.RepoDir != "" {
		return c.RepoDir
	}
	return ".ccloud"
}

type Region struct {
	Name      string
	Host      string
	Port      int
	Authority []byte
	Gateway   string
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
