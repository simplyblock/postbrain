# Compatibility Matrix

This page summarizes supported runtime combinations.

## Server runtime

- Go: follows `go.mod` toolchain requirement.
- PostgreSQL: 18 required (uuidv7).
- Required extensions:
  - `vector`
  - `pg_trgm`
  - `uuid-ossp`
  - `pg_cron`
  - `pg_partman`

## Deployment tooling

- Docker: current stable engine/runtime.
- Helm: 3+.
- Kubernetes: Gateway/HTTPRoute features require Gateway API CRDs and a compatible controller.

## Binary targets

Server and client artifacts are published for:

- Linux: `amd64`, `arm64`
- macOS: `amd64`, `arm64`
- Windows: `amd64`, `arm64`

## Packaging targets

- Linux packages: `.deb`, `.rpm` (`postbrain-server`, `postbrain-client`)
- Packaging manifests:
  - Homebrew
  - MacPorts
  - winget

## Notes

- cert-manager integration is optional and only required when using chart-managed certificate resources.
- OAuth/social login availability depends on external provider configuration.
