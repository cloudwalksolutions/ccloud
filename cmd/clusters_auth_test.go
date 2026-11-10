package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud clusters auth", func() {
	var (
		kubeconfig string
		bin        string
		out        *bytes.Buffer
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, example)
	}

	tool := func(name, script string) {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
	}

	BeforeEach(func() {
		home := GinkgoT().TempDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
		bin = GinkgoT().TempDir()
		GinkgoT().Setenv("KUBECONFIG", kubeconfig)
		GinkgoT().Setenv("PATH", bin)
		out = &bytes.Buffer{}
		tool("cloudflared", "exit 0")
		tool("kubectl", "exit 0")
		tool("kubectl-oidc_login", "exit 0")
	})

	It("leaves your kubeconfig as it was when signing in fails, and says why", func() {
		existing := []byte("apiVersion: v1\nkind: Config\ncurrent-context: work\ncontexts:\n- name: work\n  context: {cluster: work, user: work}\nclusters:\n- name: work\n  cluster: {server: https://work.example}\nusers:\n- name: work\n  user: {token: t}\n")
		Expect(os.MkdirAll(filepath.Dir(kubeconfig), 0o700)).To(Succeed())
		Expect(os.WriteFile(kubeconfig, existing, 0o600)).To(Succeed())
		tool("kubectl", `echo "error: You must be logged in to the server (Unauthorized)" >&2; exit 1`)

		err := run("clusters", "auth", "--region", "north1")

		Expect(err).To(MatchError(ContainSubstring("Unauthorized")))
		Expect(os.ReadFile(kubeconfig)).To(Equal(existing))
	})

	It("adds the region beside your other clusters and switches you to it, however often it runs", func() {
		Expect(os.MkdirAll(filepath.Dir(kubeconfig), 0o700)).To(Succeed())
		Expect(os.WriteFile(kubeconfig, []byte("apiVersion: v1\nkind: Config\ncurrent-context: work\ncontexts:\n- name: work\n  context: {cluster: work, user: work}\nclusters:\n- name: work\n  cluster: {server: https://work.example}\nusers:\n- name: work\n  user: {token: t}\n"), 0o600)).To(Succeed())
		tool("kubectl", `echo "Username  dev@example.test"`)

		Expect(run("clusters", "auth", "--region", "north1")).To(Succeed())
		Expect(run("clusters", "auth", "--region", "north1")).To(Succeed())

		cfg, err := clientcmd.LoadFromFile(kubeconfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.CurrentContext).To(Equal("ccloud-north1"))
		Expect(out.String()).To(ContainSubstring(`Switched to context "ccloud-north1"`))
		Expect(cfg.Contexts).To(HaveKey("work"))
		Expect(cfg.Contexts).To(HaveLen(2))
		cluster := cfg.Clusters[cfg.Contexts["ccloud-north1"].Cluster]
		Expect(cluster.Server).To(Equal("https://api.example.test"))
		Expect(cluster.ProxyURL).To(Equal("socks5://127.0.0.1:17001"))
		Expect(cluster.CertificateAuthorityData).To(Equal(example.Regions[0].Authority))
		_, err = clientcmd.NewNonInteractiveClientConfig(*cfg, "ccloud-north1", &clientcmd.ConfigOverrides{}, nil).ClientConfig()
		Expect(err).NotTo(HaveOccurred(), "kubectl must accept the context as written")
		exec := cfg.AuthInfos[cfg.Contexts["ccloud-north1"].AuthInfo].Exec
		Expect(exec.Args).To(Equal([]string{"clusters", "credentials", "--region", "north1"}))
		Expect(exec.ProvideClusterInfo).To(BeTrue(), "credentials finds its tunnel through the proxy-url kubectl passes it")
		Expect(out.String()).To(ContainSubstring("dev@example.test"))
	})

	It("names the tool it needs that this machine lacks", func() {
		Expect(os.Remove(filepath.Join(bin, "cloudflared"))).To(Succeed())

		err := run("clusters", "auth", "--region", "north1")

		Expect(err).To(MatchError(ContainSubstring("cloudflared")))
		Expect(kubeconfig).NotTo(BeAnExistingFile())
	})

	It("refuses a region the cloud does not have, naming the ones it does", func() {
		err := run("clusters", "auth", "--region", "west9")

		Expect(err).To(MatchError(ContainSubstring("west9")))
		Expect(err).To(MatchError(And(ContainSubstring("north1"), ContainSubstring("south1"))))
		Expect(kubeconfig).NotTo(BeAnExistingFile())
	})

	It("asks for a region when given none, showing how to call it and naming the regions", func() {
		err := run("clusters", "auth")

		Expect(err).To(MatchError(ContainSubstring("ccloud clusters auth --region <region>")))
		Expect(err).To(MatchError(And(ContainSubstring("north1"), ContainSubstring("south1"))))
		Expect(kubeconfig).NotTo(BeAnExistingFile())
	})

	It("refuses to sign in from a build that carries no OIDC client for its cloud", func() {
		unbuilt := example
		unbuilt.ClientID = ""

		err := cmd.Run([]string{"clusters", "auth", "--region", "north1"}, out, out, unbuilt)

		Expect(err).To(MatchError("this ccloud was built without an OIDC client for the example cloud; install a release build"))
		Expect(kubeconfig).NotTo(BeAnExistingFile())
	})
})
