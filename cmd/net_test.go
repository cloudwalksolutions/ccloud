package cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

var _ = Describe("ccloud net", func() {
	var (
		out, errs *bytes.Buffer
		gateway   *fakeGateway
		home      cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, errs, home, "dev")
	}

	BeforeEach(func() {
		out, errs = &bytes.Buffer{}, &bytes.Buffer{}
		gateway = &fakeGateway{hosts: []string{"prod"}}
		server := httptest.NewServer(http.HandlerFunc(gateway.serve))
		DeferCleanup(server.Close)
		home = example
		home.Regions = append([]cloud.Region{}, example.Regions...)
		home.Regions[0].Gateway = server.URL

		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		for name, script := range map[string]string{
			"cloudflared":        `[ "$1 $2" = "access token" ] && echo access-jwt`,
			"kubectl-oidc_login": `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`,
		} {
			Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
		}
	})

	It("asks which environment", func() {
		err := run("net", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("which environment? ccloud net --env <env>")))
	})

	It("names the region and what its gateway said when it cannot answer", func() {
		gateway.refuse = http.StatusBadGateway

		err := run("net", "-a", "acme", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("asking north1 about acme's prod traffic: the gateway answered 502")))
		Expect(errs.String()).To(Equal("Asking north1 about acme's prod traffic…\n"))
	})

	It("says when nothing went out and nothing was dropped", func() {
		gateway.network = `{"ingress":{"requestsPerSecond":0,"serverErrors":0,"p95Seconds":0},"egress":[],"drops":[]}`

		Expect(run("net", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(Equal("Ingress over the last 1h: no requests.\n" +
			"No connections to the internet in the last 1h.\n" +
			"No connections dropped in the last 1h.\n"))
	})

	It("shows ingress, every destination with what went unanswered or was reset, and the drops, over the time asked", func() {
		gateway.network = `{"ingress":{"requestsPerSecond":3.2,"serverErrors":0.005,"p95Seconds":0.21},
			"egress":[{"destination":"hydro.nationalmap.gov","port":443,"connections":412,"unanswered":398,"reset":3},
			          {"destination":"140.82.112.5","port":443,"connections":20,"unanswered":0,"reset":0}],
			"drops":[{"destination":"10.0.4.9","port":5432,"reason":"POLICY_DENIED","count":12}]}`

		Expect(run("net", "-a", "acme", "-e", "prod", "-r", "north1", "--since", "6h")).To(Succeed())

		Expect(gateway.asked[0].URL.Query().Get("since")).To(Equal("6h0m0s"))
		lines := strings.Split(out.String(), "\n")
		Expect(lines[0]).To(Equal("Ingress over the last 6h: 3.20 requests/s, 0.5% 5xx, p95 210ms."))
		Expect(lines[2]).To(MatchRegexp(`^DESTINATION\s+PORT\s+CONNECTIONS\s+UNANSWERED\s+RESET$`))
		Expect(lines[3]).To(MatchRegexp(`^hydro\.nationalmap\.gov\s+443\s+412\s+398\s+3\s+! 97% unanswered, 3 reset$`))
		Expect(lines[4]).To(MatchRegexp(`^140\.82\.112\.5\s+443\s+20\s+0\s+0$`))
		Expect(lines[6]).To(MatchRegexp(`^DROPPED TO\s+PORT\s+REASON\s+COUNT$`))
		Expect(lines[7]).To(MatchRegexp(`^10\.0\.4\.9\s+5432\s+POLICY_DENIED\s+12$`))
	})
})
