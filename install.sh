#!/usr/bin/env bash
# Official Linux install script for ctop (RHEL / Ubuntu)
# Runs fully unprivileged with no root/sudo access required.

set -e

KERNEL=$(uname -s)
ARCH=$(uname -m)

function output() { echo -e "\033[32mctop-install\033[0m" "$@"; }

function command_exists() {
  command -v "$@" > /dev/null 2>&1
}

# Only Linux (RHEL and Ubuntu) supported by this installer
if [ "$KERNEL" != "Linux" ]; then
  output "Error: Platform '$KERNEL' is not supported by this installer. (Supported: Linux RHEL/Ubuntu)"
  exit 1
fi

case $ARCH in
  x86_64 | amd64) MATCH_BUILD="linux-amd64" ;;
  *)
    output "Error: Architecture '$ARCH' is not supported."
    exit 1
    ;;
esac

for req in curl wget sha256sum; do
  command_exists "$req" || {
    output "Error: missing required '$req' binary"
    exit 1
  }
done

# Determine user-writable target installation directory
if [ -n "$BIN_DIR" ]; then
  INSTALL_DIR="$BIN_DIR"
elif [ -w "/usr/local/bin" ]; then
  INSTALL_DIR="/usr/local/bin"
else
  INSTALL_DIR="${HOME}/.local/bin"
fi

mkdir -p "$INSTALL_DIR"

function extract_url() {
  local match="$1"
  local json="$2"
  echo "$json" | grep -o '"browser_download_url": *"[^"]*"' | grep "$match" | head -n 1 | cut -d '"' -f 4
}

TMP=$(mktemp -d "${TMPDIR:-/tmp}/ctop.XXXXX")
trap 'rm -rf "$TMP"' EXIT INT TERM
cd "$TMP"

output "fetching latest release info"
resp=$(curl -sSL https://api.github.com/repos/edsilegx/ctop/releases/latest)

output "fetching release checksums"
checksum_url=$(extract_url "sha256sums.txt" "$resp")
if [ -z "$checksum_url" ]; then
  output "Error: Failed to extract sha256sums.txt download URL from GitHub API."
  exit 1
fi
wget -q "$checksum_url" -O sha256sums.txt

# skip if latest already installed
cur_ctop=$(command -v ctop 2> /dev/null || true)
if [ -n "$cur_ctop" ]; then
  cur_sum=$(sha256sum "$cur_ctop" | awk '{print $1}')
  if grep -q "$cur_sum" sha256sums.txt; then
    output "ctop is already up-to-date at $cur_ctop"
    exit 0
  fi
fi

output "fetching latest ctop binary"
url=$(extract_url "$MATCH_BUILD" "$resp")
if [ -z "$url" ]; then
  output "Error: Failed to locate download URL for $MATCH_BUILD."
  exit 1
fi
wget -q --show-progress "$url"

output "verifying checksum"
sha256sum -c --quiet --ignore-missing sha256sums.txt || {
  output "Error: Checksum verification failed!"
  exit 1
}

output "installing to $INSTALL_DIR/ctop"
chmod +x ctop-*
mv ctop-* "$INSTALL_DIR/ctop"

output "Installation complete!"

# Check if INSTALL_DIR is in PATH
if ! echo ":$PATH:" | grep -q ":$INSTALL_DIR:"; then
  output "Note: '$INSTALL_DIR' is not in your current PATH."
  output "Add it to your environment: export PATH=\"\$PATH:$INSTALL_DIR\""
fi

# Inform about Docker socket / group access
if [ -S "/var/run/docker.sock" ] && [ ! -w "/var/run/docker.sock" ]; then
  if ! id -nG | grep -qw "docker"; then
    output "Note: To access Docker containers without root, ensure your user is in the 'docker' group:"
    output "      sudo usermod -aG docker \$USER (or configure appropriate sudo/socket access policies)"
  fi
fi
