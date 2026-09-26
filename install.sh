#!/bin/sh
# Installs buti from the GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/BartInTheField/buti/main/install.sh | sh
#
# Environment:
#   BUTI_VERSION      release to install, e.g. 2026.09.26.1 (default: latest)
#   BUTI_INSTALL_DIR  where to put the binary (default: ~/.local/bin)
set -eu

repo="BartInTheField/buti"
install_dir="${BUTI_INSTALL_DIR:-$HOME/.local/bin}"

err() {
	echo "buti install: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || err "$1 is required"
}

need curl
need tar
need uname

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) err "unsupported OS $(uname -s); download a binary from https://github.com/$repo/releases" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) err "unsupported architecture $(uname -m)" ;;
esac

version="${BUTI_VERSION:-}"
if [ -z "$version" ]; then
	# /releases/latest redirects to /releases/tag/<version>.
	url=$(curl -fsSLo /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
		err "could not find the latest release"
	case "$url" in
	*/releases/tag/*) version="${url##*/}" ;;
	*) err "no release published yet" ;;
	esac
fi

name="buti_${version}_${os}_${arch}"
base="https://github.com/$repo/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading buti $version ($os/$arch)..."
curl -fsSL -o "$tmp/$name.tar.gz" "$base/$name.tar.gz" || err "download failed: $base/$name.tar.gz"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || err "download failed: $base/checksums.txt"

expected=$(grep " $name.tar.gz\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$expected" ] || err "no checksum for $name.tar.gz"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$name.tar.gz" | cut -d' ' -f1)
else
	actual=$(shasum -a 256 "$tmp/$name.tar.gz" | cut -d' ' -f1)
fi
[ "$expected" = "$actual" ] || err "checksum mismatch for $name.tar.gz"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
mkdir -p "$install_dir"
install -m 755 "$tmp/$name/buti" "$install_dir/buti"
echo "Installed buti $version to $install_dir/buti"

case ":$PATH:" in
*":$install_dir:"*) ;;
*) echo "Note: $install_dir is not on your PATH." ;;
esac
if ! command -v but >/dev/null 2>&1; then
	echo "Note: buti needs the GitButler CLI (\`but\`) on your PATH: https://gitbutler.com"
fi
