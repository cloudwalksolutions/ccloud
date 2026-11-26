package cmd_test

import (
	"bytes"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud db restore", func() {
	var (
		out, errs *bytes.Buffer
		cluster   *fakeCluster
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, errs, example, "dev")
	}

	BeforeEach(func() {
		out, errs = &bytes.Buffer{}, &bytes.Buffer{}
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		cluster = newFakeCluster()
		cluster.signIn("north1")
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
	})

	It("refuses to restore without confirming when nobody is at a terminal to confirm, before reading or fetching anything", func() {
		err := run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1", "--from", "south1")

		Expect(err).To(MatchError("restoring replaces acme's stage database in north1; pass --dangerously-restore to restore it without confirming"))
		Expect(cluster.writes).To(BeEmpty())
		Expect(cluster.execs).To(BeEmpty())
	})

	It("restores nothing when what you type is not the environment's name", func() {
		atTerminal("prod\n")

		err := run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1", "--backup", "20261126-175333F")

		Expect(out.String()).To(Equal("This replaces acme's stage database in north1 with backup 20261126-175333F; whatever was written since then is lost.\nType stage to restore it: "))
		Expect(err).To(MatchError(`you typed "prod", not "stage"; nothing was restored`))
		Expect(cluster.writes).To(BeEmpty())
	})

	It("restores the backup you name, follows it until the app is back on it, then says what it restored", func() {
		cluster.workflows("stage-acme", ciRun("db-restore-r3run", time.Now().Add(-time.Minute)).from("db-restore").finished("Succeeded", 30*time.Second))

		Expect(run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1", "--backup", "20261126-175333F", "--dangerously-restore")).To(Succeed())

		Expect(cluster.writes).To(HaveLen(1))
		Expect(cluster.writes[0].body["spec"]).To(Equal(map[string]any{
			"workflowTemplateRef": map[string]any{"name": "db-restore"},
			"arguments":           map[string]any{"parameters": []any{map[string]any{"name": "backup", "value": "20261126-175333F"}}},
		}))
		Expect(lines(out)).To(ContainElement("Restored acme's stage database in north1 from backup 20261126-175333F."))
	})

	It("says so when you may not restore the environment, as with stage and prod for their developers", func() {
		cluster.readOnly("stage-acme")

		Expect(run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1", "--backup", "20261126-175333F", "--dangerously-restore")).
			To(MatchError("you may not restore acme's stage database in north1"))
		Expect(run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1", "--dangerously-restore")).
			To(MatchError("you may not restore acme's stage database in north1"))
	})

	It("cautions you before restoring prod, which replaces the live database", func() {
		cluster.workflows("prod-acme", ciRun("db-restore-r3run", time.Now().Add(-time.Minute)).from("db-restore").finished("Succeeded", 30*time.Second))

		Expect(run("db", "restore", "-e", "prod", "-a", "acme", "-r", "north1", "--backup", "20261126-175333F", "--dangerously-restore")).To(Succeed())

		Expect(errs.String()).To(HavePrefix("CAUTION: this is acme's production database, and its only primary."))
	})

	It("says so when there is no backup to restore", func() {
		cluster.exec("stage-acme/db-0", pgbackrestInfo())

		err := run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1", "--dangerously-restore")

		Expect(err).To(MatchError("acme's stage database in north1 has no backups to restore; ccloud db backup takes one"))
		Expect(cluster.writes).To(BeEmpty())
	})

	It("fetches another region's off-site backups and restores the newest of them, as when moving the environment here", func() {
		cluster.workflows("stage-acme",
			ciRun("db-fetch-r3run", time.Now().Add(-2*time.Minute)).from("db-fetch").finished("Succeeded", 30*time.Second),
			ciRun("db-restore-r3run", time.Now().Add(-time.Minute)).from("db-restore").finished("Succeeded", 30*time.Second),
		)
		cluster.exec("stage-acme/db-0", pgbackrestInfo(backup("20261126-175333F", time.Now().Add(-20*time.Minute), 8_400_000, "moving")))

		Expect(run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1", "--from", "south1", "--dangerously-restore")).To(Succeed())

		Expect(cluster.execs).To(Equal([]string{"stage-acme/db-0 database: pgbackrest info --stanza=db --output=json --repo2-path=/restore/south1/stage-acme/crunchydb/repo2"}))
		Expect(cluster.writes).To(HaveLen(2))
		Expect(cluster.writes[1].body["spec"]).To(Equal(map[string]any{
			"workflowTemplateRef": map[string]any{"name": "db-restore"},
			"arguments": map[string]any{"parameters": []any{
				map[string]any{"name": "backup", "value": "20261126-175333F"},
				map[string]any{"name": "from", "value": "south1"},
			}},
		}))
		Expect(lines(out)).To(ContainElement("Restored acme's stage database in north1 from backup 20261126-175333F, taken in south1."))
	})

	It("restores the newest backup when you name none, and says which before you confirm", func() {
		atTerminal("stage\n")
		cluster.exec("stage-acme/db-0", pgbackrestInfo(
			backup("20261124-030000F", time.Now().Add(-50*time.Hour), 7_900_000, ""),
			backup("20261126-175333F", time.Now().Add(-20*time.Minute), 8_400_000, "before the move"),
		))
		cluster.workflows("stage-acme", ciRun("db-restore-r3run", time.Now().Add(-time.Minute)).from("db-restore").finished("Succeeded", 30*time.Second))

		Expect(run("db", "restore", "-e", "stage", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(HavePrefix("This replaces acme's stage database in north1 with backup 20261126-175333F; whatever was written since then is lost.\n"))
		Expect(cluster.writes[0].body["spec"]).To(HaveKeyWithValue("arguments",
			map[string]any{"parameters": []any{map[string]any{"name": "backup", "value": "20261126-175333F"}}}))
	})
})
