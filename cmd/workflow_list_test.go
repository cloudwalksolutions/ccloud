package cmd_test

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud workflow list", func() {
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
	})

	It("asks which app's runs you want", func() {
		err := run("workflow", "list", "--region", "north1")

		Expect(err).To(MatchError(ContainSubstring("which app? ccloud workflow list --app <app>")))
	})

	It("sends you to sign in when you have not signed in to the region", func() {
		err := run("workflow", "list", "-a", "shop", "-r", "north1")

		Expect(err).To(MatchError(ContainSubstring("run ccloud clusters auth --region north1")))
	})

	It("says so when the cluster does not let you read the app's runs", func() {
		ci.forbid("ci-other")
		ci.signIn("north1")

		err := run("workflow", "list", "-a", "other", "-r", "north1")

		Expect(err).To(MatchError(`you may not read other's runs in north1 (ci-other)`))
	})

	It("says there are none rather than printing an empty table", func() {
		ci.signIn("north1")

		Expect(run("workflow", "list", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(Equal("No runs for shop in north1.\n"))
	})

	It("lists the app's runs newest first, with their result, pull request, branch and timing", func() {
		now := time.Now().Truncate(time.Second)
		ci.workflows("ci-shop",
			ciRun("release-m4k2p", now.Add(-3*time.Hour)).flow("release").finished("Succeeded", 4*time.Minute+12*time.Second),
			ciRun("api-tests-x7k2p", now.Add(-time.Hour)).flow("api-tests").pr("105", "3f9a2c4e1d", "feat/BOAT-105/search").finished("Failed", 2*time.Minute+3*time.Second),
			ciRun("build-images-q9w1z", now.Add(-5*time.Minute)).flow("build-images").pr("105", "3f9a2c4e1d", "feat/BOAT-105/search").running(),
		)
		ci.signIn("north1")

		Expect(run("workflow", "list", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements(
			MatchRegexp(`^STATUS\s+FLOW\s+PR\s+BRANCH\s+SHA\s+ID\s+ELAPSED\s+AGE$`),
			MatchRegexp(`^\*\s+build-images\s+#105\s+feat/BOAT-105/search\s+3f9a2c4\s+build-images-q9w1z\s+5m\ds\s+5m ago$`),
			MatchRegexp(`^X\s+api-tests\s+#105\s+feat/BOAT-105/search\s+3f9a2c4\s+api-tests-x7k2p\s+2m3s\s+1h ago$`),
			MatchRegexp(`^✓\s+release\s+release-m4k2p\s+4m12s\s+3h ago$`),
		))
	})

	It("colours each result when asked to, keeping the columns aligned", func() {
		GinkgoT().Setenv("CLICOLOR_FORCE", "1")
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now().Add(-time.Hour)).flow("api-tests").finished("Failed", time.Minute))
		ci.signIn("north1")

		Expect(run("workflow", "list", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(ContainSubstring("\x1b[31mX\x1b[0m"))
		plain := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(out.String(), "")
		header, row, _ := strings.Cut(plain, "\n")
		Expect(strings.Index(row, "api-tests")).To(Equal(strings.Index(header, "FLOW")))
	})

	It("narrows to one flow and pull request, and to the newest few", func() {
		now := time.Now()
		ci.workflows("ci-shop",
			ciRun("api-tests-old", now.Add(-3*time.Hour)).flow("api-tests").pr("105", "1111111", "feat/a").finished("Failed", time.Minute),
			ciRun("api-tests-new", now.Add(-2*time.Hour)).flow("api-tests").pr("105", "2222222", "feat/a").finished("Succeeded", time.Minute),
			ciRun("web-tests-new", now.Add(-time.Hour)).flow("web-tests").pr("105", "2222222", "feat/a").finished("Succeeded", time.Minute),
			ciRun("api-tests-other", now.Add(-time.Hour)).flow("api-tests").pr("106", "3333333", "feat/b").finished("Succeeded", time.Minute),
		)
		ci.signIn("north1")

		Expect(run("workflow", "list", "-a", "shop", "-r", "north1", "--flow", "api-tests", "--pr", "105", "-L", "1")).To(Succeed())

		Expect(lines(out)).To(HaveExactElements(HavePrefix("STATUS"), ContainSubstring("api-tests-new")))
	})
})
