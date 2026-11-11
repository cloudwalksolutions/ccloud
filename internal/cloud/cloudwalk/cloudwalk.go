package cloudwalk

import (
	_ "embed"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

var (
	//go:embed ca/us-central1.crt
	usCentral1 []byte
	//go:embed ca/us-east1.crt
	usEast1 []byte
)

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
			region("us-central1", "k8s.us-central1.cloudwalksolutions.net", 16444, usCentral1),
			region("us-east1", "k8s.us-east1.cloudwalksolutions.net", 16443, usEast1),
		},
	}
}

func region(name, host string, port int, authority []byte) cloud.Region {
	return cloud.Region{Name: name, Host: host, Port: port, Authority: authority, Logs: "https://logs." + name + ".cloudwalksolutions.net"}
}
