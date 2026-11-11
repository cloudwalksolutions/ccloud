package cmd_test

import (
	"bytes"
	"net"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud clusters credentials", func() {
	var (
		bin     string
		calls   string
		out     *bytes.Buffer
		address string
	)

	tool := func(name, script string) {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
	}

	freeAddress := func() string {
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = probe.Close() }()
		return probe.Addr().String()
	}

	BeforeEach(func() {
		bin = GinkgoT().TempDir()
		calls = filepath.Join(GinkgoT().TempDir(), "calls")
		GinkgoT().Setenv("PATH", bin)
		out = &bytes.Buffer{}
		address = freeAddress()
		GinkgoT().Setenv("KUBERNETES_EXEC_INFO", `{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","spec":{"interactive":true,"cluster":{"server":"https://api.example.test","proxy-url":"socks5://`+address+`"}}}`)
	})

	It("hands kubectl your token from the cloud's issuer over the tunnel already open, without opening another", func() {
		tunnel, err := net.Listen("tcp", address)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(tunnel.Close)
		tool("cloudflared", `echo "cloudflared $*" >> `+calls)
		tool("kubectl-oidc_login", `echo "oidc-login $*" >> `+calls+`; echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)

		Expect(cmd.Run([]string{"clusters", "credentials", "--region", "north1"}, out, out, example, "dev")).To(Succeed())

		Expect(out.String()).To(ContainSubstring(`"token":"id-token"`))
		recorded, _ := os.ReadFile(calls)
		Expect(string(recorded)).NotTo(ContainSubstring("cloudflared"))
		Expect(string(recorded)).To(ContainSubstring("get-token --oidc-issuer-url=https://issuer.example.test --oidc-client-id=client.example.test --oidc-client-secret=public-desktop-secret --oidc-extra-scope=email"))
	})

	It("sends you to sign in when run by hand rather than by kubectl for a ccloud context", func() {
		GinkgoT().Setenv("KUBERNETES_EXEC_INFO", "")

		err := cmd.Run([]string{"clusters", "credentials", "--region", "north1"}, out, out, example, "dev")

		Expect(err).To(MatchError("kubectl runs this for a ccloud context; sign in with ccloud clusters auth"))
	})

	It("opens the region's tunnel when none is listening, and hands kubectl your token once it is up", func() {
		tool("cloudflared", `echo "cloudflared $*" >> `+calls+`; exec /bin/sleep 0.5`)
		tool("kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
		go func() {
			defer GinkgoRecover()
			Eventually(calls).Should(BeAnExistingFile())
			tunnel, err := net.Listen("tcp", address)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(tunnel.Close)
		}()

		Expect(cmd.Run([]string{"clusters", "credentials", "--region", "north1"}, out, out, example, "dev")).To(Succeed())

		Expect(out.String()).To(ContainSubstring(`"token":"id-token"`))
	})

	It("names cloudflared when it is not installed", func() {
		err := cmd.Run([]string{"clusters", "credentials", "--region", "north1"}, out, out, example, "dev")

		Expect(err).To(MatchError(HavePrefix("starting cloudflared for k8s.north1.example.test")))
	})

	It("opens the region's tunnel at the kubeconfig's proxy when none is listening, and says so when cloudflared cannot", func() {
		tool("cloudflared", `echo "cloudflared $*" >> `+calls+`; exit 1`)

		err := cmd.Run([]string{"clusters", "credentials", "--region", "north1"}, out, out, example, "dev")

		Expect(err).To(MatchError(ContainSubstring("cloudflared")))
		Expect(os.ReadFile(calls)).To(ContainSubstring("access tcp --hostname k8s.north1.example.test --url " + address))
	})
})
