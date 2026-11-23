package cmd_test

import (
	"cmp"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

type fakeGateway struct {
	asked     []*http.Request
	answers   []string
	refuse    int
	after     func(asked int)
	hosts     []string
	resources string
	network   string
	summary   string
	streams   []reply
	slow      time.Duration
	asking    func()
}

type reply struct {
	status int
	body   string
}

func servedBy(gateways ...*fakeGateway) cloud.Cloud {
	home := example
	home.Regions = append([]cloud.Region{}, example.Regions...)
	for i, g := range gateways {
		server := httptest.NewServer(http.HandlerFunc(g.serve))
		DeferCleanup(server.Close)
		home.Regions[i].Gateway = server.URL
	}
	return home
}

func signInTools() string {
	bin := GinkgoT().TempDir()
	GinkgoT().Setenv("PATH", bin)
	tool(bin, "cloudflared", `[ "$1 $2" = "access token" ] && echo access-jwt`)
	tool(bin, "kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
	return bin
}

func tool(bin, name, script string) {
	GinkgoHelper()
	Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
}

func (g *fakeGateway) serve(w http.ResponseWriter, r *http.Request) {
	if g.refuse == 0 && g.answer(w, r) {
		return
	}
	g.asked = append(g.asked, r)
	if g.after != nil {
		defer g.after(len(g.asked))
	}
	if g.refuse != 0 {
		http.Error(w, "dev@example.test may not read logs in prod-acme", g.refuse)
		return
	}
	if r.URL.Path == "/v1/logs/stream" {
		g.stream(w, r)
		return
	}
	answer := `{"entries":[]}`
	if len(g.answers) > 0 {
		answer, g.answers = g.answers[0], g.answers[1:]
	}
	_, _ = w.Write([]byte(answer))
}

func (g *fakeGateway) answer(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	switch {
	case path == "/v1/logs/containers":
		g.asked = append(g.asked, r)
		_, _ = w.Write([]byte(`{"containers":["api","nginx"]}`))
	case strings.HasSuffix(path, "/resources"):
		g.asked = append(g.asked, r)
		_, _ = w.Write([]byte(cmp.Or(g.resources, `{"environments":[]}`)))
	case strings.HasSuffix(path, "/summary"):
		g.asked = append(g.asked, r)
		time.Sleep(g.slow)
		_, _ = w.Write([]byte(g.summary))
	case strings.HasSuffix(path, "/network"):
		g.asked = append(g.asked, r)
		_, _ = w.Write([]byte(g.network))
	case strings.HasPrefix(path, "/v1/apps/"):
		g.environments(w, strings.TrimPrefix(path, "/v1/apps/"))
	default:
		return false
	}
	return true
}

func (g *fakeGateway) environments(w http.ResponseWriter, app string) {
	time.Sleep(g.slow)
	if g.asking != nil {
		g.asking()
	}
	environments := []string{}
	for _, env := range g.hosts {
		url := ""
		if env != "ci" {
			url = fmt.Sprintf(`,"url":"https://%s.%s.test"`, env, app)
		}
		environments = append(environments, fmt.Sprintf(`{"name":%q,"namespace":"%s-%s"%s}`, env, env, app, url))
	}
	_, _ = fmt.Fprintf(w, `{"environments":[%s]}`, strings.Join(environments, ","))
}

func (g *fakeGateway) stream(w http.ResponseWriter, r *http.Request) {
	if len(g.streams) == 0 {
		if g.after != nil {
			g.after(len(g.asked))
		}
		<-r.Context().Done()
		return
	}
	var next reply
	next, g.streams = g.streams[0], g.streams[1:]
	if next.status != 0 {
		http.Error(w, next.body, next.status)
		return
	}
	_, _ = w.Write([]byte(next.body))
}
