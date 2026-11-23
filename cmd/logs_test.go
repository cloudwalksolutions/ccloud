package cmd_test

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

type fakeGateway struct {
	asked     []*http.Request
	answers   []string
	refuse    int
	after     func(asked int)
	hosts     []string
	resources string
	network   string
	summary   string
	streams   []reply
	slow      time.Duration
	asking    func()
}

type reply struct {
	status int
	body   string
}

func (g *fakeGateway) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/logs/containers" && g.refuse == 0 {
		g.asked = append(g.asked, r)
		_, _ = w.Write([]byte(`{"containers":["api","nginx"]}`))
		return
	}
	if strings.HasSuffix(r.URL.Path, "/resources") && g.refuse == 0 {
		g.asked = append(g.asked, r)
		_, _ = w.Write([]byte(cmp.Or(g.resources, `{"environments":[]}`)))
		return
	}
	if strings.HasSuffix(r.URL.Path, "/summary") && g.refuse == 0 {
		g.asked = append(g.asked, r)
		time.Sleep(g.slow)
		_, _ = w.Write([]byte(g.summary))
		return
	}
	if strings.HasSuffix(r.URL.Path, "/network") && g.refuse == 0 {
		g.asked = append(g.asked, r)
		_, _ = w.Write([]byte(g.network))
		return
	}
	if app, ok := strings.CutPrefix(r.URL.Path, "/v1/apps/"); ok && g.refuse == 0 {
		time.Sleep(g.slow)
		if g.asking != nil {
			g.asking()
		}
		environments := []string{}
		for _, env := range g.hosts {
			url := ""
			if env != "ci" {
				url = fmt.Sprintf(`,"url":"https://%s.%s.test"`, env, app)
			}
			environments = append(environments, fmt.Sprintf(`{"name":%q,"namespace":"%s-%s"%s}`, env, env, app, url))
		}
		_, _ = fmt.Fprintf(w, `{"environments":[%s]}`, strings.Join(environments, ","))
		return
	}
	g.asked = append(g.asked, r)
	if g.after != nil {
		defer g.after(len(g.asked))
	}
	if g.refuse != 0 {
		http.Error(w, "dev@example.test may not read logs in prod-acme", g.refuse)
		return
	}
	if r.URL.Path == "/v1/logs/stream" {
		if len(g.streams) == 0 {
			if g.after != nil {
				g.after(len(g.asked))
			}
			<-r.Context().Done()
			return
		}
		var next reply
		next, g.streams = g.streams[0], g.streams[1:]
		if next.status != 0 {
			http.Error(w, next.body, next.status)
			return
		}
		_, _ = w.Write([]byte(next.body))
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
		out, errs *bytes.Buffer
		gateway   *fakeGateway
		home      cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, errs, home, "dev")
	}

	tool := func(bin, name, script string) {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
	}

	BeforeEach(func() {
		out, errs = &bytes.Buffer{}, &bytes.Buffer{}
		gateway = &fakeGateway{}
		server := httptest.NewServer(http.HandlerFunc(gateway.serve))
		DeferCleanup(server.Close)
		home = example
		home.Regions = append([]cloud.Region{}, example.Regions...)
		home.Regions[0].Gateway = server.URL

		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `[ "$1 $2" = "access token" ] && echo access-jwt`)
		tool(bin, "kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
	})

	It("asks which of the app's environments to read", func() {
		err := run("logs", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError(ContainSubstring("which environment? ccloud logs --env <env>")))
	})

	It("flattens a JSON line into its level, message and fields, and leaves other lines as they are", func() {
		gateway.answers = []string{`{"entries":[
			{"time":"2026-10-26T07:33:21Z","pod":"api-7f9c","line":"{\"level\":\"error\",\"msg\":\"db query failed\",\"table\":\"orders\",\"attempt\":3}"},
			{"time":"2026-10-26T07:33:22Z","pod":"web-5d2a","line":"POST /checkout 500"}]}`}

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(lines(out)[0]).To(HaveSuffix(" api-7f9c ERROR db query failed attempt=3 table=orders"))
		Expect(lines(out)[1]).To(HaveSuffix(" web-5d2a POST /checkout 500"))
	})

	It("colors a level by how bad it is, when asked to, as on a terminal", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		gateway.answers = []string{`{"entries":[
			{"time":"2026-10-26T07:33:21Z","pod":"api","line":"{\"level\":\"error\",\"msg\":\"a\"}"},
			{"time":"2026-10-26T07:33:21Z","pod":"api","line":"{\"level\":\"warn\",\"msg\":\"b\"}"},
			{"time":"2026-10-26T07:33:21Z","pod":"api","line":"{\"level\":\"info\",\"msg\":\"c\"}"}]}`}

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(ContainSubstring("\x1b[31mERROR\x1b[0m a"))
		Expect(out.String()).To(ContainSubstring("\x1b[33mWARN\x1b[0m b"))
		Expect(out.String()).To(ContainSubstring("\x1b[36mINFO\x1b[0m c"))
	})

	It("prints lines exactly as stored with -o raw, and with -o json one entry per line then where to resume", func() {
		entries := `{"entries":[{"time":"2026-10-26T07:33:21Z","namespace":"prod-acme","pod":"api-7f9c","container":"api","stream":"stderr","level":"error","line":"{\"level\":\"error\",\"msg\":\"boom\"}"}]}`
		gateway.answers = []string{entries, entries}

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "-o", "raw")).To(Succeed())
		Expect(out.String()).To(Equal(`{"level":"error","msg":"boom"}` + "\n"))

		out.Reset()
		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "-o", "json")).To(Succeed())
		Expect(lines(out)).To(HaveExactElements(
			MatchJSON(`{"time":"2026-10-26T07:33:21Z","namespace":"prod-acme","pod":"api-7f9c","container":"api","stream":"stderr","level":"error","line":"{\"level\":\"error\",\"msg\":\"boom\"}"}`),
			MatchJSON(`{"next":"2026-10-26T07:33:21.000000001Z"}`),
		))
	})

	It("prints the newest lines, then follows the gateway's stream through heartbeats, reconnects and drops, each line once", func(spec SpecContext) {
		ctx, stop := context.WithCancel(spec)
		gateway.answers = []string{`{"entries":[{"time":"2026-10-26T07:33:21.5Z","pod":"api-7f9c","container":"api","line":"first"}]}`}
		second := `{"time":"2026-10-26T07:33:22Z","pod":"api-7f9c","container":"api","line":"second"}`
		gateway.streams = []reply{
			{body: "{}\n" + second + "\n" + `{"end":"max duration"}` + "\n"},
			{body: second + "\n" + `{"time":"2026-10-26T07:33:23Z","pod":"api-7f9c","container":"api","line":"third"}` + "\n"},
		}
		gateway.after = func(asked int) {
			if asked == 4 {
				stop()
			}
		}

		Expect(cmd.RunContext(ctx, []string{"logs", "-a", "acme", "-e", "prod", "-r", "north1", "-f", "-c", "api"}, out, errs, home, "dev")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements(HaveSuffix("api-7f9c first"), HaveSuffix("api-7f9c second"), HaveSuffix("api-7f9c third")))
		Expect(lines(errs)).To(HaveExactElements(
			"Reading acme's prod logs in north1…",
			"Following new lines; Ctrl-C stops.",
			"Lost the log stream (unexpected EOF); reconnecting in 1s.",
		))
		streamed := gateway.asked[1].URL
		Expect(streamed.Path).To(Equal("/v1/logs/stream"))
		Expect(streamed.Query().Get("start")).To(Equal("2026-10-26T07:33:21.5Z"))
		Expect(streamed.Query()["container"]).To(Equal([]string{"api"}))
		Expect(streamed.Query().Has("limit")).To(BeFalse())
		Expect(gateway.asked[2].URL.Query().Get("start")).To(Equal("2026-10-26T07:33:22Z"))
		Expect(gateway.asked[3].URL.Query().Get("start")).To(Equal("2026-10-26T07:33:23Z"))
	}, SpecTimeout(10*time.Second))

	Context("following", func() {
		follow := func(ctx context.Context, extra ...string) error {
			return cmd.RunContext(ctx, append([]string{"logs", "-a", "acme", "-e", "prod", "-r", "north1", "-f"}, extra...), out, errs, home, "dev")
		}
		line := `{"time":"2026-10-26T07:33:22Z","pod":"api-7f9c","container":"api","line":"ERR boom"}` + "\n"

		It("exits on the first line that matches, when asked to fail on a match", func(ctx SpecContext) {
			gateway.streams = []reply{{body: line}}

			Expect(follow(ctx, "--fail-on-match")).To(MatchError("a line matched"))
			Expect(lines(out)).To(HaveExactElements(HaveSuffix("api api-7f9c ERR boom")))
		}, SpecTimeout(5*time.Second))

		It("stops when the gateway says you may no longer read the environment", func(ctx SpecContext) {
			gateway.streams = []reply{{body: `{"end":"access revoked"}` + "\n"}}

			Expect(follow(ctx)).To(MatchError("you may no longer read acme's prod logs in north1"))
		}, SpecTimeout(5*time.Second))

		It("signs in again once when the gateway stops accepting your token, then gives up", func(ctx SpecContext) {
			gateway.streams = []reply{{status: http.StatusUnauthorized, body: "sign in again"}, {status: http.StatusUnauthorized, body: "sign in again"}}

			Expect(follow(ctx)).To(MatchError("the gateway answered 401 Unauthorized: sign in again"))
			Expect(gateway.asked).To(HaveLen(3))
		}, SpecTimeout(5*time.Second))

		It("says so when signing in again fails", func(ctx SpecContext) {
			bin := GinkgoT().TempDir()
			GinkgoT().Setenv("PATH", bin)
			tool(bin, "cloudflared", `echo access-jwt`)
			signedIn := filepath.Join(bin, "signed-in")
			tool(bin, "kubectl-oidc_login", `[ -f `+signedIn+` ] && exit 1; : > `+signedIn+`; echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
			gateway.streams = []reply{{status: http.StatusUnauthorized, body: "sign in again"}}

			Expect(follow(ctx)).To(MatchError("signing in to " + home.Issuer + " failed"))
		}, SpecTimeout(5*time.Second))

		It("says so when something other than logs answers the stream", func(ctx SpecContext) {
			gateway.streams = []reply{{body: "<!DOCTYPE html>\n"}}

			Expect(follow(ctx)).To(MatchError(HavePrefix("the gateway answered something other than logs")))
		}, SpecTimeout(5*time.Second))

		It("stops when you press Ctrl-C, even while waiting to reconnect", func(ctx SpecContext) {
			gateway.streams = []reply{{body: ""}}
			interrupted, interrupt := context.WithCancel(ctx)
			gateway.after = func(asked int) {
				if asked == 2 {
					time.AfterFunc(200*time.Millisecond, interrupt)
				}
			}

			Expect(follow(interrupted)).To(Succeed())
		}, SpecTimeout(5*time.Second))
	})

	It("reads a window you name, from a time and until a time or a while ago", func() {
		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "--since-time", "2026-10-26T07:00:00Z", "--until", "2026-10-26T08:00:00Z")).To(Succeed())
		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "--until", "10m")).To(Succeed())

		Expect(gateway.asked[0].URL.Query().Get("start")).To(Equal("2026-10-26T07:00:00Z"))
		Expect(gateway.asked[0].URL.Query().Get("end")).To(Equal("2026-10-26T08:00:00Z"))
		Expect(time.Parse(time.RFC3339, gateway.asked[1].URL.Query().Get("end"))).To(BeTemporally("~", time.Now().Add(-10*time.Minute), 5*time.Second))
	})

	It("fails when any line matched, when asked to, so a script can act on it", func() {
		gateway.answers = []string{`{"entries":[{"time":"2026-10-26T07:33:21Z","pod":"api-7f9c","container":"api","line":"ERR boom"}]}`, `{"entries":[]}`}

		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "-l", "error", "--fail-on-match")
		Expect(err).To(MatchError("1 line matched"))
		Expect(lines(out)).To(HaveExactElements(HaveSuffix("api api-7f9c ERR boom")))

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "-l", "error", "--fail-on-match")).To(Succeed())
	})

	It("lists the containers an environment has logged, to pick from with -c", func() {
		Expect(run("logs", "containers", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(Equal("api\nnginx\n"))
		Expect(errs.String()).To(Equal("Reading which containers acme's prod logged in north1…\n"))
		Expect(gateway.asked[0].URL.Query()["namespace"]).To(Equal([]string{"prod-acme"}))
		Expect(time.Parse(time.RFC3339, gateway.asked[0].URL.Query().Get("start"))).To(BeTemporally("~", time.Now().Add(-24*time.Hour), 5*time.Second))
	})

	It("refuses an end it cannot read", func() {
		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "--until", "yesterday")

		Expect(err).To(MatchError(`--until is a time like 2026-10-26T08:00:00Z or a while ago like 10m, not "yesterday"`))
	})

	It("refuses an output it cannot write, rather than guessing", func() {
		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "-o", "yaml")

		Expect(err).To(MatchError(`output is pretty, raw or json, not "yaml"`))
		Expect(gateway.asked).To(BeEmpty())
	})

	It("narrows by time, pod, container, text, pattern and level, and reads as many lines as asked", func() {
		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1",
			"--since", "6h", "--tail", "50", "--pod", "api-", "-c", "api", "-c", "nginx", "-g", "checkout", "-E", "5[0-9]{2}", "-l", "error")).To(Succeed())

		query := gateway.asked[0].URL.Query()
		Expect(time.Parse(time.RFC3339, query.Get("start"))).To(BeTemporally("~", time.Now().Add(-6*time.Hour), 5*time.Second))
		Expect(query.Get("limit")).To(Equal("50"))
		Expect(query.Get("pod")).To(Equal("api-"))
		Expect(query["container"]).To(Equal([]string{"api", "nginx"}))
		Expect(query.Get("contains")).To(Equal("checkout"))
		Expect(query.Get("regex")).To(Equal("5[0-9]{2}"))
		Expect(query.Get("level")).To(Equal("error"))
	})

	It("says so when the cluster does not let you read that environment's logs", func() {
		gateway.refuse = http.StatusForbidden

		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError("you may not read acme's prod logs in north1: dev@example.test may not read logs in prod-acme"))
	})

	It("signs you in to Access first when you have no session for the region yet, never printing the token", func() {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
		session := filepath.Join(bin, "session")
		tool(bin, "cloudflared", `case "$1 $2" in
  "access login") for url; do :; done; echo "$url" > `+session+`
    case " $* " in *" --quiet "*) ;; *) printf 'Successfully fetched your token:\n\naccess-jwt\n' ;; esac ;;
  "access token") [ -f `+session+` ] && echo access-jwt || { echo "Unable to find token" >&2; exit 1; } ;;
esac`)

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(out.String()).NotTo(ContainSubstring("access-jwt"))
		Expect(os.ReadFile(session)).To(Equal([]byte(home.Regions[0].Gateway + "\n")))
		Expect(gateway.asked).To(HaveLen(1))
		Expect(gateway.asked[0].Header.Get("cf-access-token")).To(Equal("access-jwt"))
	})

	It("names a gateway address in the cloud's definition that is not a URL", func() {
		home.Regions[0].Gateway = "https://api north1"

		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix(`north1's gateway "https://api north1" is not a URL: `)))
	})

	It("says so when something other than the gateway answers, such as Access's login page", func() {
		gateway.answers = []string{"<!DOCTYPE html><title>Sign in ・ Cloudflare Access</title>"}

		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("the gateway answered something other than logs")))
	})

	It("stops waiting for your Access sign-in when you press Ctrl-C", func(ctx SpecContext) {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `[ "$2" = "login" ] && exec /bin/sleep 5; exit 1`)
		interrupted, interrupt := context.WithCancel(ctx)
		time.AfterFunc(100*time.Millisecond, interrupt)

		Expect(cmd.RunContext(interrupted, []string{"logs", "-a", "acme", "-e", "prod", "-r", "north1"}, out, out, home, "dev")).To(Succeed())

		Expect(gateway.asked).To(BeEmpty())
	}, SpecTimeout(3*time.Second))

	It("says so when Cloudflare Access would not sign you in, and asks the gateway nothing", func() {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `echo "Unable to reach the login page" >&2; exit 1`)

		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("signing in to " + home.Regions[0].Gateway + " through Cloudflare Access")))
		Expect(errs.String()).To(ContainSubstring("Unable to reach the login page"))
		Expect(gateway.asked).To(BeEmpty())
	})

	It("names the gateway and what it answered when it cannot serve logs", func() {
		gateway.refuse = http.StatusBadGateway

		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError("the gateway answered 502 Bad Gateway: dev@example.test may not read logs in prod-acme"))
		Expect(errs.String()).To(Equal("Reading acme's prod logs in north1…\n"))
	})

	It("shows why the cloud's issuer would not sign you in, and asks the gateway nothing", func() {
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool(bin, "cloudflared", `echo access-jwt`)
		tool(bin, "kubectl-oidc_login", `echo "oauth2: invalid_grant" >&2; exit 1`)

		err := run("logs", "-a", "acme", "-e", "prod", "-r", "north1")

		Expect(err).To(MatchError("signing in to " + home.Issuer + " failed"))
		Expect(errs.String()).To(ContainSubstring("oauth2: invalid_grant"))
		Expect(gateway.asked).To(BeEmpty())
	})

	It("reads the environment's last hour as you, through Access, oldest line first", func() {
		at := time.Date(2026, 10, 26, 7, 33, 21, 0, time.UTC)
		gateway.answers = []string{`{"entries":[
			{"time":"2026-10-26T07:33:21Z","namespace":"prod-acme","pod":"api-7f9c","container":"api","line":"db query failed"},
			{"time":"2026-10-26T07:33:22Z","namespace":"prod-acme","pod":"web-5d2a","container":"web","line":"POST /checkout 500"}]}`}

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(gateway.asked).To(HaveLen(1))
		asked := gateway.asked[0]
		Expect(asked.URL.Path).To(Equal("/v1/logs"))
		Expect(asked.Header.Get("cf-access-token")).To(Equal("access-jwt"))
		Expect(asked.Header.Get("Authorization")).To(Equal("Bearer id-token"))
		query := asked.URL.Query()
		Expect(query["namespace"]).To(Equal([]string{"prod-acme"}))
		Expect(query.Get("limit")).To(Equal("200"))
		Expect(query.Get("direction")).To(Equal("backward"))
		Expect(time.Parse(time.RFC3339, query.Get("start"))).To(BeTemporally("~", time.Now().Add(-time.Hour), 5*time.Second))
		Expect(lines(out)).To(HaveExactElements(
			at.Local().Format("Jan 02 15:04:05.000")+" api api-7f9c db query failed",
			at.Add(time.Second).Local().Format("Jan 02 15:04:05.000")+" web web-5d2a POST /checkout 500",
		))
	})

	It("indents the rest of a line that runs over several, such as a stack trace, under its first", func() {
		gateway.answers = []string{`{"entries":[{"time":"2026-10-26T07:33:21Z","pod":"api-7f9c","container":"api","line":"panic: boom\ngoroutine 1:\n\tmain.go:12\n"}]}`}

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements(
			HaveSuffix(" api api-7f9c panic: boom"),
			"    goroutine 1:",
			"    \tmain.go:12",
		))
	})

	It("says when it showed only the newest lines, so you know there were more", func() {
		gateway.answers = []string{`{"entries":[{"time":"2026-10-26T07:33:21Z","pod":"api-7f9c","container":"api","line":"ready"}],"truncated":true}`}

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1", "--tail", "1")).To(Succeed())

		Expect(lines(errs)).To(ContainElement("Showed the newest 1 lines; raise --tail or narrow --since or --until for more."))
	})

	It("shows today's lines by the time alone, to the millisecond", func() {
		now := time.Now().Truncate(time.Millisecond)
		gateway.answers = []string{`{"entries":[{"time":"` + now.UTC().Format(time.RFC3339Nano) + `","pod":"api-7f9c","container":"api","line":"ready"}]}`}

		Expect(run("logs", "-a", "acme", "-e", "prod", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements(now.Local().Format("15:04:05.000") + " api api-7f9c ready"))
	})
})
