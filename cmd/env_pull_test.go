package cmd_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

var _ = Describe("ccloud env pull", func() {
	var (
		out          *bytes.Buffer
		north, south *fakeGateway
		home         cloud.Cloud
		bin          string
	)

	run := func(args ...string) error {
		return cmd.Run(args, out, out, home, "dev")
	}

	in := func(dir string) {
		was, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(dir)).To(Succeed())
		DeferCleanup(os.Chdir, was)
	}

	serve := func(g *fakeGateway) string {
		server := httptest.NewServer(http.HandlerFunc(g.serve))
		DeferCleanup(server.Close)
		return server.URL
	}

	tool := func(name, script string) {
		Expect(os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script+"\n"), 0o755)).To(Succeed())
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		GinkgoT().Setenv("KUBECONFIG", filepath.Join(GinkgoT().TempDir(), "config"))
		north = &fakeGateway{hosts: []string{"ci", "stage"}}
		south = &fakeGateway{hosts: []string{"prod"}}
		home = example
		home.Regions = append([]cloud.Region{}, example.Regions...)
		home.Regions[0].Gateway = serve(north)
		home.Regions[1].Gateway = serve(south)
		bin = GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", bin)
		tool("cloudflared", `[ "$1 $2" = "access token" ] && echo access-jwt`)
		tool("kubectl-oidc_login", `echo '{"kind":"ExecCredential","status":{"token":"id-token"}}'`)
	})

	It("needs the app's repo, which is where the env file goes", func() {
		in(GinkgoT().TempDir())

		err := run("env", "pull", "-e", "feat-x", "-r", "north1")

		Expect(err).To(MatchError("run ccloud env pull inside the app's repo"))
	})

	DescribeTable("copies only a feature environment, whose secrets come from the dev project",
		func(env string) {
			in(acmeRepo())

			err := run("env", "pull", "-e", env, "-r", "north1")

			Expect(err).To(MatchError("env pull copies a feature environment, whose secrets come from the dev project; " + env + " is not one"))
		},
		Entry("not stage", "stage"),
		Entry("not prod", "prod"),
		Entry("not ci", "ci"),
	)

	It("says so when no feature environment is running to copy", func() {
		in(acmeRepo())

		err := run("env", "pull")

		Expect(err).To(MatchError("no feature environment of acme is running to copy; open a pull request, or name one with -e"))
	})

	It("asks which feature environment to copy when several are running", func() {
		north.hosts = []string{"ci", "feat-a", "feat-b"}
		in(acmeRepo())

		err := run("env", "pull")

		Expect(err).To(MatchError("acme has 2 feature environments running, feat-a and feat-b; name the one to copy with -e"))
	})

	Context("with a feature environment running", func() {
		var (
			repo    string
			secrets *fakeSecretManager
			cluster *fakeCluster
		)

		writeEnvFile := func(contents string) {
			Expect(os.MkdirAll(filepath.Join(repo, "src", "api"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(repo, "src", "api", ".env"), []byte(contents), 0o600)).To(Succeed())
		}

		envFile := func() string {
			written, err := os.ReadFile(filepath.Join(repo, "src", "api", ".env"))
			Expect(err).NotTo(HaveOccurred())
			return string(written)
		}

		BeforeEach(func() {
			north.hosts = []string{"ci", "stage", "feat-x"}
			repo = acmeRepo()
			in(repo)
			secrets = &fakeSecretManager{values: map[string]string{
				"acme-dev/nonprod-stripe-key": "sk_test_123",
				"acme-dev/nonprod-secret-key": "has space #hash",
			}}
			server := httptest.NewServer(http.HandlerFunc(secrets.serve))
			DeferCleanup(server.Close)
			home.Env.SecretManager = server.URL
			tool("gcloud", `[ "$*" = "auth application-default print-access-token" ] && echo adc-token`)

			cluster = newFakeCluster()
			cluster.signIn("north1")
			cluster.object("/apis/apps/v1/namespaces/feat-x-acme/deployments/web", map[string]any{
				"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "web"},
				"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{
					map[string]any{"name": "sidecar", "env": []any{secretEnv("SIDECAR_KEY", "app-secrets", "stripe-key")}},
					map[string]any{"name": "server", "env": []any{
						map[string]any{"name": "LIFECYCLE", "value": "feat-x"},
						secretEnv("STRIPE_KEY", "app-secrets", "stripe-key"),
						secretEnv("DB_PASSWORD", "crunchydb-pguser-crunchydb", "password"),
						secretEnv("SECRET_KEY", "app-secrets", "secret-key"),
					}},
				}}}},
			})
			cluster.object("/apis/external-secrets.io/v1/namespaces/feat-x-acme/externalsecrets", map[string]any{
				"apiVersion": "external-secrets.io/v1", "kind": "ExternalSecretList", "items": []any{map[string]any{
					"metadata": map[string]any{"name": "app-secrets"},
					"spec": map[string]any{
						"secretStoreRef": map[string]any{"kind": "SecretStore", "name": "secret-store"},
						"target":         map[string]any{"name": "app-secrets"},
						"data": []any{
							map[string]any{"secretKey": "stripe-key", "remoteRef": map[string]any{"key": "nonprod-stripe-key"}},
							map[string]any{"secretKey": "secret-key", "remoteRef": map[string]any{"key": "nonprod-secret-key"}},
						},
					},
				}},
			})
			cluster.object("/apis/external-secrets.io/v1/namespaces/feat-x-acme/secretstores/secret-store", map[string]any{
				"apiVersion": "external-secrets.io/v1", "kind": "SecretStore", "metadata": map[string]any{"name": "secret-store"},
				"spec": map[string]any{"provider": map[string]any{"gcpsm": map[string]any{"projectID": "acme-dev"}}},
			})
		})

		It("sends you to sign in to Google when you have no application default credentials", func() {
			tool("gcloud", `echo "ERROR: (gcloud.auth.application-default.print-access-token) Your default credentials were not found." >&2; exit 1`)

			err := run("env", "pull")

			Expect(err).To(MatchError("reading acme's secrets needs your Google sign-in; run gcloud auth application-default login"))
		})

		It("names the secrets you may not read, and writes none, when you are not one of the app's developers", func() {
			secrets.refused = map[string]bool{"acme-dev/nonprod-stripe-key": true}

			err := run("env", "pull")

			Expect(err).To(MatchError("you may not read acme's nonprod-stripe-key in acme-dev; ask to be added to acme's developers"))
			Expect(filepath.Join(repo, "src", "api", ".env")).NotTo(BeAnExistingFile())
		})

		It("names a secret the environment expects that its project lacks", func() {
			delete(secrets.values, "acme-dev/nonprod-secret-key")

			err := run("env", "pull")

			Expect(err).To(MatchError("reading nonprod-secret-key from acme-dev: secret manager answered 404"))
		})

		It("says so when Secret Manager cannot be reached", func() {
			home.Env.SecretManager = "http://127.0.0.1:1"

			err := run("env", "pull")

			Expect(err).To(MatchError(ContainSubstring("reading nonprod-stripe-key from acme-dev: ")))
		})

		It("copies the feature environment you name, wherever it runs", func() {
			north.hosts = []string{"feat-x", "feat-y"}

			Expect(run("env", "pull", "-e", "feat-x")).To(Succeed())

			Expect(out.String()).To(HavePrefix("src/api/.env, from acme's feat-x in north1:\n"))
		})

		It("sends you to sign in to the region running the environment", func() {
			err := run("env", "pull", "-e", "feat-x", "-r", "south1")

			Expect(err).To(MatchError("you have not signed in to south1 yet; run ccloud clusters auth --region south1"))
		})

		It("says so when the environment runs no API to copy", func() {
			err := run("env", "pull", "-e", "feat-z", "-r", "north1")

			Expect(err).To(MatchError("acme's feat-z in north1 runs no web to copy"))
		})

		It("names the secret store it could not find, rather than blaming the API", func() {
			delete(cluster.objects, "/apis/external-secrets.io/v1/namespaces/feat-x-acme/secretstores/secret-store")

			err := run("env", "pull")

			Expect(err).To(MatchError(HavePrefix("reading secret store secret-store: ")))
		})

		It("says so when it cannot ask where the app's environments run", func() {
			south.refuse = http.StatusBadGateway

			err := run("env", "pull")

			Expect(err).To(MatchError(HavePrefix("asking south1 where acme runs")))
		})

		It("writes the secrets the environment's API gets, under the names it gets them, into a new env file", func() {
			Expect(run("env", "pull")).To(Succeed())

			Expect(envFile()).To(Equal("STRIPE_KEY=sk_test_123\nSECRET_KEY=\"has space #hash\"\n"))
			Expect(out.String()).To(Equal("src/api/.env, from acme's feat-x in north1:\n  added    STRIPE_KEY\n  added    SECRET_KEY\n"))
		})

		It("writes to the env file you name, from the repo's root, when your API reads another", func() {
			Expect(os.MkdirAll(filepath.Join(repo, "src", "web"), 0o755)).To(Succeed())
			in(filepath.Join(repo, "src", "web"))

			Expect(run("env", "pull", "--file", "server/.env.local")).To(Succeed())

			written, err := os.ReadFile(filepath.Join(repo, "server", ".env.local"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written)).To(HavePrefix("STRIPE_KEY=sk_test_123\n"))
			Expect(out.String()).To(HavePrefix("server/.env.local, from acme's feat-x in north1:\n"))
		})

		It("adds only what your env file lacks, leaving your own settings and notes as they are", func() {
			writeEnvFile("# local\nDB_HOST=localhost\nSECRET_KEY=\"has space #hash\"\n")

			Expect(run("env", "pull")).To(Succeed())

			Expect(envFile()).To(Equal("# local\nDB_HOST=localhost\nSECRET_KEY=\"has space #hash\"\nSTRIPE_KEY=sk_test_123\n"))
			Expect(out.String()).To(Equal("src/api/.env, from acme's feat-x in north1:\n  added    STRIPE_KEY\n1 unchanged.\n"))
		})

		It("says your env file already matches when it has every value", func() {
			writeEnvFile("STRIPE_KEY=sk_test_123\nSECRET_KEY=\"has space #hash\"\n")

			Expect(run("env", "pull")).To(Succeed())

			Expect(out.String()).To(Equal("src/api/.env already matches acme's feat-x in north1.\n"))
		})

		It("starts its additions on their own line when your file's last line has no newline", func() {
			writeEnvFile("DB_HOST=localhost")

			Expect(run("env", "pull")).To(Succeed())

			Expect(envFile()).To(HavePrefix("DB_HOST=localhost\nSTRIPE_KEY=sk_test_123\n"))
		})

		It("changes nothing when a value differs, showing the start of each so you can tell which to keep", func() {
			writeEnvFile("STRIPE_KEY=sk_live_999\n")

			err := run("env", "pull")

			Expect(err).To(MatchError("src/api/.env differs from acme's feat-x; pass --force to replace:\n  STRIPE_KEY  sk_l… → sk_t…"))
			Expect(envFile()).To(Equal("STRIPE_KEY=sk_live_999\n"))
		})

		It("replaces values that differ where they stand, when you force it", func() {
			writeEnvFile("# mine\nSTRIPE_KEY=sk_live_999\nOTHER=1\n")

			Expect(run("env", "pull", "--force")).To(Succeed())

			Expect(envFile()).To(Equal("# mine\nSTRIPE_KEY=sk_test_123\nOTHER=1\nSECRET_KEY=\"has space #hash\"\n"))
			Expect(out.String()).To(Equal("src/api/.env, from acme's feat-x in north1:\n  added    SECRET_KEY\n  changed  STRIPE_KEY\n"))
		})
	})
})

func secretEnv(name, secret, key string) map[string]any {
	return map[string]any{"name": name, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": secret, "key": key}}}
}

type fakeSecretManager struct {
	values  map[string]string
	refused map[string]bool
}

func (m *fakeSecretManager) serve(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/projects/"), "/")
	name := parts[0] + "/" + parts[2]
	value, ok := m.values[name]
	switch {
	case r.Header.Get("Authorization") != "Bearer adc-token":
		http.Error(w, `{"error":{"code":401,"status":"UNAUTHENTICATED"}}`, http.StatusUnauthorized)
	case m.refused[name]:
		http.Error(w, `{"error":{"code":403,"status":"PERMISSION_DENIED"}}`, http.StatusForbidden)
	case !ok:
		http.Error(w, `{"error":{"code":404,"status":"NOT_FOUND"}}`, http.StatusNotFound)
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"payload": map[string]any{"data": base64.StdEncoding.EncodeToString([]byte(value))}})
	}
}

func acmeRepo() string {
	root := GinkgoT().TempDir()
	Expect(os.MkdirAll(filepath.Join(root, ".ccloud"), 0o755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(root, ".ccloud", "app.yaml"), []byte("app: acme\n"), 0o600)).To(Succeed())
	return root
}
