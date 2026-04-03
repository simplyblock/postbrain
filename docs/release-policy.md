# Release and Versioning Policy

This page explains how Postbrain releases are versioned and distributed.

## Version tags

- Releases are published with `vX.Y.Z` tags.
- Development builds may use non-release identifiers in CI artifacts.

## Distribution channels

- GitHub Releases (archives and packages)
- Docker image: `simplyblock/postbrain`
- Helm chart in repository (`deploy/helm/postbrain`)

## Artifact split

- `postbrain-server`: server runtime
- `postbrain-client`: CLI/hook tooling

## Breaking changes

- Breaking changes are documented in release notes.
- Configuration and operational changes should include migration notes.

## Upgrade recommendation

- Prefer incremental minor upgrades over large version jumps.
- Always run backup + smoke tests around production upgrades.
