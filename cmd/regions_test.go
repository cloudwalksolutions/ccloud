package cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

var _ = Describe("finding the region", func() {
	var (
		out          *bytes.Buffer
		north, south *fakeGateway
		home         cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, home, "dev")
	}

	tool := func(bin, name, script string) {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
	}

	serve := func(g *fakeGateway) string {
		server := httptest.NewServer(http.HandlerFunc(g.serve))
		DeferCleanup(server.Close)
		return server.URL
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		north = &fakeGateway{hosts: []string{"ci"}}
		south = &fakeGateway{hosts: []string{"prod", "stage"}}
		home = example
		home.Regions = append([]cloud.Region{}, example.Regions...)
		home.Regions[0].Gateway = serve(north)
		home.Regions[1].Gateway = serve(south)

		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `[ "$1 $2" = "access token" ] && echo access-jwt`)
		tool(bin, "kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
	})

	It("names the environments the app has when no region hosts the one asked for", func() {
		err := run("logs", "-a", "acme", "-e", "feat-search")

		Expect(err).To(MatchError("no region hosts acme's feat-search; it has ci (north1), prod (south1), stage (south1)"))
	})

	It("reads an environment's logs from the region that hosts it", func() {
		Expect(run("logs", "-a", "acme", "-e", "prod")).To(Succeed())

		Expect(north.asked).To(BeEmpty())
		Expect(south.asked).To(HaveLen(1))
		Expect(south.asked[0].URL.Query().Get("namespace")).To(Equal("prod-acme"))
	})

	It("asks every region at once", func() {
		var asked sync.WaitGroup
		var together atomic.Int32
		asked.Add(2)
		meet := func() {
			asked.Done()
			both := make(chan struct{})
			go func() { asked.Wait(); close(both) }()
			select {
			case <-both:
				together.Add(1)
			case <-time.After(time.Second):
			}
		}
		north.asking, south.asking = meet, meet

		Expect(run("logs", "-a", "acme", "-e", "prod")).To(Succeed())

		Expect(together.Load()).To(BeEquivalentTo(2))
	})

	It("signs you in once for every region it asks, and once to the region it then reads", func() {
		calls := filepath.Join(GinkgoT().TempDir(), "calls")
		tool(os.Getenv("PATH"), "kubectl-oidc_login", `echo oidc >> `+calls+`
echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
		tool(os.Getenv("PATH"), "cloudflared", `echo "$2 $3" >> `+calls+`
[ "$1 $2" = "access token" ] && echo access-jwt`)

		Expect(run("logs", "-a", "acme", "-e", "prod")).To(Succeed())

		Expect(os.ReadFile(calls)).To(SatisfyAll(
			WithTransform(func(b []byte) int { return strings.Count(string(b), "oidc") }, Equal(1)),
			WithTransform(func(b []byte) int { return strings.Count(string(b), "token --app=") }, Equal(2)),
		))
	})

	It("opens one Cloudflare sign-in at a time when no region has signed you in", func() {
		signedIn := GinkgoT().TempDir()
		logins := filepath.Join(signedIn, "logins")
		tool(os.Getenv("PATH"), "cloudflared", `case "$1 $2" in
"access token") [ -e "`+signedIn+`/${3##*:}" ] && echo access-jwt ;;
"access login") echo start >> `+logins+`
  i=0; while [ $i -lt 20000 ]; do i=$((i+1)); done
  echo end >> `+logins+`
  : > "`+signedIn+`/${4##*:}" ;;
esac`)

		Expect(run("logs", "-a", "acme", "-e", "prod")).To(Succeed())

		Expect(os.ReadFile(logins)).To(BeEquivalentTo("start\nend\nstart\nend\n"))
	})

	It("spins while it asks the regions on a terminal, then ticks off where the app runs", func() {
		GinkgoT().Setenv("CCLOUD_FORCE_TTY", "1")
		south.slow = 250 * time.Millisecond

		Expect(run("logs", "-a", "acme", "-e", "prod")).To(Succeed())

		Expect(out.String()).To(HavePrefix("\r\x1b[K⠋ Finding where acme runs in north1 and south1…\r\x1b[K⠙ Finding where acme runs in north1 and south1…"))
		Expect(out.String()).To(ContainSubstring("\r\x1b[K\x1b[32m✓\x1b[0m acme runs in north1 and south1\n"))
	})

	It("clears the spinner off the line when a sign-in prompts you", func() {
		GinkgoT().Setenv("CCLOUD_FORCE_TTY", "1")
		tool(os.Getenv("PATH"), "kubectl-oidc_login", `echo 'Open http://localhost:8000 to sign in' >&2
echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)

		Expect(run("logs", "-a", "acme", "-e", "prod")).To(Succeed())

		Expect(out.String()).To(ContainSubstring("…\r\x1b[KOpen http://localhost:8000 to sign in\n"))
	})

	It("names the region it could not ask, and how to go without asking", func() {
		GinkgoT().Setenv("CCLOUD_FORCE_TTY", "1")
		north.refuse = http.StatusBadGateway

		err := run("logs", "-a", "acme", "-e", "prod")

		Expect(err).To(MatchError(And(HavePrefix("asking north1 where acme runs: the gateway answered 502"), HaveSuffix("; or name the region with --region"))))
		Expect(out.String()).To(HaveSuffix("\r\x1b[K\x1b[31mX\x1b[0m Finding where acme runs in north1 and south1\n"))
	})

	It("reads CI runs from the region that hosts the app's CI", func() {
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		newFakeCluster().signIn("north1")

		Expect(run("workflow", "list", "-a", "acme")).To(Succeed())

		Expect(out.String()).To(Equal("Finding where acme runs in north1 and south1…\nNo runs for acme in north1.\n"))
	})

	It("signs you in to every region that hosts the repo's app", func() {
		kubeconfig := filepath.Join(GinkgoT().TempDir(), "config")
		GinkgoT().Setenv("KUBECONFIG", kubeconfig)
		tool(os.Getenv("PATH"), "kubectl", "exit 0")
		repo := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(repo, ".ccloud"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(repo, ".ccloud", "app.yaml"), []byte("app: acme\n"), 0o600)).To(Succeed())
		was, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(repo)).To(Succeed())
		DeferCleanup(os.Chdir, was)

		Expect(run("clusters", "auth")).To(Succeed())

		cfg, err := clientcmd.LoadFromFile(kubeconfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Contexts).To(HaveKey("ccloud-north1"))
		Expect(cfg.Contexts).To(HaveKey("ccloud-south1"))
	})

	It("gives up on a region whose gateway does not answer in time", func() {
		home.GatewayTimeout = 100 * time.Millisecond
		north.slow = time.Second

		err := run("logs", "-a", "acme", "-e", "prod")

		Expect(err).To(MatchError("asking north1 where acme runs: the gateway did not answer within 100ms; or name the region with --region"))
	})

	It("asks you to choose when more than one region hosts the environment", func() {
		north.hosts = []string{"prod"}

		err := run("logs", "-a", "acme", "-e", "prod")

		Expect(err).To(MatchError("north1 and south1 both host acme's prod; name one with --region"))
	})
})
