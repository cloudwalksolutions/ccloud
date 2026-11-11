package cloudwalk

import (
	"embed"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

//go:embed ca/*.crt
var authorities embed.FS

var (
	ClientID     string
	ClientSecret string
)

func Cloud() cloud.Cloud {
	return cloud.Cloud{
		Name:         "cloudwalk",
		Issuer:       "https://accounts.google.com",
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		Scopes:       []string{"email"},
		APIServer:    "https://kubernetes.default.svc.cluster.local",
		Regions: []cloud.Region{
			region("us-central1", "k8s.us-central1.cloudwalksolutions.net", 16444),
			region("us-east1", "k8s.us-east1.cloudwalksolutions.net", 16443),
		},
	}
}

func region(name, host string, port int) cloud.Region {
	authority, err := authorities.ReadFile("ca/" + name + ".crt")
	if err != nil {
		panic(err)
	}
	return cloud.Region{Name: name, Host: host, Port: port, Authority: authority, Logs: "https://logs." + name + ".cloudwalksolutions.net"}
}
