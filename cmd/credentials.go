package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	clientauthentication "k8s.io/client-go/pkg/apis/clientauthentication/v1"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func credentials(c *cobra.Command, home cloud.Cloud, region cloud.Region) error {
	address, err := proxyAddress()
	if err != nil {
		return err
	}
	if err := tunnel(c.Context(), region, address); err != nil {
		return err
	}
	token := exec.Command("kubectl-oidc_login", oidcArgs(home)...)
	token.Stdin, token.Stdout, token.Stderr = c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()
	return token.Run()
}

func oidcArgs(home cloud.Cloud) []string {
	args := []string{"get-token",
		"--oidc-issuer-url=" + home.Issuer,
		"--oidc-client-id=" + home.ClientID,
		"--oidc-client-secret=" + home.ClientSecret}
	for _, scope := range home.Scopes {
		args = append(args, "--oidc-extra-scope="+scope)
	}
	return args
}

func proxyAddress() (string, error) {
	info := clientauthentication.ExecCredential{Spec: clientauthentication.ExecCredentialSpec{Cluster: &clientauthentication.Cluster{}}}
	_ = json.Unmarshal([]byte(os.Getenv("KUBERNETES_EXEC_INFO")), &info)
	proxy, err := url.Parse(info.Spec.Cluster.ProxyURL)
	if err != nil || proxy.Host == "" {
		return "", errors.New("kubectl runs this for a ccloud context; sign in with ccloud clusters auth")
	}
	return proxy.Host, nil
}

func tunnel(ctx context.Context, region cloud.Region, address string) error {
	if listening(address) {
		return nil
	}
	cloudflared := exec.Command("cloudflared", "access", "tcp", "--hostname", region.Host, "--url", address)
	cloudflared.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cloudflared.Start(); err != nil {
		return fmt.Errorf("starting cloudflared for %s: %w", region.Host, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cloudflared.Wait() }()

	ctx, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	for {
		select {
		case err := <-exited:
			return fmt.Errorf("cloudflared exited before %s was reachable at %s: %v", region.Host, address, err)
		case <-ctx.Done():
			return fmt.Errorf("cloudflared did not open %s for %s in time", address, region.Host)
		case <-time.After(100 * time.Millisecond):
			if listening(address) {
				return nil
			}
		}
	}
}

func listening(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
