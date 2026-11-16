package cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

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
		err := run("logs", "-a", "shop", "-e", "feat-search")

		Expect(err).To(MatchError("no region hosts shop's feat-search; it has ci (north1), prod (south1), stage (south1)"))
	})

	It("reads an environment's logs from the region that hosts it", func() {
		Expect(run("logs", "-a", "shop", "-e", "prod")).To(Succeed())

		Expect(north.asked).To(BeEmpty())
		Expect(south.asked).To(HaveLen(1))
		Expect(south.asked[0].URL.Query().Get("namespace")).To(Equal("prod-shop"))
	})

	It("names the region it could not ask, and how to go without asking", func() {
		north.refuse = http.StatusBadGateway

		err := run("logs", "-a", "shop", "-e", "prod")

		Expect(err).To(MatchError(And(HavePrefix("asking north1 where shop runs: the gateway answered 502"), HaveSuffix("; or name the region with --region"))))
	})

	It("reads CI runs from the region that hosts the app's CI", func() {
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		newFakeCluster().signIn("north1")

		Expect(run("workflow", "list", "-a", "shop")).To(Succeed())

		Expect(out.String()).To(Equal("No runs for shop in north1.\n"))
	})

	It("signs you in to every region that hosts the repo's app", func() {
		kubeconfig := filepath.Join(GinkgoT().TempDir(), "config")
		GinkgoT().Setenv("KUBECONFIG", kubeconfig)
		tool(os.Getenv("PATH"), "kubectl", "exit 0")
		repo := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(repo, ".ccloud"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(repo, ".ccloud", "app.yaml"), []byte("app: shop\n"), 0o600)).To(Succeed())
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

	It("asks you to choose when more than one region hosts the environment", func() {
		north.hosts = []string{"prod"}

		err := run("logs", "-a", "shop", "-e", "prod")

		Expect(err).To(MatchError("north1 and south1 both host shop's prod; name one with --region"))
	})
})
