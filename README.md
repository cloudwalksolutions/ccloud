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
```

Your app's CI runs, read through the cluster you signed in to with `clusters auth`. `list` shows
each run's result, flow, pull request, branch and timing, newest first; `view` shows a run's steps
in the order they ran, how long each took, and why one failed.

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
