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

var _ = Describe("ccloud clusters set-context", func() {
	var (
		kubeconfig string
		out        *bytes.Buffer
	)

	write := func(content string) []byte {
		Expect(os.MkdirAll(filepath.Dir(kubeconfig), 0o700)).To(Succeed())
		Expect(os.WriteFile(kubeconfig, []byte(content), 0o600)).To(Succeed())
		return []byte(content)
	}

	BeforeEach(func() {
		kubeconfig = filepath.Join(GinkgoT().TempDir(), ".kube", "config")
		GinkgoT().Setenv("KUBECONFIG", kubeconfig)
		out = &bytes.Buffer{}
	})

	It("sends you to sign in when the region is not in your kubeconfig yet", func() {
		existing := write("apiVersion: v1\nkind: Config\ncurrent-context: work\ncontexts:\n- name: work\n  context: {cluster: work, user: work}\n")

		err := cmd.Run([]string{"clusters", "set-context", "--region", "north1"}, out, out, example)

		Expect(err).To(MatchError(ContainSubstring("ccloud clusters auth --region north1")))
		Expect(os.ReadFile(kubeconfig)).To(Equal(existing))
	})

	It("switches you to a region you signed in to, keeping the namespace you last chose there", func() {
		write("apiVersion: v1\nkind: Config\ncurrent-context: work\ncontexts:\n- name: work\n  context: {cluster: work, user: work}\n- name: ccloud-north1\n  context: {cluster: ccloud-north1, user: ccloud-north1, namespace: stage-shop}\n")

		Expect(cmd.Run([]string{"clusters", "set-context", "--region", "north1"}, out, out, example)).To(Succeed())

		cfg, err := clientcmd.LoadFromFile(kubeconfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.CurrentContext).To(Equal("ccloud-north1"))
		Expect(cfg.Contexts["ccloud-north1"].Namespace).To(Equal("stage-shop"))
		Expect(out.String()).To(ContainSubstring(`Switched to context "ccloud-north1".`))
	})

	It("points the region's context at the namespace you name", func() {
		write("apiVersion: v1\nkind: Config\ncurrent-context: work\ncontexts:\n- name: work\n  context: {cluster: work, user: work, namespace: default}\n- name: ccloud-north1\n  context: {cluster: ccloud-north1, user: ccloud-north1, namespace: stage-shop}\n")

		Expect(cmd.Run([]string{"clusters", "set-context", "--region", "north1", "--namespace", "ci-shop"}, out, out, example)).To(Succeed())

		cfg, err := clientcmd.LoadFromFile(kubeconfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.CurrentContext).To(Equal("ccloud-north1"))
		Expect(cfg.Contexts["ccloud-north1"].Namespace).To(Equal("ci-shop"))
		Expect(cfg.Contexts["work"].Namespace).To(Equal("default"))
	})
})
