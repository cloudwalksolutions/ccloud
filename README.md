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

## Workflows

```sh
ccloud workflow list -a <app> -r us-east1 --pr 105
ccloud workflow view api-tests-x7k2p -a <app> -r us-east1
ccloud workflow watch api-tests-x7k2p -a <app> -r us-east1 --exit-status
ccloud workflow logs api-tests-x7k2p -a <app> -r us-east1 --failed
ccloud workflow rerun api-tests-x7k2p -a <app> -r us-east1
ccloud workflow cancel build-images-q9w1z -a <app> -r us-east1
```

Your app's CI runs, read through the cluster you signed in to with `clusters auth`. `list` shows
each run's result, flow, pull request, branch and timing, newest first; `view` shows a run's steps
in the order they ran, how long each took, and why one failed. `watch` redraws that view until the
run ends, and with `--exit-status` exits non-zero unless it succeeded. `logs` prints each step's
log as `step<TAB>line`, from its pod while it exists and from the logs gateway once it is gone.
`rerun` starts the whole run again for the same pull request and commit; `cancel` stops a run still
going, and its exit handlers still report.

## Logs

```sh
ccloud logs -a <app> -e prod -r us-central1 -l error --since 6h
ccloud logs -a <app> -e stage -r us-central1 -f -g checkout
```

An app's logs in one environment (`prod`, `stage`, `ci`, or a feature environment's name), kept
for years, so a pod long gone still has them. The region's logs gateway reads them as you: it asks
the cluster whether you may read that environment's pod logs, exactly as `kubectl logs` would.
JSON lines print as `LEVEL message key=value`; `-o raw` gives each line as stored and `-o json` one
entry per line. `-f` keeps reading until Ctrl-C. The first time, `cloudflared` opens your browser to
sign in to Cloudflare Access.

## Your cloud

A build of ccloud serves one cloud. Its regions, their Access hostnames and pinned CAs, the OIDC
issuer and the API server's name are one `cloud.Cloud` value; `internal/cloud/cloudwalk` is
CloudWalk's. The OIDC client is stamped in at release from the `GOOGLE_CLIENT_ID` and
`GOOGLE_CLIENT_SECRET` Actions secrets, so a plain `go build` cannot sign in.

## Developing

```sh
make check   # lint, build, race tests, coverage gate
```

Every merge to `main` releases: see [docs/releasing.md](docs/releasing.md).
