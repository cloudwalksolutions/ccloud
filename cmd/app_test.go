package cmd_test

import (
	"bytes"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

var _ = Describe("ccloud app", func() {
	var (
		out, errs    *bytes.Buffer
		north, south *fakeGateway
		home         cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, errs, home, "dev")
	}

	BeforeEach(func() {
		out, errs = &bytes.Buffer{}, &bytes.Buffer{}
		north = &fakeGateway{hosts: []string{"ci", "boat-105"}}
		south = &fakeGateway{hosts: []string{"prod"}}
		home = servedBy(north, south)
		signInTools()
	})

	It("says so when no region runs the app at all", func() {
		north.hosts, south.hosts = nil, nil

		Expect(run("app", "-a", "ghost")).To(MatchError("no region runs ghost, or you may not read any of its environments"))
	})

	It("asks which app outside an app repo", func() {
		Expect(run("app")).To(MatchError("which app? ccloud app --app <app>"))
	})

	It("names the region it could not ask for the app's environments", func() {
		south.refuse = http.StatusBadGateway

		Expect(run("app", "-a", "acme")).To(MatchError(HavePrefix("asking south1 where acme runs")))
	})

	It("says when no region runs the app's CI to summarize", func() {
		north.hosts = []string{"stage"}

		Expect(run("app", "-a", "acme", "--summary")).To(MatchError(HavePrefix("no region hosts acme's ci")))
	})

	It("names the region and what its gateway said when it cannot summarize", func() {
		north.refuse = http.StatusBadGateway

		Expect(run("app", "-a", "acme", "--summary", "-r", "north1")).To(MatchError(HavePrefix("asking north1 for acme's summary: the gateway answered 502")))
		Expect(errs.String()).To(Equal("Asking north1 for acme's summary…\n"))
	})

	It("waits longer for a summary than for other answers, since the gateway builds it from many GitHub calls", func() {
		home.GatewayTimeout = 100 * time.Millisecond
		north.slow = 250 * time.Millisecond
		north.summary = `{"pulls":[],"flow":{},"delivery":{},"ci":[]}`

		Expect(run("app", "-a", "acme", "--summary", "-r", "north1")).To(Succeed())
	})

	It("says when nothing is open, and marks a draft as a draft", func() {
		north.summary = `{"pulls":[],"flow":{},"delivery":{},"ci":[]}`

		Expect(run("app", "-a", "acme", "--summary")).To(Succeed())

		Expect(out.String()).To(HavePrefix("No open pull requests.\n"))

		out.Reset()
		north.summary = `{"pulls":[{"number":7,"title":"Spike","author":"dev","openedHoursAgo":1,"draft":true,"checks":[]}],"flow":{},"delivery":{},"ci":[]}`

		Expect(run("app", "-a", "acme", "--summary")).To(Succeed())

		Expect(strings.Split(out.String(), "\n")[1]).To(MatchRegexp(`^#7\s+Spike\s+dev\s+1h ago\s+draft$`))
	})

	It("lists every environment of the app, the region it runs in and where it serves", func() {
		Expect(run("app", "-a", "acme")).To(Succeed())

		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		Expect(lines[0]).To(MatchRegexp(`^ENV\s+REGION\s+URL$`))
		Expect(lines[1]).To(MatchRegexp(`^boat-105\s+north1\s+https://boat-105\.acme\.test$`))
		Expect(lines[2]).To(MatchRegexp(`^ci\s+north1$`))
		Expect(lines[3]).To(MatchRegexp(`^prod\s+south1\s+https://prod\.acme\.test$`))
	})

	It("summarizes open pull requests with their checks, then how the last weeks went, from the region running CI", func() {
		north.summary = `{"pulls":[{"number":105,"title":"Search by lake","author":"dev","branch":"feat/BOAT-105/search",
			"openedHoursAgo":72,"draft":false,"reviewed":true,"environment":"https://boat-105.acme.test",
			"checks":[{"flow":"api-tests","status":"Succeeded","run":"api-tests-x1"},{"flow":"web-tests","status":"Running","run":"web-tests-x2"}]}],
			"flow":{"merged":2,"openToMergeHours":12,"firstReviewHours":1.5},
			"delivery":{"deploys":1,"mergeToProdHours":1},
			"ci":[{"flow":"api-tests","runs":3,"medianSeconds":699.5519645,"firstTryPassRate":0.5,"reruns":1}]}`

		Expect(run("app", "-a", "acme", "--summary", "--weeks", "1")).To(Succeed())

		Expect(south.asked).To(BeEmpty())
		Expect(north.asked[0].URL.Query().Get("weeks")).To(Equal("1"))
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		Expect(lines[0]).To(MatchRegexp(`^PR\s+TITLE\s+AUTHOR\s+OPENED\s+REVIEW\s+CHECKS\s+ENVIRONMENT$`))
		Expect(lines[1]).To(MatchRegexp(`^#105\s+Search by lake\s+dev\s+3d ago\s+reviewed\s+✓ api-tests  \* web-tests\s+https://boat-105\.acme\.test$`))
		Expect(lines[3:]).To(Equal([]string{
			"Last 1 week:",
			"  pull requests  2 merged; 12h from open to merge; 1.5h to a first review",
			"  delivery       1 prod deploy; 1h from merge to prod",
			"  api-tests      3 runs; median 11m40s; 50% pass first time; 1 rerun",
		}))
	})

	It("lines up pull requests with different numbers of colored checks on a terminal", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		north.summary = `{"pulls":[
			{"number":105,"title":"Search","author":"dev","openedHoursAgo":1,"environment":"https://boat-105.acme.test",
			 "checks":[{"flow":"api","status":"Succeeded"},{"flow":"web","status":"Failed"}]},
			{"number":106,"title":"Map","author":"dev","openedHoursAgo":1,"environment":"https://boat-106.acme.test",
			 "checks":[{"flow":"api-tests","status":"Running"}]}],
			"flow":{},"delivery":{},"ci":[]}`

		Expect(run("app", "-a", "acme", "--summary")).To(Succeed())

		lines := strings.Split(plain(out.String()), "\n")
		Expect(column(lines[1], "https://")).To(Equal(column(lines[0], "ENVIRONMENT")))
		Expect(column(lines[2], "https://")).To(Equal(column(lines[0], "ENVIRONMENT")))
	})
})
