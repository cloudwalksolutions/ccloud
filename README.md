# ccloud

The developer CLI for your apps in CloudWalk Cloud, or your own Custom Cloud.

```sh
brew install --cask cloudwalksolutions/tap/ccloud
```

The cask brings `kubectl`, `cloudflared` and kubelogin with it.

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
app: shop
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
ccloud logs -e stage -f -g checkout
```

An app's logs in one environment (`prod`, `stage`, `ci`, or a feature environment's name), kept
for years, so a pod long gone still has them. The region's gateway ([docs/gateway.md](docs/gateway.md)) reads them as you: it asks
the cluster whether you may read that environment's pod logs, exactly as `kubectl logs` would.
JSON lines print as `LEVEL message key=value`; `-o raw` gives each line as stored and `-o json` one
entry per line. `-f` keeps reading until Ctrl-C. The first time, `cloudflared` opens your browser to
sign in to Cloudflare Access.

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

## Your cloud

A build of ccloud serves one cloud. Its regions, their Access hostnames, gateways and pinned CAs,
the OIDC issuer, the API server's name and the app repo directory are one `cloud.Cloud` value; `internal/cloud/cloudwalk` is
CloudWalk's. The OIDC client is stamped in at release from the `GOOGLE_CLIENT_ID` and
`GOOGLE_CLIENT_SECRET` Actions secrets, so a plain `go build` cannot sign in.

## Developing

```sh
make check   # lint, build, race tests, coverage gate
```

Every merge to `main` releases: see [docs/releasing.md](docs/releasing.md).
