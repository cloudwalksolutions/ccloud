package cmd_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

var _ = Describe("a newer ccloud", func() {
	var (
		releases *httptest.Server
		home     cloud.Cloud
		asked    int
	)

	BeforeEach(func() {
		asked = 0
		releases = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			asked++
			_, _ = w.Write([]byte(`{"tag_name":"v0.0.18","html_url":"https://releases.example.test/v0.0.18"}`))
		}))
		DeferCleanup(releases.Close)
		home = example
		home.Releases = releases.URL
		GinkgoT().Setenv("HOME", GinkgoT().TempDir())
		GinkgoT().Setenv("XDG_CACHE_HOME", GinkgoT().TempDir())
	})

	It("is named under the version, with where to get it", func() {
		out := &bytes.Buffer{}

		Expect(cmd.Run([]string{"--version"}, out, out, home, "0.0.10 (commit acba18d, built 2026-11-11)")).To(Succeed())

		Expect(out.String()).To(Equal("ccloud version 0.0.10 (commit acba18d, built 2026-11-11)\n" +
			"ccloud 0.0.18 is out (you have 0.0.10): https://releases.example.test/v0.0.18\n"))
	})

	It("is not named when you already run it, or something later", func() {
		for _, running := range []string{"0.0.18", "0.0.19", "0.1.0"} {
			out := &bytes.Buffer{}

			Expect(cmd.Run([]string{"--version"}, out, out, home, running+" (commit acba18d, built 2026-11-18)")).To(Succeed())

			Expect(out.String()).To(Equal("ccloud version " + running + " (commit acba18d, built 2026-11-18)\n"))
		}
	})

	It("is never asked for by a build that is not a release", func() {
		out := &bytes.Buffer{}

		Expect(cmd.Run([]string{"--version"}, out, out, home, "dev (commit none, built unknown)")).To(Succeed())

		Expect(out.String()).To(Equal("ccloud version dev (commit none, built unknown)\n"))
		Expect(asked).To(BeZero())
	})

	It("is named on stderr after any other command, asking at most once a day", func() {
		first, second := &bytes.Buffer{}, &bytes.Buffer{}

		Expect(cmd.Run([]string{"clusters"}, &bytes.Buffer{}, first, home, "0.0.10 (commit acba18d, built 2026-11-11)")).To(Succeed())
		Expect(cmd.Run([]string{"clusters"}, &bytes.Buffer{}, second, home, "0.0.10 (commit acba18d, built 2026-11-11)")).To(Succeed())

		Expect(first.String()).To(Equal("ccloud 0.0.18 is out (you have 0.0.10): https://releases.example.test/v0.0.18\n"))
		Expect(second.String()).To(BeEmpty())
		Expect(asked).To(Equal(1))
	})

	It("is never asked for when CCLOUD_NO_UPDATE_CHECK is set, and nothing is written to the cache", func() {
		GinkgoT().Setenv("CCLOUD_NO_UPDATE_CHECK", "1")
		errOut := &bytes.Buffer{}

		Expect(cmd.Run([]string{"clusters"}, &bytes.Buffer{}, errOut, home, "0.0.10 (commit acba18d, built 2026-11-11)")).To(Succeed())

		Expect(errOut.String()).To(BeEmpty())
		Expect(asked).To(BeZero())
		cache, err := os.UserCacheDir()
		Expect(err).NotTo(HaveOccurred())
		Expect(filepath.Join(cache, "ccloud")).NotTo(BeADirectory())
	})

	It("is not asked for after a command when there is no cache to remember the last check in", func() {
		GinkgoT().Setenv("HOME", "")
		GinkgoT().Setenv("XDG_CACHE_HOME", "")

		Expect(cmd.Run([]string{"clusters"}, &bytes.Buffer{}, &bytes.Buffer{}, home, "0.0.10 (commit acba18d, built 2026-11-11)")).To(Succeed())

		Expect(asked).To(BeZero())
	})

	It("stays quiet when the releases cannot be read", func() {
		garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "<html>rate limited</html>", http.StatusForbidden)
		}))
		DeferCleanup(garbage.Close)
		for _, releasesURL := range []string{garbage.URL, "http://127.0.0.1:1/releases"} {
			home.Releases = releasesURL
			out := &bytes.Buffer{}

			Expect(cmd.Run([]string{"--version"}, out, out, home, "0.0.10 (commit acba18d, built 2026-11-11)")).To(Succeed())

			Expect(out.String()).To(Equal("ccloud version 0.0.10 (commit acba18d, built 2026-11-11)\n"))
		}
	})
})
