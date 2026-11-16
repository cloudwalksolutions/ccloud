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

var _ = Describe("ccloud app", func() {
	var (
		out          *bytes.Buffer
		north, south *fakeGateway
		home         cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, home, "dev")
	}

	serve := func(g *fakeGateway) string {
		server := httptest.NewServer(http.HandlerFunc(g.serve))
		DeferCleanup(server.Close)
		return server.URL
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		north = &fakeGateway{hosts: []string{"ci", "boat-105"}}
		south = &fakeGateway{hosts: []string{"prod"}}
		home = example
		home.Regions = append([]cloud.Region{}, example.Regions...)
		home.Regions[0].Gateway = serve(north)
		home.Regions[1].Gateway = serve(south)

		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		for name, script := range map[string]string{
			"cloudflared":        `[ "$1 $2" = "access token" ] && echo access-jwt`,
			"kubectl-oidc_login": `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`,
		} {
			Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
		}
	})

	It("asks which app outside an app repo", func() {
		Expect(run("app")).To(MatchError("which app? ccloud app --app <app>"))
	})

	It("names the region it could not ask for the app's environments", func() {
		south.refuse = http.StatusBadGateway

		Expect(run("app", "-a", "shop")).To(MatchError(HavePrefix("asking south1 where shop runs")))
	})

	It("says when no region runs the app's CI to summarize", func() {
		north.hosts = []string{"stage"}

		Expect(run("app", "-a", "shop", "--summary")).To(MatchError(HavePrefix("no region hosts shop's ci")))
	})

	It("names the region and what its gateway said when it cannot summarize", func() {
		north.refuse = http.StatusBadGateway

		Expect(run("app", "-a", "shop", "--summary", "-r", "north1")).To(MatchError(HavePrefix("asking north1 for shop's summary: the gateway answered 502")))
	})

	It("says when nothing is open, and marks a draft as a draft", func() {
		north.summary = `{"pulls":[],"flow":{},"delivery":{},"ci":[]}`

		Expect(run("app", "-a", "shop", "--summary")).To(Succeed())

		Expect(out.String()).To(HavePrefix("No open pull requests.\n"))

		out.Reset()
		north.summary = `{"pulls":[{"number":7,"title":"Spike","author":"dev","openedHoursAgo":1,"draft":true,"checks":[]}],"flow":{},"delivery":{},"ci":[]}`

		Expect(run("app", "-a", "shop", "--summary")).To(Succeed())

		Expect(strings.Split(out.String(), "\n")[1]).To(MatchRegexp(`^#7\s+Spike\s+dev\s+1h ago\s+draft$`))
	})

	It("lists every environment of the app, the region it runs in and where it serves", func() {
		Expect(run("app", "-a", "shop")).To(Succeed())

		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		Expect(lines[0]).To(MatchRegexp(`^ENV\s+REGION\s+URL$`))
		Expect(lines[1]).To(MatchRegexp(`^boat-105\s+north1\s+https://boat-105\.shop\.test$`))
		Expect(lines[2]).To(MatchRegexp(`^ci\s+north1$`))
		Expect(lines[3]).To(MatchRegexp(`^prod\s+south1\s+https://prod\.shop\.test$`))
	})

	It("summarizes open pull requests with their checks, then how the last weeks went, from the region running CI", func() {
		north.summary = `{"pulls":[{"number":105,"title":"Search by lake","author":"dev","branch":"feat/BOAT-105/search",
			"openedHoursAgo":72,"draft":false,"reviewed":true,"environment":"https://boat-105.shop.test",
			"checks":[{"flow":"api-tests","status":"Succeeded","run":"api-tests-x1"},{"flow":"web-tests","status":"Running","run":"web-tests-x2"}]}],
			"flow":{"merged":2,"openToMergeHours":12,"firstReviewHours":1.5},
			"delivery":{"deploys":1,"mergeToProdHours":1},
			"ci":[{"flow":"api-tests","runs":3,"medianSeconds":700,"firstTryPassRate":0.5,"reruns":1}]}`

		Expect(run("app", "-a", "shop", "--summary", "--weeks", "1")).To(Succeed())

		Expect(south.asked).To(BeEmpty())
		Expect(north.asked[0].URL.Query().Get("weeks")).To(Equal("1"))
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		Expect(lines[0]).To(MatchRegexp(`^PR\s+TITLE\s+AUTHOR\s+OPENED\s+REVIEW\s+CHECKS\s+ENVIRONMENT$`))
		Expect(lines[1]).To(MatchRegexp(`^#105\s+Search by lake\s+dev\s+3d ago\s+reviewed\s+✓ api-tests  \* web-tests\s+https://boat-105\.shop\.test$`))
		Expect(lines[3:]).To(Equal([]string{
			"Last 1 week:",
			"  pull requests  2 merged; 12h from open to merge; 1.5h to a first review",
			"  delivery       1 prod deploy; 1h from merge to prod",
			"  api-tests      3 runs; median 11m40s; 50% pass first time; 1 rerun",
		}))
	})
})
