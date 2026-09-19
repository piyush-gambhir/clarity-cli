#!/bin/sh
set -eu

repo='piyush-gambhir/clarity-cli'
install_dir="${INSTALL_DIR:-${HOME}/.local/bin}"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in darwin|linux) ;; *) echo 'Use a Windows release ZIP for Windows.' >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo 'Unsupported architecture.' >&2; exit 1 ;; esac

if [ -n "${VERSION:-}" ]; then
  version="$VERSION"
  case "$version" in v*) ;; *) version="v$version" ;; esac
  if ! printf '%s\n' "$version" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
    echo 'VERSION must be a semantic release version such as v0.1.0.' >&2
    exit 1
  fi
  base="https://github.com/$repo/releases/download/$version"
else
  base="https://github.com/$repo/releases/latest/download"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
asset="clarity-cli_${os}_${arch}.tar.gz"
curl --proto '=https' --tlsv1.2 -fsSL "$base/$asset" -o "$tmp/$asset"
curl --proto '=https' --tlsv1.2 -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
else
  actual=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo 'Checksum verification failed.' >&2
  exit 1
fi
# Extract only the expected binary, not arbitrary archive paths.
tar -xzf "$tmp/$asset" -C "$tmp" clarity
if [ ! -f "$tmp/clarity" ] || [ -L "$tmp/clarity" ]; then
  echo 'Archive does not contain a regular clarity binary.' >&2
  exit 1
fi
mkdir -p "$install_dir"
install -m 755 "$tmp/clarity" "$install_dir/clarity"
echo "Installed $install_dir/clarity. Ensure $install_dir is on PATH."
