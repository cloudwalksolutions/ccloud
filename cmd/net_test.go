package cmd_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/app"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

var (
	apiDeployment     = app.Workload{Kind: "Deployment", Name: "api"}
	workerStatefulSet = app.Workload{Kind: "StatefulSet", Name: "worker"}
)

func answer(v any) string {
	GinkgoHelper()
	encoded, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	return string(encoded)
}

func section(out string, name string) []string {
	var rows []string
	in := false
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		heading := !strings.HasPrefix(line, " ")
		if heading {
			in = strings.HasPrefix(line, name)
		}
		if in && !heading {
			rows = append(rows, strings.Join(strings.Fields(line), " "))
		}
	}
	return rows
}

var _ = Describe("ccloud net", func() {
	var (
		out, errs *bytes.Buffer
		gateway   *fakeGateway
		home      cloud.Cloud
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, errs, home, "dev")
	}

	prod := func(args ...string) error {
		return run(append([]string{"net", "-a", "acme", "-e", "prod", "-r", "north1"}, args...)...)
	}

	BeforeEach(func() {
		out, errs = &bytes.Buffer{}, &bytes.Buffer{}
		gateway = &fakeGateway{hosts: []string{"prod"}}
		home = servedBy(gateway)
		signInTools()
	})

	It("lists each host the app serves with its request rate, server errors and p95", func() {
		gateway.network = answer(app.Traffic{Ingress: app.Ingress{RequestsPerSecond: 3, Hosts: []app.Host{
			{Host: "shop.example.test", Service: "web", RequestsPerSecond: 3, ServerErrors: 0.08, P95Seconds: 0.04},
		}}})

		Expect(prod()).To(Succeed())

		Expect(section(out.String(), "INGRESS")).To(ContainElement("shop.example.test web 3.00 8.0% 40ms"))
	})

	It("leaves out a host that had no requests", func() {
		gateway.network = answer(app.Traffic{Ingress: app.Ingress{RequestsPerSecond: 3, Hosts: []app.Host{
			{Host: "shop.example.test", Service: "web", RequestsPerSecond: 3},
			{Host: "idle.example.test", Service: "web"},
		}}})

		Expect(prod()).To(Succeed())

		Expect(section(out.String(), "INGRESS")).NotTo(ContainElement(HavePrefix("idle.example.test")))
	})

	It("puts each destination under the workload that called it", func() {
		gateway.network = answer(app.Traffic{Egress: []app.Egress{
			{Workload: workerStatefulSet, Destination: "storage.example.test", Port: 443, Connections: 8},
			{Workload: apiDeployment, Destination: "hydro.example.test", Port: 443, Connections: 412},
		}})

		Expect(prod()).To(Succeed())

		Expect(section(out.String(), "EGRESS")[1:]).To(HaveExactElements(
			"api (Deployment)",
			HavePrefix("hydro.example.test "),
			"worker (StatefulSet)",
			HavePrefix("storage.example.test "),
		))
	})

	It("lists a workload's busiest destination first", func() {
		gateway.network = answer(app.Traffic{Egress: []app.Egress{
			{Workload: apiDeployment, Destination: "quiet.example.test", Port: 443, Connections: 20},
			{Workload: apiDeployment, Destination: "busy.example.test", Port: 443, Connections: 412},
		}})

		Expect(prod()).To(Succeed())

		Expect(section(out.String(), "EGRESS")[2:]).To(HaveExactElements(HavePrefix("busy."), HavePrefix("quiet.")))
	})

	It("shows how many connections went unanswered or were reset, and what share that is", func() {
		gateway.network = answer(app.Traffic{Egress: []app.Egress{
			{Workload: apiDeployment, Destination: "hydro.example.test", Port: 443, Connections: 412, Unanswered: 398, Reset: 3},
		}})

		Expect(prod()).To(Succeed())

		Expect(section(out.String(), "EGRESS")).To(ContainElement("hydro.example.test 443 412 398 (97%) 3 (1%)"))
	})

	It("lists what was dropped under the workload that tried it, with why", func() {
		gateway.network = answer(app.Traffic{Drops: []app.Drop{
			{Workload: workerStatefulSet, Destination: "10.0.4.9", Port: 5432, Reason: "POLICY_DENIED", Count: 12},
		}})

		Expect(prod()).To(Succeed())

		Expect(section(out.String(), "DROPPED")[1:]).To(HaveExactElements("worker (StatefulSet)", "10.0.4.9 5432 POLICY_DENIED 12"))
	})

	It("says none for a direction that had nothing", func() {
		gateway.network = answer(app.Traffic{})

		Expect(prod()).To(Succeed())

		Expect(lines(out)[1:]).To(Equal([]string{"INGRESS  none", "EGRESS  none", "DROPPED  none"}))
	})

	It("looks back as far as asked", func() {
		gateway.network = answer(app.Traffic{})

		Expect(prod("--since", "6h")).To(Succeed())

		Expect(gateway.asked[0].URL.Query().Get("since")).To(Equal("6h0m0s"))
		Expect(lines(out)[0]).To(Equal("prod · north1 · last 6h"))
	})

	It("marks each direction with an arrow on a terminal", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		gateway.network = answer(app.Traffic{})

		Expect(prod()).To(Succeed())

		Expect(plain(out.String())).To(ContainSubstring("↓ INGRESS  ✓ none\n↑ EGRESS  ✓ none\n✗ DROPPED  ✓ none\n"))
	})

	It("colors a value yellow or red once it crosses its threshold, and leaves the rest alone", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		gateway.network = answer(app.Traffic{
			Ingress: app.Ingress{RequestsPerSecond: 1, ServerErrors: 0.08, P95Seconds: 0.9},
			Egress:  []app.Egress{{Workload: apiDeployment, Destination: "hydro.example.test", Port: 443, Connections: 412, Unanswered: 398, Reset: 3}},
		})

		Expect(prod()).To(Succeed())

		Expect(out.String()).To(ContainSubstring(red("8.0%")), "5xx at 1% or more")
		Expect(out.String()).To(ContainSubstring(yellow("900ms")), "p95 at 500ms or more")
		Expect(out.String()).To(ContainSubstring(red("398 (97%)")), "unanswered at 10% or more")
		Expect(out.String()).To(ContainSubstring(" 3 (1%)\n"), "a reset under 5% stays plain")
	})

	It("sums up every environment when none is named, prod first", func() {
		gateway.network = answer(map[string][]app.EnvironmentTraffic{"environments": {
			{Name: "stage"},
			{Name: "prod", Traffic: app.Traffic{
				Egress: []app.Egress{{Workload: apiDeployment, Destination: "a.example.test", Connections: 400}, {Workload: workerStatefulSet, Destination: "b.example.test", Connections: 32, Unanswered: 32}},
				Drops:  []app.Drop{{Workload: apiDeployment, Destination: "10.0.4.9", Count: 12}},
			}},
		}})

		Expect(run("net", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(gateway.asked[0].URL.Query().Has("env")).To(BeFalse())
		Expect(lines(out)[1:]).To(HaveExactElements(
			HavePrefix("prod "),
			HavePrefix("stage "),
		))
		Expect(strings.Join(strings.Fields(lines(out)[1]), " ")).To(HaveSuffix("2 432 32 (7%) 12"), "hosts, connections, unanswered, dropped")
	})

	It("asks every region for its environments when no region is named", func() {
		other := &fakeGateway{network: answer(map[string][]app.EnvironmentTraffic{"environments": {{Name: "stage"}}})}
		gateway.network = answer(map[string][]app.EnvironmentTraffic{"environments": {{Name: "prod"}}})
		home = servedBy(gateway, other)

		Expect(run("net", "-a", "acme")).To(Succeed())

		Expect(lines(out)[1:]).To(HaveExactElements(HavePrefix("prod "), HavePrefix("stage ")))
	})

	It("names the region and what its gateway said when it cannot sum up every environment", func() {
		gateway.refuse = http.StatusBadGateway

		err := run("net", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("asking north1 about acme's traffic: the gateway answered 502")))
	})

	It("asks which app outside an app repo", func() {
		Expect(run("net")).To(MatchError("which app? ccloud net --app <app>"))
	})

	It("refuses a region the cloud does not have", func() {
		Expect(run("net", "-a", "acme", "-r", "west9")).To(MatchError(HavePrefix(`no region "west9"`)))
	})

	It("names the environments the app has when none is the one asked for", func() {
		home = servedBy(gateway, &fakeGateway{})

		Expect(run("net", "-a", "acme", "-e", "feat-x")).To(MatchError(HavePrefix("no region hosts acme's feat-x; it has prod (north1)")))
	})

	It("names the region and what its gateway said when it cannot answer", func() {
		gateway.refuse = http.StatusBadGateway

		err := prod()

		Expect(err).To(MatchError(HavePrefix("asking north1 about acme's prod traffic: the gateway answered 502")))
		Expect(errs.String()).To(Equal("Asking north1 about acme's prod traffic…\n"))
	})
})

func red(text string) string    { return "\x1b[31m" + text + "\x1b[0m" }
func yellow(text string) string { return "\x1b[33m" + text + "\x1b[0m" }
