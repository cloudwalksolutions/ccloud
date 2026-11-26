package cmd_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud db backup", func() {
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
	})

	It("refuses a note with a quote, backslash or line break in it, before backing anything up", func() {
		err := run("db", "backup", "-e", "stage", "-a", "acme", "-r", "north1", "--note", `the "big" move`)

		Expect(err).To(MatchError(`a note may not hold quotes, backslashes or line breaks: "the \"big\" move"`))
		Expect(cluster.writes).To(BeEmpty())
	})

	It("says so when you may not back the environment up, as with stage and prod for their developers", func() {
		cluster.readOnly("stage-acme")

		err := run("db", "backup", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("you may not back up acme's stage database in north1"))
	})

	It("fails when the backup does, so nobody counts on a backup that is not there", func() {
		cluster.workflows("stage-acme", ciRun("db-backup-r3run", time.Now().Add(-time.Minute)).finished("Failed", 30*time.Second))

		err := run("db", "backup", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("db-backup-r3run did not succeed: Failed"))
	})

	It("backs up with your note, follows it until it is off-site, then shows the backup it took", func() {
		cluster.workflows("stage-acme", ciRun("db-backup-r3run", time.Now().Add(-time.Minute)).from("db-backup").finished("Succeeded", 30*time.Second))
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		cluster.exec("stage-acme/db-0", pgbackrestInfo(
			backup("20261126-174214F", time.Now().Add(-2*time.Hour), 7_900_000, "the first"),
			backup("20261126-175333F", time.Now().Add(-20*time.Second), 8_400_000, "before the move"),
		))

		Expect(run("db", "backup", "-e", "stage", "-a", "acme", "-r", "north1", "--note", "before the move")).To(Succeed())

		Expect(cluster.writes[0].body["spec"]).To(Equal(map[string]any{
			"workflowTemplateRef": map[string]any{"name": "db-backup"},
			"arguments":           map[string]any{"parameters": []any{map[string]any{"name": "note", "value": "before the move"}}},
		}))
		Expect(cluster.execs).To(Equal([]string{"stage-acme/db-0 database: pgbackrest info --stanza=db --output=json"}))
		Expect(lines(out)[0]).To(Equal("✓ db-backup"))
		Expect(lines(out)).To(ContainElements(
			"Backed up acme's stage database in north1, and copied it off-site:",
			"  label     20261126-175333F",
			"  finished  just now",
			"  size      8Mi",
			"  note      before the move",
		))
	})

	It("says so when the database has no primary to read the backups from", func() {
		cluster.workflows("stage-acme", ciRun("db-backup-r3run", time.Now().Add(-time.Minute)).finished("Succeeded", 30*time.Second))

		err := run("db", "backup", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("acme's stage database has no primary running in north1"))
	})

	It("passes on what pgbackrest says when it cannot read the backups", func() {
		cluster.workflows("stage-acme", ciRun("db-backup-r3run", time.Now().Add(-time.Minute)).finished("Succeeded", 30*time.Second))
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		cluster.execFails("stage-acme/db-0", "ERROR: [055]: unable to load info file")

		err := run("db", "backup", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError(ContainSubstring("ERROR: [055]: unable to load info file")))
	})

	It("says so when pgbackrest answers something other than its JSON", func() {
		cluster.workflows("stage-acme", ciRun("db-backup-r3run", time.Now().Add(-time.Minute)).finished("Succeeded", 30*time.Second))
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		cluster.exec("stage-acme/db-0", "stanza: db")

		err := run("db", "backup", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError(ContainSubstring("pgbackrest info answered something other than JSON")))
	})

	It("cautions you before backing up prod, which runs against the live database", func() {
		cluster.workflows("prod-acme", ciRun("db-backup-r3run", time.Now().Add(-time.Minute)).finished("Succeeded", 30*time.Second))
		cluster.pod("prod-acme", "db-0", map[string]string{"role": "primary"})
		cluster.exec("prod-acme/db-0", pgbackrestInfo(backup("20261126-175333F", time.Now(), 8_400_000, "")))

		Expect(run("db", "backup", "-e", "prod", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(errs.String()).To(HavePrefix("CAUTION: this is acme's production database, and its only primary."))
	})
})

func pgbackrestInfo(backups ...map[string]any) string {
	info, err := json.Marshal([]any{map[string]any{"name": "db", "backup": backups}})
	Expect(err).NotTo(HaveOccurred())
	return string(info)
}

func backup(label string, stopped time.Time, stored int, note string) map[string]any {
	return map[string]any{
		"label":      label,
		"type":       "full",
		"timestamp":  map[string]any{"start": stopped.Add(-15 * time.Second).Unix(), "stop": stopped.Unix()},
		"info":       map[string]any{"size": stored * 6, "repository": map[string]any{"size": stored}},
		"annotation": map[string]any{"note": note},
	}
}
