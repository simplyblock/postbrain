# Compatibility Matrix

This page summarizes the deployment/runtime combinations that are expected to work in production.
Use it when planning new environments or debugging unexplained behavior.

## Server runtime

Postbrain server builds and runtime behavior follow the repository toolchain and DB assumptions:

- Go: follows `go.mod` toolchain requirement.
- PostgreSQL: 18 required.
- Required extensions:
  - `vector`
  - `pg_trgm`
  - `uuid-ossp`
  - `pg_cron`
  - `pg_partman`

## Deployment tooling

These are the standard deployment paths covered by repository workflows and docs:

- Docker: current stable engine/runtime.
- Helm: 3+.
- Kubernetes: Gateway/HTTPRoute features require Gateway API CRDs and a compatible controller.

## Binary targets

Server and client release artifacts are published for:

- Linux: `amd64`, `arm64`
- macOS: `amd64`, `arm64`
- Windows: `amd64`, `arm64`

## Packaging targets

Package output is split by component (`postbrain-server`, `postbrain-client`):

- Linux packages: `.deb`, `.rpm` (`postbrain-server`, `postbrain-client`)
- Packaging manifests:
  - Homebrew
  - MacPorts
  - winget

## Notes

- cert-manager integration is optional and only required when using chart-managed certificate resources.
- OAuth/social login availability depends on external provider configuration.

If you are outside these combinations, start with the nearest supported baseline before filing bugs. Most “random”
runtime issues in practice come from missing DB extensions, unsupported architecture assumptions, or incomplete
Gateway/cert-manager prerequisites.
