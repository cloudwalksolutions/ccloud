package cmd_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud workflow watch", func() {
	var (
		out *bytes.Buffer
		ci  *fakeCluster
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, example, "dev")
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		ci = newFakeCluster()
		ci.signIn("north1")
	})

	It("says how a run that already finished ended, without waiting", func() {
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now().Add(-time.Hour)).flow("api-tests").finished("Succeeded", time.Minute))

		Expect(run("workflow", "watch", "api-tests-x7k2p", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements(
			"✓ api-tests",
			"api-tests-x7k2p started 1h ago and took 1m0s",
			"",
			"✓ api-tests-x7k2p completed: Succeeded",
		))
	})

	It("exits non-zero for a run that did not succeed, when asked to, so a script can wait on it", func() {
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now().Add(-time.Hour)).flow("api-tests").finished("Failed", time.Minute))

		err := run("workflow", "watch", "api-tests-x7k2p", "-a", "shop", "-r", "north1", "--exit-status")

		Expect(err).To(MatchError("api-tests-x7k2p did not succeed: Failed"))
	})

	It("redraws the run's steps on a terminal each time it looks, until the run ends", func() {
		GinkgoT().Setenv("CCLOUD_FORCE_TTY", "1")
		GinkgoT().Setenv("NO_COLOR", "1")
		started := time.Now().Add(-time.Minute)
		ci.progress("ci-shop",
			ciRun("api-tests-x7k2p", started).flow("api-tests").running().
				step("api-tests-x7k2p-1", "lint", "Running", started, 0, ""),
			ciRun("api-tests-x7k2p", started).flow("api-tests").finished("Succeeded", time.Minute).
				step("api-tests-x7k2p-1", "lint", "Succeeded", started, 41*time.Second, ""),
		)

		Expect(run("workflow", "watch", "api-tests-x7k2p", "-a", "shop", "-r", "north1", "-i", "1ms")).To(Succeed())

		screens := strings.Split(out.String(), "\x1b[H\x1b[2J")
		Expect(screens).To(HaveLen(3))
		Expect(screens[1]).To(ContainSubstring("* lint"))
		Expect(screens[1]).To(ContainSubstring("Looking again every 1ms; Ctrl-C stops watching, not the run."))
		Expect(screens[2]).To(ContainSubstring("✓ lint in 41s"))
		Expect(screens[2]).To(HaveSuffix("✓ api-tests-x7k2p completed: Succeeded\n"))
	})

	It("stops watching at once when you press Ctrl-C, without calling it a failure", func(ctx SpecContext) {
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now()).flow("api-tests").running())
		interrupted, interrupt := context.WithCancel(ctx)
		time.AfterFunc(100*time.Millisecond, interrupt)

		err := cmd.RunContext(interrupted, []string{"workflow", "watch", "api-tests-x7k2p", "-a", "shop", "-r", "north1", "-i", "1h"}, out, out, example, "dev")

		Expect(err).NotTo(HaveOccurred())
	}, SpecTimeout(3*time.Second))

	It("says so when the run is deleted while you watch it", func() {
		ci.progress("ci-shop", ciRun("api-tests-x7k2p", time.Now()).flow("api-tests").running(), nil)

		err := run("workflow", "watch", "api-tests-x7k2p", "-a", "shop", "-r", "north1", "-i", "1ms")

		Expect(err).To(MatchError("shop has no run api-tests-x7k2p in north1; see ccloud workflow list -a shop -r north1"))
	})

	It("waits for a run still going, then says how it ended", func() {
		started := time.Now().Add(-time.Minute)
		ci.progress("ci-shop",
			ciRun("api-tests-x7k2p", started).flow("api-tests").running(),
			ciRun("api-tests-x7k2p", started).flow("api-tests").running(),
			ciRun("api-tests-x7k2p", started).flow("api-tests").finished("Failed", time.Minute),
		)

		Expect(run("workflow", "watch", "api-tests-x7k2p", "-a", "shop", "-r", "north1", "-i", "1ms")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements(
			"X api-tests",
			"api-tests-x7k2p started 1m ago and took 1m0s",
			"",
			"X api-tests-x7k2p completed: Failed",
		))
	})
})
