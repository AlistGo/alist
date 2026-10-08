#!/usr/bin/env bash
set -euo pipefail

# Keep the existing FreeBSD ABI baseline after its release files move to archives.
release=14.3
arch=${1:?Usage: build-freebsd.sh <386|amd64|arm64>}
case "$arch" in
  386) os_arch=i386; target=i386 ;;
  amd64) os_arch=amd64; target=x86_64 ;;
  arm64) os_arch=arm64; target=aarch64 ;;
  *) echo "Unsupported FreeBSD architecture: $arch" >&2; exit 1 ;;
esac

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
base_url="https://archive.freebsd.org/old-releases/$os_arch/$release-RELEASE"
curl --fail --location --retry 3 "$base_url/MANIFEST" -o "$work_dir/MANIFEST"
curl --fail --location --retry 3 "$base_url/base.txz" -o "$work_dir/base.txz"
checksum=$(awk '$1 == "base.txz" { print $2 }' "$work_dir/MANIFEST")
(cd "$work_dir"; printf '%s  base.txz\n' "$checksum" | sha256sum --check -)
mkdir "$work_dir/sysroot"
tar -xf "$work_dir/base.txz" -C "$work_dir/sysroot"

built_at=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
git_commit=$(git rev-parse HEAD)
version=${GITHUB_REF:-dev}
version=${version#refs/tags/}
ldflags="-w -s
-X 'github.com/alist-org/alist/v3/internal/conf.BuiltAt=$built_at'
-X 'github.com/alist-org/alist/v3/internal/conf.GitAuthor=Xhofe'
-X 'github.com/alist-org/alist/v3/internal/conf.GitCommit=$git_commit'
-X 'github.com/alist-org/alist/v3/internal/conf.Version=$version'
-X 'github.com/alist-org/alist/v3/internal/conf.WebVersion=dev'"
mkdir -p build
CGO_ENABLED=1 GOOS=freebsd GOARCH="$arch" CGO_LDFLAGS=-fuse-ld=lld \
  CC="clang --target=$target-unknown-freebsd$release --sysroot=$work_dir/sysroot" \
  go build -o "build/alist-freebsd-$arch" -ldflags="$ldflags" .
