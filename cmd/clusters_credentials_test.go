package cmd_test

import (
	"bytes"
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

	It("opens the region's tunnel when none is listening, and says so when cloudflared cannot", func() {
		tool("cloudflared", `echo "cloudflared $*" >> `+calls+`; exit 1`)

		err := cmd.Run([]string{"clusters", "credentials", "us-east1"}, out, out)

		Expect(err).To(MatchError(ContainSubstring("cloudflared")))
		Expect(os.ReadFile(calls)).To(ContainSubstring("access tcp --hostname k8s.us-east1.cloudwalksolutions.net --url 127.0.0.1:16443"))
	})
})
