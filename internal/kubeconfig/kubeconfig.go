package kubeconfig

import (
	"errors"
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"
	api "k8s.io/client-go/tools/clientcmd/api"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func Path() string {
	return clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
}

func Entry(home cloud.Cloud, region cloud.Region, self string) *api.Config {
	cfg := api.NewConfig()
	name := region.Context()
	cfg.Clusters[name] = &api.Cluster{
		Server:                   home.APIServer,
		CertificateAuthorityData: region.Authority,
		ProxyURL:                 fmt.Sprintf("socks5://127.0.0.1:%d", region.Port),
	}
	cfg.AuthInfos[name] = &api.AuthInfo{Exec: &api.ExecConfig{
		APIVersion:         "client.authentication.k8s.io/v1",
		Command:            self,
		Args:               []string{"clusters", "credentials", "--region", region.Name},
		InteractiveMode:    api.IfAvailableExecInteractiveMode,
		ProvideClusterInfo: true,
	}}
	cfg.Contexts[name] = &api.Context{Cluster: name, AuthInfo: name}
	cfg.CurrentContext = name
	return cfg
}

var ErrNoContext = errors.New("no such context")

func Use(path, context, namespace string) error {
	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if cfg == nil || cfg.Contexts[context] == nil {
		return ErrNoContext
	}
	cfg.CurrentContext = context
	if namespace != "" {
		cfg.Contexts[context].Namespace = namespace
	}
	return clientcmd.WriteToFile(*cfg, path)
}

func Merge(path string, entry *api.Config) error {
	cfg, err := clientcmd.LoadFromFile(path)
	if os.IsNotExist(err) {
		cfg, err = api.NewConfig(), nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	for name, cluster := range entry.Clusters {
		cfg.Clusters[name] = cluster
	}
	for name, user := range entry.AuthInfos {
		cfg.AuthInfos[name] = user
	}
	for name, context := range entry.Contexts {
		cfg.Contexts[name] = context
	}
	cfg.CurrentContext = entry.CurrentContext
	return clientcmd.WriteToFile(*cfg, path)
}
