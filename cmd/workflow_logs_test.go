package cmd_test

import (
	"bytes"
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

var _ = Describe("ccloud workflow logs", func() {
	var (
		out     *bytes.Buffer
		ci      *fakeCluster
		gateway *fakeGateway
		home    cloud.Cloud
		started time.Time
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, home, "dev")
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		ci = newFakeCluster()
		ci.signIn("north1")

		gateway = &fakeGateway{}
		server := httptest.NewServer(http.HandlerFunc(gateway.serve))
		DeferCleanup(server.Close)
		home = example
		home.Regions = append([]cloud.Region{}, example.Regions...)
		home.Regions[0].Logs = server.URL
		bin := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		for name, script := range map[string]string{
			"cloudflared":        `echo access-jwt`,
			"kubectl-oidc_login": `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`,
		} {
			Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
		}

		started = time.Now().Add(-10 * time.Minute).Truncate(time.Second)
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", started).flow("api-tests").finished("Failed", 2*time.Minute).
			step("api-tests-x7k2p-2", "unit", "Failed", started.Add(45*time.Second), time.Minute, "exit code 1").
			step("api-tests-x7k2p-1", "lint", "Succeeded", started.Add(time.Second), 41*time.Second, ""))
	})

	It("prints each step's log in the order the steps ran, each line named for its step", func() {
		ci.podLog("ci-shop", "api-tests-x7k2p-lint-1", "ok 12 files\n")
		ci.podLog("ci-shop", "api-tests-x7k2p-unit-2", "FAIL auth\nexit 1\n")

		Expect(run("workflow", "logs", "api-tests-x7k2p", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements("lint\tok 12 files", "unit\tFAIL auth", "unit\texit 1"))
	})

	It("prints a line however long it is, as a minified bundle or stats dump can be", func() {
		long := strings.Repeat("x", 100_000)
		ci.podLog("ci-shop", "api-tests-x7k2p-lint-1", long+"\nlast")
		ci.podLog("ci-shop", "api-tests-x7k2p-unit-2", "")

		Expect(run("workflow", "logs", "api-tests-x7k2p", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements("lint\t"+long, "lint\tlast"))
	})

	It("says why the gateway would not give a step's log once its pod is gone", func() {
		gateway.refuse = http.StatusForbidden

		err := run("workflow", "logs", "api-tests-x7k2p", "-a", "shop", "-r", "north1")

		Expect(err).To(MatchError("the logs gateway answered 403 Forbidden: dev@example.test may not read logs in prod-shop"))
	})

	It("passes over a step that never ran, which has no log anywhere", func() {
		ci.workflows("ci-shop", ciRun("release-m4k2p", started).flow("release").finished("Succeeded", time.Minute).
			step("release-m4k2p-1", "build", "Succeeded", started, 30*time.Second, "").
			step("release-m4k2p-2", "deploy-preview", "Skipped", time.Time{}, 0, "when 'false' evaluated false"))
		ci.podLog("ci-shop", "release-m4k2p-build-1", "built\n")

		Expect(run("workflow", "logs", "release-m4k2p", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements("build\tbuilt"))
		Expect(gateway.asked).To(BeEmpty())
	})

	It("prints only the failed steps with --failed", func() {
		ci.podLog("ci-shop", "api-tests-x7k2p-lint-1", "ok 12 files\n")
		ci.podLog("ci-shop", "api-tests-x7k2p-unit-2", "FAIL auth\n")

		Expect(run("workflow", "logs", "api-tests-x7k2p", "-a", "shop", "-r", "north1", "--failed")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements("unit\tFAIL auth"))
	})

	It("reads a step whose pod is gone from the logs gateway, over the time the step ran", func() {
		ci.podLog("ci-shop", "api-tests-x7k2p-lint-1", "ok 12 files\n")
		gateway.answers = []string{`{"entries":[
			{"time":"2026-10-26T07:33:21Z","pod":"api-tests-x7k2p-unit-2","container":"main","line":"FAIL auth"},
			{"time":"2026-10-26T07:33:22Z","pod":"api-tests-x7k2p-unit-2","container":"main","line":"exit 1"}]}`}

		Expect(run("workflow", "logs", "api-tests-x7k2p", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements("lint\tok 12 files", "unit\tFAIL auth", "unit\texit 1"))
		Expect(gateway.asked).To(HaveLen(1))
		query := gateway.asked[0].URL.Query()
		Expect(query["namespace"]).To(Equal([]string{"ci-shop"}))
		Expect(query.Get("pod")).To(Equal("api-tests-x7k2p-unit-2"))
		Expect(query.Get("container")).To(Equal("main"))
		Expect(query.Get("start")).To(Equal(started.Add(45 * time.Second).UTC().Format(time.RFC3339)))
		Expect(query.Get("end")).To(Equal(started.Add(106 * time.Second).UTC().Format(time.RFC3339)))
		Expect(query.Get("direction")).To(Equal("forward"))
		Expect(query.Get("limit")).To(Equal("5000"))
	})
})
