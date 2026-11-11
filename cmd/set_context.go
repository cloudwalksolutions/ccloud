package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/kubeconfig"
)

func setContext(c *cobra.Command, region cloud.Region) error {
	namespace, err := c.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	err = kubeconfig.Use(kubeconfig.Path(), region.Context(), namespace)
	if errors.Is(err, kubeconfig.ErrNoContext) {
		return errNoSignInFor(region)
	}
	if err != nil {
		return err
	}
	return switched(c, region)
}

func errNoSignInFor(region cloud.Region) error {
	return fmt.Errorf("you have not signed in to %s yet; run ccloud clusters auth --region %s", region.Name, region.Name)
}

func switched(c *cobra.Command, region cloud.Region) error {
	_, err := fmt.Fprintf(c.OutOrStdout(), "Switched to context %q.\n", region.Context())
	return err
}
