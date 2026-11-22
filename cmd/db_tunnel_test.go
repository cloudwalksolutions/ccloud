package cmd_test

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"regexp"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/cloudwalksolutions/ccloud/cmd"
)

var _ = Describe("ccloud db tunnel", func() {
	owner := map[string]string{"user": "acme", "password": "s3cr3t/pa55", "dbname": "acmedb"}

	var (
		out     *gbytes.Buffer
		cluster *fakeCluster
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, example, "dev")
	}

	open := func(args ...string) {
		ctx, closeTunnel := context.WithCancel(context.Background())
		closed := make(chan error, 1)
		go func() { closed <- cmd.RunContext(ctx, args, out, out, example, "dev") }()
		DeferCleanup(func() {
			closeTunnel()
			Eventually(closed).Should(Receive(BeNil()))
		})
	}

	freePort := func() string {
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = probe.Close() }()
		return strconv.Itoa(probe.Addr().(*net.TCPAddr).Port)
	}

	roundTrip := func(port string) string {
		conn, err := net.Dial("tcp", "localhost:"+port)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = conn.Close() }()
		_, err = conn.Write([]byte("SELECT 1"))
		Expect(err).NotTo(HaveOccurred())
		echoed := make([]byte, len("SELECT 1"))
		_, err = io.ReadFull(conn, echoed)
		Expect(err).NotTo(HaveOccurred())
		return string(echoed)
	}

	BeforeEach(func() {
		out = gbytes.NewBuffer()
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		GinkgoT().Setenv("HOME", GinkgoT().TempDir())
		GinkgoT().Setenv("XDG_CACHE_HOME", GinkgoT().TempDir())
		cluster = newFakeCluster()
		cluster.signIn("north1")
	})

	It("sends you to sign in when you have not signed in to the region", func() {
		err := run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "south1")

		Expect(err).To(MatchError("you have not signed in to south1 yet; run ccloud clusters auth --region south1"))
	})

	It("says so when you may not tunnel into the environment", func() {
		cluster.forbid("stage-acme")

		err := run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("you may not tunnel into acme's stage database in north1"))
	})

	It("cautions you about prod before anything connects, even when you may not tunnel into it", func() {
		cluster.forbid("prod-acme")

		err := run("db", "tunnel", "-e", "prod", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("you may not tunnel into acme's prod database in north1"))
		Expect(string(out.Contents())).To(Equal(
			"CAUTION: this is acme's production database, and its only primary.\n" +
				"  - Every query adds load to the live app: keep them small and indexed.\n" +
				"  - A write changes real data at once, and nothing here undoes it.\n" +
				"  - Reads show customers' personal data: select only what you need.\n"))
	})

	It("says so when the environment has no database", func() {
		err := run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("acme's stage has no database in north1"))
	})

	It("says so when the database has no primary running", func() {
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-replica-0", map[string]string{"role": "replica"})

		err := run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1")

		Expect(err).To(MatchError("acme's stage database has no primary running in north1"))
	})

	It("forwards a local port to the primary, and prints what a client needs with the password cut short", func() {
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-replica-0", map[string]string{"role": "replica"})
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		port := freePort()

		open("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "-p", port)

		Eventually(out).Should(gbytes.Say(
			"Tunneled to acme's stage database in north1; Ctrl-C closes it.\n" +
				"  host      localhost\n" +
				"  port      " + port + "\n" +
				"  database  acmedb\n" +
				"  user      acme\n" +
				"  password  s3cr…\n" +
				"  url       postgres://acme:s3cr…@localhost:" + port + "/acmedb\n"))
		Expect(roundTrip(port)).To(Equal("SELECT 1"))
		Expect(cluster.forwarded).To(Receive(Equal("stage-acme/db-0:5432")))
	})

	It("listens on the database's own port, or the next one free after it", func() {
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		if taken, err := net.Listen("tcp", "localhost:5432"); err == nil {
			DeferCleanup(taken.Close)
		}

		open("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1")

		Eventually(out).Should(gbytes.Say(`  port      (\d+)\n`))
		port := regexp.MustCompile(`port +(\d+)`).FindStringSubmatch(string(out.Contents()))[1]
		Expect(strconv.Atoi(port)).To(BeNumerically(">", 5432))
		Expect(roundTrip(port)).To(Equal("SELECT 1"))
	})

	It("says so when there is no tunnel in the background to stop", func() {
		err := run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "--stop")

		Expect(err).To(MatchError("no tunnel to acme's stage database is open in the background"))
	})

	It("keeps the tunnel open in the background, cautioning agents, until you stop it", func() {
		GinkgoT().Setenv("CCLOUD_TEST_RUNS_CLI", "1")
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		port := freePort()

		Expect(run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "-p", port, "--background")).To(Succeed())

		Expect(string(out.Contents())).To(Equal(
			"Tunneled to acme's stage database in north1; ccloud db tunnel -e stage -a acme --stop closes it.\n" +
				"  host      localhost\n" +
				"  port      " + port + "\n" +
				"  database  acmedb\n" +
				"  user      acme\n" +
				"  password  s3cr…\n" +
				"  url       postgres://acme:s3cr…@localhost:" + port + "/acmedb\n" +
				"\nAI agents: this reaches acme's real stage data. Review every write, and every read of sensitive data, with the user before running it.\n"))
		Expect(roundTrip(port)).To(Equal("SELECT 1"))

		out = gbytes.NewBuffer()
		Expect(run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "--stop")).To(Succeed())

		Expect(out).To(gbytes.Say("Closed the tunnel to acme's stage database.\n"))
		Eventually(func() error {
			conn, err := net.Dial("tcp", "localhost:"+port)
			if err == nil {
				_ = conn.Close()
			}
			return err
		}).Should(HaveOccurred())
	})

	It("refuses a port something else already listens on, rather than reporting its tunnel open", func() {
		GinkgoT().Setenv("CCLOUD_TEST_RUNS_CLI", "1")
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		taken, err := net.Listen("tcp", "localhost:0")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(taken.Close)
		port := strconv.Itoa(taken.Addr().(*net.TCPAddr).Port)

		err = run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "-p", port, "--background")

		Expect(err).To(MatchError("localhost:" + port + " is already in use; pick another port with --port"))
	})

	It("says so when the tunnel in the background closes before it opens", func() {
		GinkgoT().Setenv("CCLOUD_TEST_RUNS_CLI", "1")
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		cluster.readOnly("stage-acme")
		port := freePort()

		err := run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "-p", port, "--background")

		Expect(err).To(MatchError("the tunnel to acme's stage database exited before opening localhost:" + port + ": exit status 1; run it without --background to see why"))
	})

	It("keeps one tunnel to an environment in the background, not two", func() {
		GinkgoT().Setenv("CCLOUD_TEST_RUNS_CLI", "1")
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		Expect(run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "--background")).To(Succeed())
		DeferCleanup(run, "db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "--stop")

		err := run("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "--background")

		Expect(err).To(MatchError("a tunnel to acme's stage database is already open in the background; ccloud db tunnel -e stage -a acme --stop closes it"))
	})

	It("ends a background tunnel to prod with prod's caution, for the agent that reads it last", func() {
		GinkgoT().Setenv("CCLOUD_TEST_RUNS_CLI", "1")
		cluster.secret("prod-acme", "db-owner", owner)
		cluster.pod("prod-acme", "db-0", map[string]string{"role": "primary"})

		Expect(run("db", "tunnel", "-e", "prod", "-a", "acme", "-r", "north1", "--background")).To(Succeed())
		DeferCleanup(run, "db", "tunnel", "-e", "prod", "-a", "acme", "-r", "north1", "--stop")

		Expect(string(out.Contents())).To(HaveSuffix(
			"\nAI agents: this reaches acme's real prod data. Review every write, and every read of sensitive data, with the user before running it.\n" +
				"CAUTION: this is acme's production database, and its only primary.\n" +
				"  - Every query adds load to the live app: keep them small and indexed.\n" +
				"  - A write changes real data at once, and nothing here undoes it.\n" +
				"  - Reads show customers' personal data: select only what you need.\n"))
	})

	It("prints the whole password when asked for it, escaped in the URL", func() {
		cluster.secret("stage-acme", "db-owner", owner)
		cluster.pod("stage-acme", "db-0", map[string]string{"role": "primary"})
		port := freePort()

		open("db", "tunnel", "-e", "stage", "-a", "acme", "-r", "north1", "-p", port, "-s")

		Eventually(out).Should(gbytes.Say(
			"  password  s3cr3t/pa55\n" +
				"  url       postgres://acme:s3cr3t%2Fpa55@localhost:" + port + "/acmedb\n"))
	})
})
