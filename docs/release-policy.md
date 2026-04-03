# Release and Versioning Policy

This page explains how Postbrain releases are versioned and distributed.

## Version tags

Stable releases are published with `vX.Y.Z` tags.
CI and pre-release pipelines may generate non-stable artifacts for validation, but production installs should prefer
tagged releases.

## Distribution channels

Release artifacts are published through multiple channels so teams can choose the distribution model that fits their
operations:

- GitHub Releases (archives and packages)
- Docker image: `simplyblock/postbrain`
- Helm chart in repository (`deploy/helm/postbrain`)

## Artifact split

Artifacts are intentionally split so server/runtime and operator tooling can be managed independently:

- `postbrain-server`: server runtime
- `postbrain-client`: CLI/hook tooling

## Breaking changes

Breaking changes are documented in release notes.
When a release includes configuration, migration, or deployment behavior changes, release notes should include explicit
migration guidance.

## Upgrade recommendation

- Prefer incremental minor upgrades over large version jumps.
- Always run backup + smoke tests around production upgrades.

If your environment has strict change windows, pin exact tags and avoid “latest” deployment references for production.
