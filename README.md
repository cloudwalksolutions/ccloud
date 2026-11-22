# ccloud

The developer CLI for your apps in CloudWalk Cloud, or your own Custom Cloud.

```sh
brew install --cask cloudwalksolutions/tap/ccloud
```

The cask brings `kubectl`, `cloudflared` and kubelogin with it.

`ccloud --version` says when a newer release is out, and any command says so at most once a day.
`CCLOUD_NO_UPDATE_CHECK=1` turns that off.

## Clusters

```sh
ccloud clusters auth --region us-central1
kubectl get pods -n prod-<app>
ccloud clusters set-context --region us-east1 --namespace ci-<app>
```

`clusters auth` signs you in through your cloud's Cloudflare Access and its OIDC provider in your
browser, then adds a `ccloud-<region>` context to your kubeconfig and switches you to it. What you
may do is decided by the cluster, never by ccloud.

`clusters set-context` switches between regions you have signed in to, and with `--namespace` sets
the namespace kubectl uses there.

## Inside an app repo

An app repo names its app in `.ccloud/app.yaml` (CloudWalk's repos use `.cloudwalk/app.yaml`):

```yaml
app: acme
```

Anywhere inside the repo, `--app` defaults to it, and the region is found for you: ccloud asks each
region's gateway which of the app's environments it hosts, and uses the one hosting `ci` for
workflows and the `-e` environment for logs. `clusters auth` with no `--region` signs in to every
region hosting the app. `-a` and `-r` still win when given.

## Workflows

```sh
ccloud workflow list --pr 105
ccloud workflow view api-tests-x7k2p
ccloud workflow watch api-tests-x7k2p --exit-status
ccloud workflow logs api-tests-x7k2p --failed
ccloud workflow rerun api-tests-x7k2p
ccloud workflow cancel build-images-q9w1z
```

Your app's CI runs, read through the cluster you signed in to with `clusters auth`. `list` shows
each run's result, flow, pull request, branch and timing, newest first; `view` shows a run's steps
in the order they ran, how long each took, and why one failed. `watch` redraws that view until the
run ends, and with `--exit-status` exits non-zero unless it succeeded. `logs` prints each step's
log as `step<TAB>line`, from its pod while it exists and from the region's gateway once it is gone.
`rerun` starts the whole run again for the same pull request and commit; `cancel` stops a run still
going, and its exit handlers still report.

## Logs

```sh
ccloud logs -e prod -l error --since 6h
ccloud logs -e stage -f -c api -c nginx
ccloud logs containers -e stage
ccloud logs -e prod -l error --since 10m -o json --fail-on-match
```

An app's logs in one environment (`prod`, `stage`, `ci`, or a feature environment's name), kept
for years, so a pod long gone still has them. The region's gateway ([docs/gateway.md](docs/gateway.md)) reads them as you: it asks
the cluster whether you may read that environment's pod logs, exactly as `kubectl logs` would.

Each line shows its time (with the date when not today), container and pod. JSON lines print as
`LEVEL message key=value`, and a line that runs over several is indented under its first. `-c`
picks containers and repeats; `ccloud logs containers` lists an environment's. `--since`,
`--since-time` and `--until` set the window, and a note says when `--tail` cut lines off.

`-f` follows the gateway's live stream until Ctrl-C, reconnecting on its own after a drop and
printing each line once. `-o raw` gives each line as stored; `-o json` gives one entry per line, with
its stream and level, then a `{"next": …}` line to resume from with `--since-time`.
`--fail-on-match` exits non-zero when any line matched, for scripts and agents. The first time,
`cloudflared` opens your browser to sign in to Cloudflare Access.

## Top

```sh
ccloud top
ccloud top -e prod
```

What an app uses. With no `-e`, one row per environment in every region: pods, CPU used against
requested, memory used against its limits, and how many containers and nodes are flagged. With
`-e`, one row per container, its node, and why it is flagged: throttled by its CPU limit 25% of the
time or more, at 90% of its memory limit, OOMKilled or restarted 3 times in 24 hours, or no CPU or
memory limit at all. A node under pressure is named apart, since that is the node, not the
container's limits.

## Net

```sh
ccloud net -e prod
ccloud net -e ci --since 24h
```

One environment's traffic over `--since` (an hour unless asked): its requests per second, share
of 5xx and p95; every internet host it connected to, by name where the cloud saw the lookup, with
how many connections got no answer or were reset; and every connection the network refused. A host
that leaves 10% or more unanswered, or resets any, is flagged.

## App

```sh
ccloud app
ccloud app --summary --weeks 2
```

`ccloud app` lists the app's environments, the region each runs in and where it serves. With
`--summary`, from the region running its CI: every open pull request with its age, review state,
each flow's result on its latest commit and the environment it deployed to; then, over `--weeks`
(4 unless asked), pull requests merged and how long they took to merge and to get a first review,
prod deploys and how long a merge took to reach prod, and per CI flow its runs, median duration,
share passing first time and reruns.

## Database

```sh
ccloud db tunnel -e stage
ccloud db tunnel -e stage --background
ccloud db tunnel -e stage --stop
```

Opens a local port to an environment's database primary, by default 5432 or the next free port, and
prints the host, port, database, user, password and URL a client needs. The password is cut to its
first characters unless you pass `-s`. Developers may tunnel into stage and feature environments;
only admins may tunnel into prod, and prod is always preceded by a caution. `--background` keeps
the tunnel open after the command returns, until `--stop`, and tells any AI agent reading the
output to review writes and sensitive reads with you first.

```sh
ccloud db reset -e feat-new-checkout
ccloud db reset -e feat-new-checkout --dangerously-reset
```

Drops every table in an environment's database, then migrates and seeds it from scratch and restarts
its API, following the run until it ends. It asks you to type the environment's name first;
`--dangerously-reset` skips that, and without a terminal it is required. Developers may reset feature environments; only
admins may reset stage, and no one can reset prod.

## Env

```sh
ccloud env pull
ccloud env pull -e feat-new-checkout --force
```

Run inside an app repo. It writes the secrets a feature environment's API gets into your local env
file, under the names the API reads them by, with the values from the dev project's Secret Manager.
It uses your own Google sign-in, so you need to be one of the app's developers; when you have none,
it runs `gcloud auth application-default login` for you and carries on. Without `-e` it copies the
app's one running feature environment. It only adds what your file lacks. Where a value differs it
changes nothing and shows the start of each value, unless you pass `--force`. `--file` writes
another env file, from the repo's root. Your own settings, such as the database and Redis, are never
touched. It names each key it added or changed, never a value.

## Your cloud

A build of ccloud serves one cloud. Its regions, their Access hostnames, gateways and pinned CAs,
the OIDC issuer, the API server's name, the app repo directory and where each environment's
database is are one `cloud.Cloud` value; `internal/cloud/cloudwalk` is
CloudWalk's. The OIDC client is stamped in at release from the `GOOGLE_CLIENT_ID` and
`GOOGLE_CLIENT_SECRET` Actions secrets, so a plain `go build` cannot sign in.

## Developing

```sh
make check   # lint, build, race tests, coverage gate
```

Every merge to `main` releases: see [docs/releasing.md](docs/releasing.md).
