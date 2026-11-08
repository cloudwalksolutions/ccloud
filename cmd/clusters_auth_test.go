package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/clientcmd"

	"ccloud/cmd"
	"ccloud/internal/google"
)

var _ = Describe("ccloud clusters auth", func() {
	var (
		kubeconfig string
		bin        string
		out        *bytes.Buffer
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out)
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
		google.ClientID, google.ClientSecret = "client.apps.googleusercontent.com", "public-desktop-secret"
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

		err := run("clusters", "auth", "us-east1")

		Expect(err).To(MatchError(ContainSubstring("Unauthorized")))
		Expect(os.ReadFile(kubeconfig)).To(Equal(existing))
	})

	It("adds the region beside your other clusters, without switching you to it, however often it runs", func() {
		Expect(os.MkdirAll(filepath.Dir(kubeconfig), 0o700)).To(Succeed())
		Expect(os.WriteFile(kubeconfig, []byte("apiVersion: v1\nkind: Config\ncurrent-context: work\ncontexts:\n- name: work\n  context: {cluster: work, user: work}\nclusters:\n- name: work\n  cluster: {server: https://work.example}\nusers:\n- name: work\n  user: {token: t}\n"), 0o600)).To(Succeed())
		tool("kubectl", `echo "Username  walker@cloudwalksolutions.io"`)

		Expect(run("clusters", "auth", "us-east1")).To(Succeed())
		Expect(run("clusters", "auth", "us-east1")).To(Succeed())

		cfg, err := clientcmd.LoadFromFile(kubeconfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.CurrentContext).To(Equal("work"))
		Expect(cfg.Contexts).To(HaveKey("work"))
		Expect(cfg.Contexts).To(HaveLen(2))
		cluster := cfg.Clusters[cfg.Contexts["ccloud-us-east1"].Cluster]
		Expect(cluster.Server).To(Equal("https://kubernetes.default.svc.cluster.local"))
		Expect(cluster.ProxyURL).To(HavePrefix("socks5://127.0.0.1:"))
		Expect(string(cluster.CertificateAuthorityData)).To(ContainSubstring("BEGIN CERTIFICATE"))
		Expect(cfg.AuthInfos[cfg.Contexts["ccloud-us-east1"].AuthInfo].Exec.Args).To(Equal([]string{"clusters", "credentials", "us-east1"}))
		Expect(out.String()).To(ContainSubstring("walker@cloudwalksolutions.io"))
	})

	It("names the tool it needs that this machine lacks", func() {
		Expect(os.Remove(filepath.Join(bin, "cloudflared"))).To(Succeed())

		err := run("clusters", "auth", "us-east1")

		Expect(err).To(MatchError(ContainSubstring("cloudflared")))
		Expect(kubeconfig).NotTo(BeAnExistingFile())
	})

	It("refuses a region the datacenter does not have, naming the ones it does", func() {
		err := run("clusters", "auth", "us-west9")

		Expect(err).To(MatchError(ContainSubstring("us-west9")))
		Expect(err).To(MatchError(And(ContainSubstring("us-central1"), ContainSubstring("us-east1"))))
		Expect(kubeconfig).NotTo(BeAnExistingFile())
	})

	It("refuses to sign in from a build that carries no Google client", func() {
		google.ClientID = ""

		err := run("clusters", "auth", "us-east1")

		Expect(err).To(MatchError(ContainSubstring("built without a Google client")))
		Expect(kubeconfig).NotTo(BeAnExistingFile())
	})
})
