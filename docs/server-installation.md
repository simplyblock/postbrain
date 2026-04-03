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

## Next steps after installation

After server is running:

1. Create bearer tokens (`postbrain token create`).
2. Configure client env (`POSTBRAIN_URL`, `POSTBRAIN_TOKEN`, optional `POSTBRAIN_SCOPE`).
3. Install agent skills (see [Using Postbrain with Coding Agents](./using-with-coding-agents.md)).
4. Configure OAuth/social login if needed (see [OAuth Logins and Configuration](./oauth-logins.md)).
