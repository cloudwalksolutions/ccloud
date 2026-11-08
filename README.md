# ccloud

Reach your apps in the cloudwalk datacenter as yourself.

```sh
ccloud clusters auth --region us-central1
kubectl --context ccloud-us-central1 get pods -n prod-<app>
```

`clusters auth` signs you in through Cloudflare Access and Google in your browser, then adds a
`ccloud-<region>` context to your kubeconfig. It never changes your current context. What you may
do is decided by the cluster, from your Google Workspace domain: read your apps' namespaces, and
delete pods in feature environments and CI.

## Install

You need `kubectl`, `cloudflared` and kubelogin:

```sh
brew install kubectl cloudflared int128/kubelogin/kubelogin
```

From a checkout of this repo, with `.env` holding the Google desktop client:

```sh
(set -a; . ./.env; cd cli && go install -ldflags "-X ccloud/internal/google.ClientID=$DEVELOPER_OIDC_CLIENT_ID -X ccloud/internal/google.ClientSecret=$DEVELOPER_OIDC_CLIENT_SECRET" ./cmd/ccloud)
```
