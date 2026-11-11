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

	clientauthentication "k8s.io/client-go/pkg/apis/clientauthentication/v1"
)

func (c command) credentials() error {
	info := clientauthentication.ExecCredential{Spec: clientauthentication.ExecCredentialSpec{Cluster: &clientauthentication.Cluster{}}}
	_ = json.Unmarshal([]byte(os.Getenv("KUBERNETES_EXEC_INFO")), &info)
	proxy, err := url.Parse(info.Spec.Cluster.ProxyURL)
	if err != nil || proxy.Host == "" {
		return errors.New("kubectl runs this for a ccloud context; sign in with ccloud clusters auth")
	}
	if err := c.tunnel(proxy.Host); err != nil {
		return err
	}
	token := exec.CommandContext(c.Context(), "kubectl-oidc_login", c.oidcArgs()...)
	token.Stdin, token.Stdout, token.Stderr = c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()
	return token.Run()
}

func (c command) oidcArgs() []string {
	args := []string{"get-token",
		"--oidc-issuer-url=" + c.home.Issuer,
		"--oidc-client-id=" + c.home.ClientID,
		"--oidc-client-secret=" + c.home.ClientSecret}
	for _, scope := range c.home.Scopes {
		args = append(args, "--oidc-extra-scope="+scope)
	}
	return args
}

func (c command) tunnel(address string) error {
	if listening(address) {
		return nil
	}
	cloudflared := exec.Command("cloudflared", "access", "tcp", "--hostname", c.region.Host, "--url", address)
	cloudflared.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cloudflared.Start(); err != nil {
		return fmt.Errorf("starting cloudflared for %s: %w", c.region.Host, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cloudflared.Wait() }()

	ctx, stop := context.WithTimeout(c.Context(), 20*time.Second)
	defer stop()
	for {
		select {
		case err := <-exited:
			return fmt.Errorf("cloudflared exited before %s was reachable at %s: %v", c.region.Host, address, err)
		case <-ctx.Done():
			return fmt.Errorf("cloudflared did not open %s for %s in time", address, c.region.Host)
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
