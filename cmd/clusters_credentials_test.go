package cmd_test

import (
	"bytes"
	"net"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"ccloud/cmd"
	"ccloud/internal/google"
)

var _ = Describe("ccloud clusters credentials", func() {
	var (
		bin   string
		calls string
		out   *bytes.Buffer
	)

	tool := func(name, script string) {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
	}

	BeforeEach(func() {
		bin = GinkgoT().TempDir()
		calls = filepath.Join(GinkgoT().TempDir(), "calls")
		GinkgoT().Setenv("PATH", bin)
		google.ClientID, google.ClientSecret = "client.apps.googleusercontent.com", "public-desktop-secret"
		out = &bytes.Buffer{}
	})

	It("hands kubectl your Google token over the tunnel already open, without opening another", func() {
		tunnel, err := net.Listen("tcp", "127.0.0.1:16443")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(tunnel.Close)
		tool("cloudflared", `echo "cloudflared $*" >> `+calls)
		tool("kubectl-oidc_login", `echo "oidc-login $*" >> `+calls+`; echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)

		Expect(cmd.Run([]string{"clusters", "credentials", "us-east1"}, out, out)).To(Succeed())

		Expect(out.String()).To(ContainSubstring(`"token":"id-token"`))
		recorded, _ := os.ReadFile(calls)
		Expect(string(recorded)).NotTo(ContainSubstring("cloudflared"))
		Expect(string(recorded)).To(ContainSubstring("get-token --oidc-issuer-url=https://accounts.google.com --oidc-client-id=client.apps.googleusercontent.com --oidc-client-secret=public-desktop-secret"))
	})

	It("opens the region's tunnel when none is listening, and says so when cloudflared cannot", func() {
		tool("cloudflared", `echo "cloudflared $*" >> `+calls+`; exit 1`)

		err := cmd.Run([]string{"clusters", "credentials", "us-east1"}, out, out)

		Expect(err).To(MatchError(ContainSubstring("cloudflared")))
		Expect(os.ReadFile(calls)).To(ContainSubstring("access tcp --hostname k8s.us-east1.cloudwalksolutions.net --url 127.0.0.1:16443"))
	})
})
