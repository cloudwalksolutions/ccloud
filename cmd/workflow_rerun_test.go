package cmd_test

import (
	"bytes"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud workflow rerun", func() {
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

	It("says so when the cluster does not let you start the app's runs", func() {
		ci.workflows("ci-shop", ciRun("api-tests-x7k2p", time.Now().Add(-time.Hour)).flow("api-tests").finished("Failed", time.Minute))
		ci.readOnly("ci-shop")

		err := run("workflow", "rerun", "api-tests-x7k2p", "-a", "shop", "-r", "north1")

		Expect(err).To(MatchError("you may not cancel or rerun shop's runs in north1 (ci-shop)"))
	})

	It("starts a new run of the same flow, for the same pull request and commit, without Argo's own bookkeeping", func() {
		original := ciRun("api-tests-x7k2p", time.Now().Add(-time.Hour)).flow("api-tests").
			pr("105", "3f9a2c4e1d", "feat/BOAT-105/search").finished("Failed", time.Minute).
			label("workflows.argoproj.io/phase", "Failed").label("workflows.argoproj.io/completed", "true")
		original["metadata"].(map[string]any)["generateName"] = "api-tests-"
		original["spec"].(map[string]any)["workflowTemplateRef"] = map[string]any{"name": "api-tests"}
		original["spec"].(map[string]any)["shutdown"] = "Stop"
		ci.workflows("ci-shop", original)

		Expect(run("workflow", "rerun", "api-tests-x7k2p", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(ci.writes).To(HaveLen(1))
		Expect(ci.writes[0].method).To(Equal("POST"))
		Expect(ci.writes[0].body).To(Equal(map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata": map[string]any{
				"generateName": "api-tests-",
				"labels":       map[string]any{"flow": "api-tests", "pr": "105", "sha": "3f9a2c4e1d"},
			},
			"spec": map[string]any{
				"arguments":           map[string]any{"parameters": []any{map[string]any{"name": "revision", "value": "feat/BOAT-105/search"}}},
				"workflowTemplateRef": map[string]any{"name": "api-tests"},
			},
		}))
		Expect(out.String()).To(Equal("Started api-tests-r3run, a rerun of api-tests-x7k2p. Follow it with: ccloud workflow watch api-tests-r3run -a shop -r north1\n"))
	})
})
