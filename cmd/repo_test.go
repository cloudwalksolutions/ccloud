package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("an app repo", func() {
	var out *bytes.Buffer

	run := func(args ...string) error {
		return cmd.Run(args, out, out, example, "dev")
	}

	inRepo := func(appFile string) string {
		root := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(root, ".ccloud"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, ".ccloud", "app.yaml"), []byte(appFile), 0o600)).To(Succeed())
		was, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(root)).To(Succeed())
		DeferCleanup(os.Chdir, was)
		return root
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
	})

	It("names its app file when it cannot read it", func() {
		inRepo("app: [acme")

		err := run("workflow", "list", "-r", "north1")

		Expect(err).To(MatchError(ContainSubstring(filepath.Join(".ccloud", "app.yaml"))))
	})

	It("names the app for you anywhere inside it", func() {
		root := inRepo("app: acme\n")
		Expect(os.MkdirAll(filepath.Join(root, "src", "api"), 0o755)).To(Succeed())
		Expect(os.Chdir(filepath.Join(root, "src", "api"))).To(Succeed())
		newFakeCluster().signIn("north1")

		Expect(run("workflow", "list", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(Equal("No runs for acme in north1.\n"))
	})

	It("still lets you name another app", func() {
		inRepo("app: acme\n")
		newFakeCluster().signIn("north1")

		Expect(run("workflow", "list", "-a", "other", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(Equal("No runs for other in north1.\n"))
	})
})
