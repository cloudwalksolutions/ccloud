package cmd_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

type fakeGateway struct {
	asked   []*http.Request
	answers []string
	refuse  int
	after   func(asked int)
}

func (g *fakeGateway) serve(w http.ResponseWriter, r *http.Request) {
	g.asked = append(g.asked, r)
	if g.after != nil {
		defer g.after(len(g.asked))
	}
	if g.refuse != 0 {
		http.Error(w, "dev@example.test may not read logs in prod-shop", g.refuse)
		return
	}
	answer := `{"entries":[]}`
	if len(g.answers) > 0 {
		answer, g.answers = g.answers[0], g.answers[1:]
	}
	_, _ = w.Write([]byte(answer))
}

var _ = Describe("ccloud logs", func() {
	var (
		out     *bytes.Buffer
		gateway *fakeGateway
		home    cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, home, "dev")
	}

	tool := func(bin, name, script string) {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		gateway = &fakeGateway{}
		server := httptest.NewServer(http.HandlerFunc(gateway.serve))
		DeferCleanup(server.Close)
		home = example
		home.Regions = append([]cloud.Region{}, example.Regions...)
		home.Regions[0].Logs = server.URL

		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `[ "$1 $2" = "access token" ] && echo access-jwt`)
		tool(bin, "kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
	})

	It("asks which of the app's environments to read", func() {
		err := run("logs", "-a", "shop", "-r", "north1")

		Expect(err).To(MatchError(ContainSubstring("which environment? ccloud logs --env <env>")))
	})

	It("flattens a JSON line into its level, message and fields, and leaves other lines as they are", func() {
		gateway.answers = []string{`{"entries":[
			{"time":"2026-10-26T07:33:21Z","pod":"api-7f9c","line":"{\"level\":\"error\",\"msg\":\"db query failed\",\"table\":\"orders\",\"attempt\":3}"},
			{"time":"2026-10-26T07:33:22Z","pod":"web-5d2a","line":"POST /checkout 500"}]}`}

		Expect(run("logs", "-a", "shop", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(lines(out)[0]).To(HaveSuffix(" api-7f9c ERROR db query failed attempt=3 table=orders"))
		Expect(lines(out)[1]).To(HaveSuffix(" web-5d2a POST /checkout 500"))
	})

	It("colours a level by how bad it is, when asked to, as on a terminal", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		gateway.answers = []string{`{"entries":[
			{"time":"2026-10-26T07:33:21Z","pod":"api","line":"{\"level\":\"error\",\"msg\":\"a\"}"},
			{"time":"2026-10-26T07:33:21Z","pod":"api","line":"{\"level\":\"warn\",\"msg\":\"b\"}"},
			{"time":"2026-10-26T07:33:21Z","pod":"api","line":"{\"level\":\"info\",\"msg\":\"c\"}"}]}`}

		Expect(run("logs", "-a", "shop", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(ContainSubstring("\x1b[31mERROR\x1b[0m a"))
		Expect(out.String()).To(ContainSubstring("\x1b[33mWARN\x1b[0m b"))
		Expect(out.String()).To(ContainSubstring("\x1b[36mINFO\x1b[0m c"))
	})

	It("prints lines exactly as stored with -o raw, and one JSON entry per line with -o json", func() {
		entries := `{"entries":[{"time":"2026-10-26T07:33:21Z","namespace":"prod-shop","pod":"api-7f9c","container":"api","line":"{\"level\":\"error\",\"msg\":\"boom\"}"}]}`
		gateway.answers = []string{entries, entries}

		Expect(run("logs", "-a", "shop", "-e", "prod", "-r", "north1", "-o", "raw")).To(Succeed())
		Expect(out.String()).To(Equal(`{"level":"error","msg":"boom"}` + "\n"))

		out.Reset()
		Expect(run("logs", "-a", "shop", "-e", "prod", "-r", "north1", "-o", "json")).To(Succeed())
		Expect(out.String()).To(MatchJSON(`{"time":"2026-10-26T07:33:21Z","namespace":"prod-shop","pod":"api-7f9c","container":"api","line":"{\"level\":\"error\",\"msg\":\"boom\"}"}`))
	})

	It("keeps reading new lines from where it left off until you stop it", func() {
		ctx, stop := context.WithCancel(context.Background())
		gateway.answers = []string{
			`{"entries":[{"time":"2026-10-26T07:33:21.5Z","pod":"api-7f9c","line":"first"}]}`,
			`{"entries":[{"time":"2026-10-26T07:33:22Z","pod":"api-7f9c","line":"second"}]}`,
		}
		gateway.after = func(asked int) {
			if asked == 3 {
				stop()
			}
		}

		Expect(cmd.RunContext(ctx, []string{"logs", "-a", "shop", "-e", "prod", "-r", "north1", "-f", "-i", "1ms"}, out, out, home, "dev")).To(Succeed())

		Expect(lines(out)).To(HaveLen(2))
		Expect(lines(out)[0]).To(HaveSuffix("api-7f9c first"))
		Expect(lines(out)[1]).To(HaveSuffix("api-7f9c second"))
		next := gateway.asked[1].URL.Query()
		Expect(next.Get("start")).To(Equal("2026-10-26T07:33:21.500000001Z"))
		Expect(next.Get("direction")).To(Equal("forward"))
		Expect(gateway.asked[2].URL.Query().Get("start")).To(Equal("2026-10-26T07:33:22.000000001Z"))
	})

	It("narrows by time, pod, container, text, pattern and level, and reads as many lines as asked", func() {
		Expect(run("logs", "-a", "shop", "-e", "prod", "-r", "north1",
			"--since", "6h", "--tail", "50", "--pod", "api-", "-c", "api", "-g", "checkout", "-E", "5[0-9]{2}", "-l", "error")).To(Succeed())

		query := gateway.asked[0].URL.Query()
		Expect(time.Parse(time.RFC3339, query.Get("start"))).To(BeTemporally("~", time.Now().Add(-6*time.Hour), 5*time.Second))
		Expect(query.Get("limit")).To(Equal("50"))
		Expect(query.Get("pod")).To(Equal("api-"))
		Expect(query.Get("container")).To(Equal("api"))
		Expect(query.Get("contains")).To(Equal("checkout"))
		Expect(query.Get("regex")).To(Equal("5[0-9]{2}"))
		Expect(query.Get("level")).To(Equal("error"))
	})

	It("says so when the cluster does not let you read that environment's logs", func() {
		gateway.refuse = http.StatusForbidden

		err := run("logs", "-a", "shop", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError("you may not read shop's prod logs in north1: dev@example.test may not read logs in prod-shop"))
	})

	It("signs you in to Access first when you have no session for the region yet", func() {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
		session := filepath.Join(bin, "session")
		tool(bin, "cloudflared", `case "$1 $2" in
  "access login") echo "$3" > `+session+` ;;
  "access token") [ -f `+session+` ] && echo access-jwt || { echo "Unable to find token" >&2; exit 1; } ;;
esac`)

		Expect(run("logs", "-a", "shop", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(os.ReadFile(session)).To(Equal([]byte(home.Regions[0].Logs + "\n")))
		Expect(gateway.asked).To(HaveLen(1))
		Expect(gateway.asked[0].Header.Get("cf-access-token")).To(Equal("access-jwt"))
	})

	It("names a logs address in the cloud's definition that is not a URL", func() {
		home.Regions[0].Logs = "https://logs north1"

		err := run("logs", "-a", "shop", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix(`north1's logs gateway "https://logs north1" is not a URL: `)))
	})

	It("says so when something other than the gateway answers, such as Access's login page", func() {
		gateway.answers = []string{"<!DOCTYPE html><title>Sign in ・ Cloudflare Access</title>"}

		err := run("logs", "-a", "shop", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("the logs gateway answered something other than logs")))
	})

	It("stops waiting for your Access sign-in when you press Ctrl-C", func(ctx SpecContext) {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `[ "$2" = "login" ] && exec /bin/sleep 5; exit 1`)
		interrupted, interrupt := context.WithCancel(ctx)
		time.AfterFunc(100*time.Millisecond, interrupt)

		Expect(cmd.RunContext(interrupted, []string{"logs", "-a", "shop", "-e", "prod", "-r", "north1"}, out, out, home, "dev")).To(Succeed())

		Expect(gateway.asked).To(BeEmpty())
	}, SpecTimeout(3*time.Second))

	It("says so when Cloudflare Access would not sign you in, and asks the gateway nothing", func() {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `echo "Unable to reach the login page" >&2; exit 1`)

		err := run("logs", "-a", "shop", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("signing in to " + home.Regions[0].Logs + " through Cloudflare Access")))
		Expect(out.String()).To(ContainSubstring("Unable to reach the login page"))
		Expect(gateway.asked).To(BeEmpty())
	})

	It("names the gateway and what it answered when it cannot serve logs", func() {
		gateway.refuse = http.StatusBadGateway

		err := run("logs", "-a", "shop", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError("the logs gateway answered 502 Bad Gateway: dev@example.test may not read logs in prod-shop"))
	})

	It("shows why the cloud's issuer would not sign you in, and asks the gateway nothing", func() {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `echo access-jwt`)
		tool(bin, "kubectl-oidc_login", `echo "oauth2: invalid_grant" >&2; exit 1`)

		err := run("logs", "-a", "shop", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError("signing in to " + home.Issuer + " failed"))
		Expect(out.String()).To(ContainSubstring("oauth2: invalid_grant"))
		Expect(gateway.asked).To(BeEmpty())
	})

	It("reads the environment's last hour as you, through Access, oldest line first", func() {
		at := time.Date(2026, 10, 26, 7, 33, 21, 0, time.UTC)
		gateway.answers = []string{`{"entries":[
			{"time":"2026-10-26T07:33:21Z","namespace":"prod-shop","pod":"api-7f9c","container":"api","line":"db query failed"},
			{"time":"2026-10-26T07:33:22Z","namespace":"prod-shop","pod":"web-5d2a","container":"web","line":"POST /checkout 500"}]}`}

		Expect(run("logs", "-a", "shop", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(gateway.asked).To(HaveLen(1))
		asked := gateway.asked[0]
		Expect(asked.URL.Path).To(Equal("/v1/logs"))
		Expect(asked.Header.Get("cf-access-token")).To(Equal("access-jwt"))
		Expect(asked.Header.Get("Authorization")).To(Equal("Bearer id-token"))
		query := asked.URL.Query()
		Expect(query["namespace"]).To(Equal([]string{"prod-shop"}))
		Expect(query.Get("limit")).To(Equal("200"))
		Expect(query.Get("direction")).To(Equal("backward"))
		Expect(time.Parse(time.RFC3339, query.Get("start"))).To(BeTemporally("~", time.Now().Add(-time.Hour), 5*time.Second))
		Expect(lines(out)).To(HaveExactElements(
			at.Local().Format("15:04:05")+" api-7f9c db query failed",
			at.Add(time.Second).Local().Format("15:04:05")+" web-5d2a POST /checkout 500",
		))
	})
})
