package cmd_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud workflow view", func() {
	var (
		out, errs *bytes.Buffer
		ci        *fakeCluster
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, errs, example, "dev")
	}

	BeforeEach(func() {
		out, errs = &bytes.Buffer{}, &bytes.Buffer{}
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		ci = newFakeCluster()
		ci.signIn("north1")
	})

	It("points you at the list when the run is not there", func() {
		err := run("workflow", "view", "api-tests-gone", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("acme has no run api-tests-gone in north1; see ccloud workflow list -a acme -r north1"))
		Expect(errs.String()).To(Equal("Reading api-tests-gone in north1…\n"))
	})

	It("shows the run's steps in the order they ran, with how long each took and why one failed", func() {
		started := time.Now().Add(-time.Hour).Truncate(time.Second)
		ci.workflows("ci-acme", ciRun("api-tests-x7k2p", started).flow("api-tests").
			pr("105", "3f9a2c4e1d", "feat/BOAT-105/search").finished("Failed", 2*time.Minute+3*time.Second).
			step("api-tests-x7k2p-2", "unit", "Failed", started.Add(45*time.Second), time.Minute+18*time.Second, "exit code 1").
			step("api-tests-x7k2p-1", "lint", "Succeeded", started.Add(time.Second), 41*time.Second, "").
			step("api-tests-x7k2p-4", "deploy-preview", "Skipped", time.Time{}, 0, "when 'false' evaluated false").
			step("api-tests-x7k2p-3", "e2e", "Running", started.Add(50*time.Second), 0, "").
			step("api-tests-x7k2p-5", "api-tests-x7k2p.hooks.running", "Succeeded", started, time.Second, ""))

		Expect(run("workflow", "view", "api-tests-x7k2p", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(Equal(`X api-tests · #105 · feat/BOAT-105/search · 3f9a2c4
api-tests-x7k2p started 1h ago and took 2m3s

✓ lint in 41s
X unit in 1m18s
  exit code 1
* e2e
- deploy-preview

To see what failed, try: ccloud workflow logs api-tests-x7k2p -a acme -r north1 --failed
`))
	})

	It("lists steps that started at the same moment by name, the same way every time", func() {
		started := time.Now().Add(-time.Hour).Truncate(time.Second)
		run := ciRun("api-tests-x7k2p", started).flow("api-tests").finished("Succeeded", time.Minute)
		for i, name := range []string{"web", "api", "lint", "unit", "docs", "e2e"} {
			run = run.step(fmt.Sprintf("api-tests-x7k2p-%d", i), name, "Succeeded", started.Add(time.Second), time.Second, "")
		}
		ci.workflows("ci-acme", run)

		Expect(cmd.Run([]string{"workflow", "view", "api-tests-x7k2p", "-a", "acme", "-r", "north1"}, out, errs, example, "dev")).To(Succeed())

		Expect(lines(out)[3:9]).To(Equal([]string{"✓ api in 1s", "✓ docs in 1s", "✓ e2e in 1s", "✓ lint in 1s", "✓ unit in 1s", "✓ web in 1s"}))
	})

	It("colors each result when asked to, as on a terminal", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		started := time.Now().Add(-time.Hour)
		ci.workflows("ci-acme", ciRun("api-tests-x7k2p", started).flow("api-tests").finished("Failed", time.Minute).
			step("api-tests-x7k2p-1", "lint", "Succeeded", started, time.Second, "").
			step("api-tests-x7k2p-2", "unit", "Failed", started.Add(time.Second), time.Second, "exit code 1"))

		Expect(run("workflow", "view", "api-tests-x7k2p", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(ContainSubstring("\x1b[32m✓\x1b[0m lint"))
		Expect(out.String()).To(ContainSubstring("\x1b[31mX\x1b[0m unit"))
	})

	It("says how long a run still going has been running", func() {
		ci.workflows("ci-acme", ciRun("release-m4k2p", time.Now().Add(-90*time.Second)).flow("release").running())

		Expect(run("workflow", "view", "release-m4k2p", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(ContainElement(MatchRegexp(`^release-m4k2p started 1m ago and has run for 1m3\ds$`)))
	})

	It("asks which run you want to see", func() {
		err := run("workflow", "view", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError(ContainSubstring("which run? ccloud workflow view <run>")))
	})
})
