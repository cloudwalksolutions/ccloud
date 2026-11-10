package cmd_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud --help", func() {
	help := func(args ...string) string {
		out := &bytes.Buffer{}
		Expect(cmd.Run(append(args, "--help"), out, out, example, "dev")).To(Succeed())
		return out.String()
	}

	It("speaks of the cloud it was built for, with that cloud's regions in its examples", func() {
		Expect(help()).To(ContainSubstring("example"))
		for _, page := range []string{help(), help("clusters", "auth"), help("clusters", "set-context")} {
			Expect(page).NotTo(ContainSubstring("datacenter"))
			Expect(page).NotTo(ContainSubstring("us-central1"))
		}
		Expect(help("clusters", "auth")).To(ContainSubstring("ccloud clusters auth --region north1"))
		Expect(help("clusters", "set-context")).To(ContainSubstring("ccloud clusters set-context --region north1"))
	})

	It("tells you which release you are running", func() {
		out := &bytes.Buffer{}

		Expect(cmd.Run([]string{"--version"}, out, out, example, "v0.1.7 (commit abc1234, built 2026-11-09)")).To(Succeed())

		Expect(out.String()).To(Equal("ccloud version v0.1.7 (commit abc1234, built 2026-11-09)\n"))
	})
})
