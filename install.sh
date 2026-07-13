#!/bin/sh
# Installs the tfvault binary into ~/.local/bin and links it into
# Terraform's plugin directory (~/.terraform.d/plugins) via
# `tfvault install`. No sudo required.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tedilabs/tfvault/main/install.sh | sh
#   TFVAULT_VERSION=v0.1.0 sh install.sh   # pin a specific version
set -eu

REPO="tedilabs/tfvault"
BINARY="tfvault"
BIN_DIR="${HOME}/.local/bin"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) echo "unsupported OS: $os (darwin and linux only)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch (amd64 and arm64 only)" >&2; exit 1 ;;
esac

version="${TFVAULT_VERSION:-}"
if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -n1 | cut -d'"' -f4)
fi
if [ -z "$version" ]; then
  echo "failed to determine the latest release version" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

base="https://github.com/${REPO}/releases/download/${version}"
asset="tfvault_${version#v}_${os}_${arch}.tar.gz"

echo "Downloading ${asset} (${version})..."
curl -fsSL -o "${tmp}/${asset}" "${base}/${asset}"
curl -fsSL -o "${tmp}/checksums.txt" "${base}/checksums.txt"

(
  cd "$tmp"
  if command -v sha256sum >/dev/null 2>&1; then
    grep "  ${asset}\$" checksums.txt | sha256sum -c - >/dev/null
  else
    grep "  ${asset}\$" checksums.txt | shasum -a 256 -c - >/dev/null
  fi
)
echo "Checksum verified."

tar -xzf "${tmp}/${asset}" -C "$tmp"
mkdir -p "$BIN_DIR"
install -m 0755 "${tmp}/${BINARY}" "${BIN_DIR}/${BINARY}"
echo "Installed ${BINARY} ${version} to ${BIN_DIR}"

"${BIN_DIR}/${BINARY}" install

case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *) echo "note: ${BIN_DIR} is not in your PATH; add it to use the tfvault CLI directly" ;;
esac

cat <<'EOF'

To finish setup, add this to your ~/.terraformrc
(or a per-account file selected via TF_CLI_CONFIG_FILE):

  credentials_helper "tfvault" {
    args = []
  }

Then check everything with: tfvault status
EOF
