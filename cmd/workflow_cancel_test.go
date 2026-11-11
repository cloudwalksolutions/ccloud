package cmd_test

import (
	"bytes"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud workflow cancel", func() {
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

	It("leaves a run that already finished alone, and says how it ended", func() {
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now().Add(-time.Hour)).flow("api-tests").finished("Failed", time.Minute))

		err := run("workflow", "cancel", "api-tests-x7k2p", "-a", "shop", "-r", "north1")

		Expect(err).To(MatchError("api-tests-x7k2p already finished: Failed"))
		Expect(ci.writes).To(BeEmpty())
	})

	It("says so when the cluster does not let you change the app's runs", func() {
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now().Add(-time.Minute)).flow("api-tests").running())
		ci.readOnly("ci-shop")

		err := run("workflow", "cancel", "api-tests-x7k2p", "-a", "shop", "-r", "north1")

		Expect(err).To(MatchError("you may not cancel or rerun shop's runs in north1 (ci-shop)"))
	})

	It("stops a run still going, letting its exit handlers report", func() {
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now().Add(-time.Minute)).flow("api-tests").running())

		Expect(run("workflow", "cancel", "api-tests-x7k2p", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(ci.writes).To(HaveLen(1))
		Expect(ci.writes[0].method).To(Equal("PATCH"))
		Expect(ci.writes[0].contentType).To(Equal("application/merge-patch+json"))
		Expect(ci.writes[0].body).To(Equal(map[string]any{"spec": map[string]any{"shutdown": "Stop"}}))
		Expect(out.String()).To(Equal("Cancelling api-tests-x7k2p. Follow it with: ccloud workflow watch api-tests-x7k2p -a shop -r north1\n"))
	})
})
