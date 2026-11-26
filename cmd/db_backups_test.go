package cmd_test

import (
	"bytes"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud db backups", func() {
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

	It("says so when the environment has no backups yet", func() {
		cluster.exec("stage-acme/db-0", pgbackrestInfo())

		Expect(run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(out.String()).To(Equal("acme's stage database in north1 has no backups yet; ccloud db backup takes one.\n"))
	})

	It("says so when it cannot tell which backups are off-site, as when the nightly copy cannot be read", func() {
		cluster.exec("stage-acme/db-0", pgbackrestInfo(backup("20261126-175333F", time.Now(), 8_400_000, "")))

		err := run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError(HavePrefix("cannot tell which of acme's stage backups in north1 are off-site: ")))
	})

	It("says so when you may not read the backups, as developers may not in stage and prod", func() {
		cluster.readOnly("stage-acme")

		err := run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("you may not read acme's stage backups in north1"))
	})

	It("says so when you may not fetch another region's backups", func() {
		cluster.readOnly("stage-acme")

		err := run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1", "--from", "south1")

		Expect(err).To(MatchError("you may not read acme's stage backups in north1"))
	})

	It("names the regions there are when --from is not one of them, before fetching anything", func() {
		err := run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1", "--from", "west1")

		Expect(err).To(MatchError(`no region "west1"; example has north1, south1`))
		Expect(cluster.writes).To(BeEmpty())
	})

	It("fetches another region's off-site copy, then lists the backups in it, all of them off-site", func() {
		cluster.workflows("stage-acme", ciRun("db-fetch-r3run", time.Now().Add(-time.Minute)).from("db-fetch").finished("Succeeded", 30*time.Second))
		cluster.exec("stage-acme/db-0", pgbackrestInfo(backup("20261126-175333F", time.Now().Add(-20*time.Minute), 8_400_000, "before the move")))

		Expect(run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1", "--from", "south1")).To(Succeed())

		Expect(cluster.writes[0].body["spec"]).To(Equal(map[string]any{
			"workflowTemplateRef": map[string]any{"name": "db-fetch"},
			"arguments":           map[string]any{"parameters": []any{map[string]any{"name": "from", "value": "south1"}}},
		}))
		Expect(cluster.execs).To(Equal([]string{"stage-acme/db-0 database: pgbackrest info --stanza=db --output=json --repo2-path=/restore/south1/stage-acme/crunchydb/repo2"}))
		Expect(lines(out)[1:]).To(Equal([]string{"20261126-175333F  full  20m ago   8Mi   before the move  yes"}))
	})

	It("lists the backups newest first, with those the nightly copy has taken off-site", func() {
		cluster.exec("stage-acme/db-0", pgbackrestInfo(
			backup("20261124-030000F", time.Now().Add(-50*time.Hour), 7_900_000, ""),
			backup("20261126-175333F", time.Now().Add(-20*time.Minute), 8_400_000, "before the move"),
		))
		cluster.object("/apis/batch/v1/namespaces/s3/jobs", offsiteJobs(
			offsiteJob("offsite-29925120", "Complete", time.Now().Add(-48*time.Hour)),
			offsiteJob("offsite-29928000", "Failed", time.Now().Add(-12*time.Hour)),
			offsiteJob("snapshot-29928000", "Complete", time.Now().Add(-time.Minute)).ownedBy("snapshot"),
		))

		Expect(run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(lines(out)).To(Equal([]string{
			"LABEL             TYPE  FINISHED  SIZE  NOTE             OFF-SITE",
			"20261126-175333F  full  20m ago   8Mi   before the move  not yet",
			"20261124-030000F  full  2d ago    8Mi                    yes",
		}))
	})

	It("counts a backup off-site once the backup workflow that took it has copied it there", func() {
		cluster.exec("stage-acme/db-0", pgbackrestInfo(
			backup("20261126-175333F", time.Now().Add(-20*time.Minute), 8_400_000, "before the move"),
			backup("20261126-181002F", time.Now().Add(-5*time.Minute), 8_400_000, ""),
		))
		cluster.object("/apis/batch/v1/namespaces/s3/jobs", offsiteJobs())
		cluster.workflows("stage-acme",
			ciRun("db-backup-r3run", time.Now().Add(-21*time.Minute)).from("db-backup").finished("Succeeded", 2*time.Minute),
			ciRun("db-backup-f4il", time.Now().Add(-6*time.Minute)).from("db-backup").finished("Failed", 2*time.Minute),
			ciRun("db-reset-r3run", time.Now().Add(-4*time.Minute)).from("db-reset").finished("Succeeded", time.Minute),
		)

		Expect(run("db", "backups", "-e", "stage", "-a", "acme", "-r", "north1")).To(Succeed())

		Expect(lines(out)[1:]).To(Equal([]string{
			"20261126-181002F  full  5m ago    8Mi                    not yet",
			"20261126-175333F  full  20m ago   8Mi   before the move  yes",
		}))
	})
})

type job map[string]any

func offsiteJobs(jobs ...job) map[string]any {
	return map[string]any{"apiVersion": "batch/v1", "kind": "JobList", "metadata": map[string]any{}, "items": jobs}
}

func offsiteJob(name, condition string, started time.Time) job {
	return job{
		"metadata": map[string]any{"name": name, "namespace": "s3"},
		"status": map[string]any{
			"startTime":  started.UTC().Format(time.RFC3339),
			"conditions": []any{map[string]any{"type": condition, "status": "True"}},
		},
	}.ownedBy("offsite")
}

func (j job) ownedBy(cronJob string) job {
	j["metadata"].(map[string]any)["ownerReferences"] = []any{map[string]any{"apiVersion": "batch/v1", "kind": "CronJob", "name": cronJob, "uid": "u1"}}
	return j
}
