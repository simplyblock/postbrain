# Platform Quickstarts

Quick install references by platform. Use these as fast-start paths, then move to environment-specific configuration.

## Linux (binary script)

Recommended for teams that want minimal packaging dependencies:

```bash
./scripts/install-postbrain.sh server
./scripts/install-postbrain.sh client
```

## Linux (`.deb`/`.rpm`)

Recommended for environments with package management requirements:

```bash
sudo dpkg -i postbrain-server_<version>_linux_amd64.deb
sudo dpkg -i postbrain-client_<version>_linux_amd64.deb
```

```bash
sudo rpm -Uvh postbrain-server_<version>_linux_amd64.rpm
sudo rpm -Uvh postbrain-client_<version>_linux_amd64.rpm
```

## macOS

Use release script install for direct binaries:

```bash
./scripts/install-postbrain.sh server
./scripts/install-postbrain.sh client
```

Homebrew/MacPorts manifests are available under `packaging/`.

## Windows

Use release artifacts or winget manifests from `packaging/winget/`.

## Kubernetes

Use Helm for Kubernetes-native deployments:

```bash
helm upgrade --install postbrain ./deploy/helm/postbrain -n default
```
