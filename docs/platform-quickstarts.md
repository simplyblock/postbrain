# Platform Quickstarts

Quick install references by platform.

## Linux (binary script)

```bash
./scripts/install-postbrain.sh server
./scripts/install-postbrain.sh client
```

## Linux (`.deb`/`.rpm`)

```bash
sudo dpkg -i postbrain-server_<version>_linux_amd64.deb
sudo dpkg -i postbrain-client_<version>_linux_amd64.deb
```

```bash
sudo rpm -Uvh postbrain-server_<version>_linux_amd64.rpm
sudo rpm -Uvh postbrain-client_<version>_linux_amd64.rpm
```

## macOS

```bash
./scripts/install-postbrain.sh server
./scripts/install-postbrain.sh client
```

Homebrew/MacPorts manifests are available under `packaging/`.

## Windows

Use release artifacts or winget manifests from `packaging/winget/`.

## Kubernetes

```bash
helm upgrade --install postbrain ./deploy/helm/postbrain -n default
```
