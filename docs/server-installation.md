# Server Installation

This page explains the supported ways to install and run the Postbrain server.

Choose the option that matches your environment:

- local process (build from source)
- GitHub release downloads (prebuilt binaries)
- Docker image (`simplyblock/postbrain`)
- Kubernetes with Helm chart

## Option 1: Local process (build from source)

Use this for local development or when you want full source-level control.

Prerequisites:

- Go toolchain matching `go.mod`
- PostgreSQL with required extensions (`vector`, `pg_trgm`, `uuid-ossp`)

Example:

```bash
git clone https://github.com/simplyblock/postbrain.git
cd postbrain

docker compose up -d postgres
cp config.example.yaml config.yaml

make build
./postbrain serve --config config.yaml
```

By default, server listens on `http://localhost:7433`.

## Option 2: Download from GitHub Releases

Use this when you want prebuilt binaries without local compilation.

1. Open the repository Releases page.
2. Download the `postbrain-server_<os>_<arch>` archive for your platform.
3. Extract `postbrain` and place it in your PATH.
4. Copy `config.example.yaml` to `config.yaml` and adjust values.
5. Start server:

```bash
postbrain serve --config /path/to/config.yaml
```

## Option 3: Docker image

Use this for simple containerized deployment.

Image:

- `simplyblock/postbrain`

Example run:

```bash
docker run --rm -p 7433:7433 \
  -v "$(pwd)/config.yaml:/etc/postbrain/config.yaml:ro" \
  simplyblock/postbrain:latest
```

The image default command already starts:

```bash
postbrain serve --config /etc/postbrain/config.yaml
```

## Option 4: Kubernetes with Helm

Use this for Kubernetes-native deployment.

The repository includes a Helm chart at:

- `deploy/helm/postbrain`

Install from repository checkout:

```bash
helm upgrade --install postbrain ./deploy/helm/postbrain
```

Use Gateway API HTTPRoute instead of Ingress:

```bash
helm upgrade --install postbrain ./deploy/helm/postbrain \
  --set ingress.enabled=false \
  --set httpRoute.enabled=true \
  --set-string 'httpRoute.parentRefs[0].name=public-gw' \
  --set-string 'httpRoute.parentRefs[0].namespace=default'
```

Important chart behavior:

- exactly one of `ingress.enabled` or `httpRoute.enabled` must be `true`
- runtime `config.yaml` is generated from chart `values.yaml`
- default image repository is `simplyblock/postbrain`

### Helm prerequisites

- Kubernetes cluster
- Helm 3+
- PostgreSQL reachable from cluster (with required extensions)
- Optional for Gateway mode: Gateway API CRDs + controller
- Optional for cert automation: cert-manager

### Routing modes

#### Mode 1: Ingress

Default mode.

```yaml
ingress:
  enabled: true

httpRoute:
  enabled: false
```

#### Mode 2: HTTPRoute with existing Gateway

Use your own existing gateway:

```yaml
ingress:
  enabled: false

httpRoute:
  enabled: true
  parentRefs:
    - name: public-gateway
      namespace: default
```

#### Mode 3: HTTPRoute with chart-managed Gateway

Let the chart create a Gateway resource:

```yaml
ingress:
  enabled: false

httpRoute:
  enabled: true
  parentRefs: []

gateway:
  enabled: true
  name: postbrain-public-gateway
  gatewayClassName: kong-class
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
      allowedRoutes:
        namespaces:
          from: All
      tls:
        mode: Terminate
        certificateRefs:
          - name: postbrain-cert-secret
            kind: Secret
            group: ""
```

Validation rules:

- If `httpRoute.enabled=true`, you must either:
  - set `httpRoute.parentRefs`, or
  - set `gateway.enabled=true`
- If `gateway.enabled=true`, set `gateway.gatewayClassName`

### Optional certificate resource

The chart can create a cert-manager `Certificate`:

```yaml
certificate:
  enabled: true
  name: postbrain-cert
  secretName: postbrain-cert-secret
  issuerRef:
    name: letsencrypt-dns
    kind: ClusterIssuer
    group: cert-manager.io
  dnsNames:
    - postbrain.simplyblock.ai
```

Important:

- Gateway `tls.certificateRefs[].name` must point to `certificate.secretName`, not the certificate resource name.
- If certificate issuance is still pending, the secret may not exist yet and Gateway can report `InvalidCertificateRef`.

### Helm troubleshooting

`exec serve failed: No such file or directory`

- Use current chart version. It sets command to `postbrain` with args `serve --config ...`.

Gateway references TLS secret that does not exist:

```bash
kubectl -n default get certificate postbrain-cert
kubectl -n default describe certificate postbrain-cert
kubectl -n default get secret postbrain-cert-secret
```

If ACME DNS01 is pending:

```bash
kubectl -n default get certificaterequest,order,challenge
kubectl -n default describe challenge <name>
```

## Next steps after installation

After server is running:

1. Create bearer tokens (`postbrain token create`).
2. Configure client env (`POSTBRAIN_URL`, `POSTBRAIN_TOKEN`, optional `POSTBRAIN_SCOPE`).
3. Install agent skills (see [Using Postbrain with Coding Agents](./using-with-coding-agents.md)).
4. Configure OAuth/social login if needed (see [OAuth Logins and Configuration](./oauth-logins.md)).
