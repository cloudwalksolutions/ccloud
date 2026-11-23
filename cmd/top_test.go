package cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

const prod = `{"environments":[{"name":"prod","namespace":"prod-acme",
	"containers":[
		{"pod":"api-7f9c","container":"api","node":"node02","workload":{"kind":"Deployment","name":"api"},
		 "cpu":{"used":0.31,"request":0.25},"memory":{"used":412000000,"limit":1073741824},"restarts":2,"oomKills":2},
		{"pod":"redis-0","container":"metrics","node":"node01","workload":{"kind":"StatefulSet","name":"redis"},
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

	It("on a terminal, draws the tree, colors each issue by how serious it is, lines numbers up on the right and explains the colors", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		south.resources = `{"environments":[{"name":"stage","namespace":"stage-acme",
			"containers":[
				{"pod":"web-1","container":"web","node":"node03","workload":{"kind":"Deployment","name":"web"},
				 "cpu":{"used":0.1,"request":0.1,"limit":0.5},"memory":{"used":104857600},"restarts":3,"oomKills":0},
				{"pod":"worker-1","container":"worker","node":"node03","workload":{"kind":"Deployment","name":"worker"},
				 "cpu":{"used":0.1,"request":0.1,"limit":0.5},"memory":{"used":104857600,"limit":209715200},"restarts":0,"oomKills":0}],
			"nodes":[{"name":"node03","cpuWait":0,"memoryStall":0.2,"conditions":[]}]}]}`

		Expect(run("top", "-a", "acme")).To(Succeed())

		Expect(out.String()).To(ContainSubstring("\x1b[31mrestarted 3×\x1b[0m, \x1b[2mno memory limit\x1b[0m"))
		Expect(out.String()).To(ContainSubstring("\x1b[33mmemory stall 20%\x1b[0m"))
		lines := strings.Split(strings.TrimSpace(regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out.String(), "")), "\n")
		Expect(lines).To(HaveExactElements(
			MatchRegexp(`^APP · ENV · WORKLOAD\s+PODS`),
			MatchRegexp(`^acme\s+2`),
			MatchRegexp(`^└─ stage\s+2\s+200m\s+100%\s+20%\s+200Mi\s+—\s+50%$`),
			MatchRegexp(`^   ├─ web \(Deployment\)\s+1`),
			MatchRegexp(`^   ├─ worker \(Deployment\)\s+1`),
			MatchRegexp(`^   └─ node03 \(node\)\s+memory stall 20%$`),
			Equal("memory: yellow ≥75% of its limit, red ≥90% · dim: no limit set · -v shows each container"),
		))
		column := func(line, text string) int { return utf8.RuneCountInString(line[:strings.Index(line, text)]) }
		Expect(column(lines[0], "PODS")+len("PODS")).To(Equal(column(lines[2], "2 ")+1), "counts end where their header does")
	})

	It("names the region it could not ask", func() {
		north.refuse = http.StatusBadGateway

		err := run("top", "-a", "acme")

		Expect(err).To(MatchError(HavePrefix("asking north1 what acme uses: the gateway answered 502")))
		Expect(errs.String()).To(Equal("Asking north1 and south1 what acme uses…\n"))
	})

	It("shows the app as a tree of its environments and their workloads, each with what it uses against its requests and limits", func() {
		north.resources = `{"environments":[{"name":"ci","namespace":"ci-acme",
			"containers":[{"pod":"runner","container":"main","node":"node05",
			 "cpu":{"used":0.1,"request":0.2},"memory":{"used":104857600,"limit":209715200},"restarts":0,"oomKills":0}],
			"nodes":[{"name":"node05","cpuWait":0,"memoryStall":0,"conditions":[]}]}]}`
		south.resources = prod

		Expect(run("top", "-a", "acme")).To(Succeed())

		Expect(strings.Split(strings.TrimSpace(out.String()), "\n")).To(HaveExactElements(
			MatchRegexp(`^APP · ENV · WORKLOAD\s+PODS\s+CPU\s+%REQ\s+%LIM\s+MEM\s+%REQ\s+%LIM\s+ISSUES$`),
			MatchRegexp(`^acme\s+3\s+460m\s+100%\s+100%\s+553Mi\s+—\s+43%$`),
			MatchRegexp(`^  prod · south1\s+2\s+360m\s+138%\s+100%\s+453Mi\s+—\s+42%$`),
			MatchRegexp(`^    api \(Deployment\)\s+1\s+310m\s+124%\s+—\s+393Mi\s+—\s+38%\s+OOMKilled 2×, restarted 2×, no CPU limit$`),
			MatchRegexp(`^    redis \(StatefulSet\)\s+1\s+50m\s+500%\s+100%\s+60Mi\s+—\s+94%\s+memory 94% of limit, throttled 31%$`),
			MatchRegexp(`^    node02 \(node\)\s+CPU wait 95%, DiskPressure$`),
			MatchRegexp(`^  ci · north1\s+1\s+100m\s+50%\s+—\s+100Mi\s+—\s+50%$`),
			MatchRegexp(`^    runner \(Pod\)\s+1\s+100m\s+50%\s+—\s+100Mi\s+—\s+50%\s+no CPU limit$`),
		))
	})

	It("lists prod, stage and ci first, then feature environments in number order, and says why a container is not running", func() {
		env := func(name string) string {
			return `{"name":"` + name + `","namespace":"` + name + `-acme","nodes":[],"containers":[{"pod":"api-1","container":"api","node":"node01",
				"cpu":{"used":0},"memory":{"used":0},"restarts":0,"oomKills":0}]}`
		}
		south.resources = `{"environments":[` + env("acme-113") + "," + env("ci") + "," + env("acme-31") + "," + env("prod") + `,
			{"name":"stage","namespace":"stage-acme","nodes":[],"containers":[{"pod":"api-2","container":"api","node":"node01","waiting":"CrashLoopBackOff",
				"cpu":{"used":0},"memory":{"used":0},"restarts":5,"oomKills":0}]}]}`

		Expect(run("top", "-a", "acme")).To(Succeed())

		var envs []string
		for _, line := range strings.Split(out.String(), "\n") {
			if label, ok := strings.CutPrefix(line, "  "); ok && !strings.HasPrefix(label, " ") {
				envs = append(envs, strings.Fields(label)[0])
			}
		}
		Expect(envs).To(Equal([]string{"prod", "stage", "ci", "acme-31", "acme-113"}))
		Expect(out.String()).To(MatchRegexp(`\n    api-2 \(Pod\)\s+1\s+0m\s+—\s+—\s+0Mi\s+—\s+—\s+not running: CrashLoopBackOff, restarted 5×, no CPU limit, no memory limit\n`))
	})

	It("counts whole cores and gibibytes once an environment uses more than one of each", func() {
		south.resources = `{"environments":[{"name":"prod","namespace":"prod-acme","nodes":[],"containers":[{"pod":"api-1","container":"api","node":"node01",
			"cpu":{"used":1.25},"memory":{"used":1610612736},"restarts":0,"oomKills":0}]}]}`

		Expect(run("top", "-a", "acme")).To(Succeed())

		Expect(out.String()).To(MatchRegexp(`\n  prod\s+1\s+1\.2\s+—\s+—\s+1\.5Gi\s`))
	})

	It("narrows to one environment, and with -v shows each workload's containers and the node each runs on", func() {
		south.resources = prod

		Expect(run("top", "-a", "acme", "-e", "prod", "-v")).To(Succeed())

		Expect(south.asked[0].URL.Query().Get("env")).To(Equal("prod"))
		Expect(strings.Split(strings.TrimSpace(out.String()), "\n")).To(HaveExactElements(
			MatchRegexp(`^APP · ENV · WORKLOAD\s+PODS\s+CPU`),
			MatchRegexp(`^acme\s+2\s+360m`),
			MatchRegexp(`^  prod\s+2\s+360m`),
			MatchRegexp(`^    api \(Deployment\)\s+1\s+310m`),
			MatchRegexp(`^      api-7f9c/api on node02\s+310m\s+124%\s+—\s+393Mi\s+—\s+38%\s+OOMKilled 2×, restarted 2×, no CPU limit$`),
			MatchRegexp(`^    redis \(StatefulSet\)\s+1\s+50m`),
			MatchRegexp(`^      redis-0/metrics on node01\s+50m\s+500%\s+100%\s+60Mi\s+—\s+94%\s+memory 94% of limit, throttled 31%$`),
			MatchRegexp(`^    node02 \(node\)\s+CPU wait 95%, DiskPressure$`),
		))
	})
})
