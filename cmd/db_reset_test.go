package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud db reset", func() {
	var (
		out     *bytes.Buffer
		cluster *fakeCluster
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, example, "dev")
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		cluster = newFakeCluster()
		cluster.signIn("north1")
	})

	It("sends you to sign in when you have not signed in to the region", func() {
		err := run("db", "reset", "-e", "feat-x", "-a", "shop", "-r", "south1", "--dangerously-reset")

		Expect(err).To(MatchError("you have not signed in to south1 yet; run ccloud clusters auth --region south1"))
	})

	It("refuses to reset without confirming when nobody is at a terminal to confirm", func() {
		err := run("db", "reset", "-e", "feat-x", "-a", "shop", "-r", "north1")

		Expect(err).To(MatchError("resetting drops shop's feat-x database in north1; pass --dangerously-reset to reset it without confirming"))
		Expect(cluster.writes).To(BeEmpty())
	})

	It("resets nothing when what you type is not the environment's name", func() {
		atTerminal("stage\n")

		err := run("db", "reset", "-e", "feat-x", "-a", "shop", "-r", "north1")

		Expect(out.String()).To(Equal("This drops every table in shop's feat-x database in north1, then migrates and seeds it from scratch.\nType feat-x to reset it: "))
		Expect(err).To(MatchError(`you typed "stage", not "feat-x"; nothing was reset`))
		Expect(cluster.writes).To(BeEmpty())
	})

	It("resets once you type the environment's name", func() {
		atTerminal("feat-x\n")
		cluster.workflows("feat-x-shop", ciRun("db-reset-r3run", time.Now().Add(-time.Minute)).finished("Succeeded", 30*time.Second))

		Expect(run("db", "reset", "-e", "feat-x", "-a", "shop", "-r", "north1")).To(Succeed())

		Expect(cluster.writes).To(HaveLen(1))
	})

	It("fails when the reset does, so whoever ran it knows the database is not ready", func() {
		cluster.workflows("feat-x-shop", ciRun("db-reset-r3run", time.Now().Add(-time.Minute)).finished("Failed", 30*time.Second))

		err := run("db", "reset", "-e", "feat-x", "-a", "shop", "-r", "north1", "--dangerously-reset")

		Expect(err).To(MatchError("db-reset-r3run did not succeed: Failed"))
	})

	It("says so when you may not reset the environment, as with stage for its developers and prod for anyone", func() {
		cluster.readOnly("stage-shop")

		err := run("db", "reset", "-e", "stage", "-a", "shop", "-r", "north1", "--dangerously-reset")

		Expect(err).To(MatchError("you may not reset shop's stage database in north1"))
	})

	It("runs the environment's own reset and follows it until the database is migrated and seeded again", func() {
		cluster.workflows("feat-x-shop", ciRun("db-reset-r3run", time.Now().Add(-time.Minute)).finished("Succeeded", 30*time.Second))

		Expect(run("db", "reset", "-e", "feat-x", "-a", "shop", "-r", "north1", "--dangerously-reset")).To(Succeed())

		Expect(cluster.writes).To(HaveLen(1))
		Expect(cluster.writes[0].body).To(Equal(map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Workflow",
			"metadata":   map[string]any{"generateName": "db-reset-"},
			"spec":       map[string]any{"workflowTemplateRef": map[string]any{"name": "db-reset"}},
		}))
		Expect(lines(out)).To(ContainElement("✓ db-reset-r3run completed: Succeeded"))
	})
})

func atTerminal(typed string) {
	GinkgoT().Setenv("CCLOUD_FORCE_TTY", "1")
	GinkgoT().Setenv("NO_COLOR", "1")
	keyboard, typing, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	_, err = typing.WriteString(typed)
	Expect(err).NotTo(HaveOccurred())
	Expect(typing.Close()).To(Succeed())
	stdin := os.Stdin
	os.Stdin = keyboard
	DeferCleanup(func() {
		os.Stdin = stdin
		_ = keyboard.Close()
	})
}
