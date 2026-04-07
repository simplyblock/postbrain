#!/usr/bin/env bash
# install-postbrain.sh - Install postbrain or postbrain-cli from GitHub releases.
#
# Usage:
#   ./install-postbrain.sh [server|client] [version]
#
# Examples:
#   ./install-postbrain.sh server
#   ./install-postbrain.sh client v1.2.3
#
# Environment:
#   POSTBRAIN_REPO       GitHub repo slug (default: simplyblock/postbrain)
#   POSTBRAIN_INSTALLDIR Install destination (default: /usr/local/bin)

set -euo pipefail

COMPONENT="${1:-server}"
VERSION_ARG="${2:-}"
REPO="${POSTBRAIN_REPO:-simplyblock/postbrain}"
INSTALL_DIR="${POSTBRAIN_INSTALLDIR:-/usr/local/bin}"

case "$COMPONENT" in
  server)
    artifact_prefix="postbrain-server"
    binary_name="postbrain"
    ;;
  client)
    artifact_prefix="postbrain-client"
    binary_name="postbrain-cli"
    ;;
  *)
    echo "unsupported component: $COMPONENT (use: server|client)" >&2
    exit 1
    ;;
esac

os="$(uname -s)"
case "$os" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "unsupported OS: $os (supported: Linux, Darwin)" >&2
    exit 1
    ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *)
    echo "unsupported arch: $arch (supported: amd64, arm64)" >&2
    exit 1
    ;;
esac

if [[ -n "$VERSION_ARG" ]]; then
  tag="$VERSION_ARG"
elif [[ -n "${POSTBRAIN_VERSION:-}" ]]; then
  tag="${POSTBRAIN_VERSION}"
else
  tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
fi

if [[ -z "$tag" ]]; then
  echo "failed to resolve release tag (pass explicit version)" >&2
  exit 1
fi

case "$tag" in
  v*) ;;
  *) tag="v${tag}" ;;
esac

artifact="${artifact_prefix}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${artifact}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading ${url}"
curl -fL "$url" -o "${tmp}/${artifact}"
tar -xzf "${tmp}/${artifact}" -C "${tmp}"

if [[ ! -f "${tmp}/${binary_name}" ]]; then
  echo "archive did not contain expected binary: ${binary_name}" >&2
  exit 1
fi

if [[ -w "${INSTALL_DIR}" ]]; then
  install -m 0755 "${tmp}/${binary_name}" "${INSTALL_DIR}/${binary_name}"
else
  sudo install -m 0755 "${tmp}/${binary_name}" "${INSTALL_DIR}/${binary_name}"
fi

echo "installed ${binary_name} to ${INSTALL_DIR}/${binary_name}"
"${INSTALL_DIR}/${binary_name}" version || true
