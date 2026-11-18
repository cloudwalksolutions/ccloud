package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/kubeconfig"
)

func setContext(c *cobra.Command, region cloud.Region) error {
	namespace, _ := c.Flags().GetString("namespace")
	if err := kubeconfig.Use(kubeconfig.Path(), region.Context(), namespace); err != nil {
		return signInError(err, region)
	}
	return switched(c, region)
}

func connect[T any](region cloud.Region, client func(*rest.Config) (T, error)) (T, error) {
	var connected T
	cfg, err := kubeconfig.REST(kubeconfig.Path(), region.Context())
	if err != nil {
		return connected, signInError(err, region)
	}
	connected, err = client(cfg)
	if err != nil {
		err = fmt.Errorf("your %s context is damaged; sign in again with ccloud clusters auth --region %s: %w", region.Context(), region.Name, err)
	}
	return connected, err
}

func signInError(err error, region cloud.Region) error {
	if errors.Is(err, kubeconfig.ErrNoContext) {
		err = fmt.Errorf("you have not signed in to %s yet; run ccloud clusters auth --region %s", region.Name, region.Name)
	}
	return err
}

func switched(c *cobra.Command, region cloud.Region) error {
	_, err := fmt.Fprintf(c.OutOrStdout(), "Switched to context %q.\n", region.Context())
	return err
}
