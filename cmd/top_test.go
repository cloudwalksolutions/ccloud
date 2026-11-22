package cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

const prod = `{"environments":[{"name":"prod","namespace":"prod-acme",
	"containers":[
		{"pod":"api-7f9c","container":"api","node":"node02",
		 "cpu":{"used":0.31,"request":0.25},"memory":{"used":412000000,"limit":1073741824},"restarts":2,"oomKills":2},
		{"pod":"redis-0","container":"metrics","node":"node01",
		 "cpu":{"used":0.05,"request":0.01,"limit":0.05},"memory":{"used":62914560,"limit":67108864},"throttled":0.31,"restarts":0,"oomKills":0}],
	"nodes":[
		{"name":"node01","cpuWait":0.2,"memoryStall":0.01,"conditions":[]},
		{"name":"node02","cpuWait":0.95,"memoryStall":0.01,"conditions":["DiskPressure"]}]}]}`

var _ = Describe("ccloud top", func() {
	var (
		out, errs    *bytes.Buffer
		north, south *fakeGateway
		home         cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, errs, home, "dev")
	}

	serve := func(g *fakeGateway) string {
		server := httptest.NewServer(http.HandlerFunc(g.serve))
		DeferCleanup(server.Close)
		return server.URL
	}

	BeforeEach(func() {
		out, errs = &bytes.Buffer{}, &bytes.Buffer{}
		north = &fakeGateway{}
		south = &fakeGateway{}
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

	It("says so when the app has no environment you may read", func() {
		Expect(run("top", "-a", "acme")).To(Succeed())

		Expect(out.String()).To(Equal("acme has no environment you may read.\n"))
	})

	It("asks which app outside an app repo", func() {
		err := run("top")

		Expect(err).To(MatchError("which app? ccloud top --app <app>"))
	})

	It("refuses a region the cloud does not have", func() {
		err := run("top", "-a", "acme", "-r", "west9")

		Expect(err).To(MatchError(HavePrefix(`no region "west9"`)))
	})

	It("flags a container with no memory limit that keeps restarting, leaves a healthy one alone, and names a node stalling on memory", func() {
		south.resources = `{"environments":[{"name":"stage","namespace":"stage-acme",
			"containers":[
				{"pod":"web-1","container":"web","node":"node03",
				 "cpu":{"used":0.1,"request":0.1,"limit":0.5},"memory":{"used":104857600},"restarts":3,"oomKills":0},
				{"pod":"web-2","container":"web","node":"node03",
				 "cpu":{"used":0.1,"request":0.1,"limit":0.5},"memory":{"used":104857600,"limit":209715200},"restarts":0,"oomKills":0}],
			"nodes":[{"name":"node03","cpuWait":0,"memoryStall":0.2,"conditions":[]}]}]}`

		Expect(run("top", "-a", "acme", "-e", "stage")).To(Succeed())

		lines := strings.Split(out.String(), "\n")
		Expect(lines[1]).To(MatchRegexp(`^web-1/web\s+0\.10/0\.10/0\.50\s+100Mi/none\s+3\s+node03\s+! no memory limit, restarted 3×$`))
		Expect(lines[2]).To(MatchRegexp(`^web-2/web\s+0\.10/0\.10/0\.50\s+100Mi/200Mi\s+0\s+node03$`))
		Expect(out.String()).To(HaveSuffix("\nnode03 is under pressure (the node, not your limits): memory stall 20%\n"))
	})

	It("names the region it could not ask", func() {
		north.refuse = http.StatusBadGateway

		err := run("top", "-a", "acme")

		Expect(err).To(MatchError(HavePrefix("asking north1 what acme uses: the gateway answered 502")))
		Expect(errs.String()).To(Equal("Asking north1 and south1 what acme uses…\n"))
	})

	It("sums every environment in every region, one row each, counting what is flagged", func() {
		north.resources = `{"environments":[{"name":"ci","namespace":"ci-acme",
			"containers":[{"pod":"runner","container":"main","node":"node05",
			 "cpu":{"used":0.1,"request":0.2},"memory":{"used":104857600,"limit":209715200},"restarts":0,"oomKills":0}],
			"nodes":[{"name":"node05","cpuWait":0,"memoryStall":0,"conditions":[]}]}]}`
		south.resources = prod

		Expect(run("top", "-a", "acme")).To(Succeed())

		lines := strings.Split(out.String(), "\n")
		Expect(lines[0]).To(MatchRegexp(`^ENV\s+REGION\s+PODS\s+CPU USED/REQ\s+MEMORY USED/LIMIT\s+FLAGS$`))
		Expect(lines[1]).To(MatchRegexp(`^ci\s+north1\s+1\s+0\.10/0\.20\s+100Mi/200Mi\s+! 1 container$`))
		Expect(lines[2]).To(MatchRegexp(`^prod\s+south1\s+2\s+0\.36/0\.26\s+453Mi/1\.1Gi\s+! 2 containers, 1 node$`))
		Expect(lines[3]).To(Equal("ccloud top -e <env> shows each container and why it is flagged."))
	})

	It("lines its columns up under a bold header on a terminal", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		south.resources = prod

		Expect(run("top", "-a", "acme")).To(Succeed())

		lines := strings.Split(regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out.String(), ""), "\n")
		Expect(strings.Index(lines[1], "south1")).To(Equal(strings.Index(lines[0], "REGION")))
	})

	It("shows one environment container by container, flagging its own limits apart from its nodes'", func() {
		south.resources = prod

		Expect(run("top", "-a", "acme", "-e", "prod")).To(Succeed())

		Expect(north.asked).To(HaveLen(1))
		Expect(south.asked[0].URL.Query().Get("env")).To(Equal("prod"))
		lines := strings.Split(out.String(), "\n")
		Expect(lines[0]).To(MatchRegexp(`^CONTAINER\s+CPU USED/REQ/LIMIT\s+MEMORY USED/LIMIT\s+RESTARTS\s+NODE\s+FLAGS$`))
		Expect(lines[1]).To(MatchRegexp(`^api-7f9c/api\s+0\.31/0\.25/none\s+393Mi/1\.0Gi\s+2\s+node02\s+! no CPU limit, OOMKilled 2×$`))
		Expect(lines[2]).To(MatchRegexp(`^redis-0/metrics\s+0\.05/0\.01/0\.05\s+60Mi/64Mi\s+0\s+node01\s+! throttled 31%, memory 94% of limit$`))
		Expect(out.String()).To(HaveSuffix("\nnode02 is under pressure (the node, not your limits): CPU wait 95%, DiskPressure\n"))
	})
})
