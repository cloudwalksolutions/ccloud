package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	clientauthentication "k8s.io/client-go/pkg/apis/clientauthentication/v1"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

type session struct {
	gateways sync.Map
	prompts  sync.Mutex
	id       string
}

type sessionKey struct{}

func withSession(ctx context.Context) context.Context {
	return context.WithValue(ctx, sessionKey{}, &session{})
}

func sessionOf(c *cobra.Command) *session {
	return c.Context().Value(sessionKey{}).(*session)
}

func (s *session) gateway(c *cobra.Command, home cloud.Cloud, region cloud.Region) (gateway.Gateway, error) {
	if signedIn, ok := s.gateways.Load(region.Name); ok {
		return signedIn.(gateway.Gateway), nil
	}
	address, err := url.Parse(region.Gateway)
	if err != nil {
		return gateway.Gateway{}, fmt.Errorf("%s's gateway %q is not a URL: %w", region.Name, region.Gateway, err)
	}
	access, err := s.accessToken(c, region)
	if err != nil {
		return gateway.Gateway{}, err
	}
	id, err := s.idToken(c, home)
	if err != nil {
		return gateway.Gateway{}, err
	}
	signedIn := gateway.Gateway{URL: address, AccessToken: access, IDToken: id, Timeout: home.GatewayTimeout}
	s.gateways.Store(region.Name, signedIn)
	return signedIn, nil
}

func (s *session) forget(region cloud.Region) {
	s.gateways.Delete(region.Name)
	s.prompts.Lock()
	defer s.prompts.Unlock()
	s.id = ""
}

func (s *session) idToken(c *cobra.Command, home cloud.Cloud) (string, error) {
	s.prompts.Lock()
	defer s.prompts.Unlock()
	if s.id != "" {
		return s.id, nil
	}
	oidc := exec.CommandContext(c.Context(), "kubectl-oidc_login", oidcArgs(home)...)
	oidc.Stderr = c.ErrOrStderr()
	credential, err := oidc.Output()
	var exchanged clientauthentication.ExecCredential
	if err != nil || json.Unmarshal(credential, &exchanged) != nil || exchanged.Status == nil {
		return "", fmt.Errorf("signing in to %s failed", home.Issuer)
	}
	s.id = exchanged.Status.Token
	return s.id, nil
}

func (s *session) accessToken(c *cobra.Command, region cloud.Region) (string, error) {
	token, err := exec.CommandContext(c.Context(), "cloudflared", "access", "token", "--app="+region.Gateway).Output()
	if err == nil {
		return strings.TrimSpace(string(token)), nil
	}
	s.prompts.Lock()
	defer s.prompts.Unlock()
	login := exec.CommandContext(c.Context(), "cloudflared", "access", "login", "--quiet", region.Gateway)
	login.Stdout, login.Stderr = c.ErrOrStderr(), c.ErrOrStderr()
	if err := login.Run(); err != nil {
		return "", fmt.Errorf("signing in to %s through Cloudflare Access: %w", region.Gateway, err)
	}
	token, err = exec.CommandContext(c.Context(), "cloudflared", "access", "token", "--app="+region.Gateway).Output()
	return strings.TrimSpace(string(token)), err
}
