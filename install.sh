#!/bin/sh
# Install rill. Run it again to update; it stops when what you have is current.
#
#   curl -fsSL https://raw.githubusercontent.com/sonquer/rill/main/install.sh | sh
#   ... | sh -s -- --version v0.1.0
#   ... | sh -s -- --dir /usr/local/bin
#   ... | sh -s -- --force
#   ... | sh -s -- --require-signature
set -eu

REPO="sonquer/rill"
BINARY="rill"
VERSION="${RILL_VERSION:-}"
DIR="${RILL_INSTALL_DIR:-}"
FORCE="${RILL_FORCE:-}"
REQUIRE_SIGNATURE="${RILL_REQUIRE_SIGNATURE:-}"

say() { printf '%s\n' "$*"; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

usage() {
  cat <<'EOF'
usage: install.sh [--version TAG] [--dir PATH] [--force] [--require-signature]

  --version TAG        install this tag instead of the latest release
  --dir PATH           install into this directory, given as an absolute path
  --force              reinstall even when the current version already matches
  --require-signature  refuse to install unless cosign verifies the release

  RILL_VERSION, RILL_INSTALL_DIR, RILL_FORCE and RILL_REQUIRE_SIGNATURE do the same.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:-}"; [ -n "$VERSION" ] || die "--version needs a tag"; shift 2 ;;
    --dir)     DIR="${2:-}";     [ -n "$DIR" ]     || die "--dir needs a path";    shift 2 ;;
    --force)   FORCE=1; shift ;;
    --require-signature) REQUIRE_SIGNATURE=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option $1" ;;
  esac
done

for tool in curl tar uname mktemp; do
  have "$tool" || die "$tool is required and is not on PATH"
done

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported operating system $(uname -s); see https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture $(uname -m); see https://github.com/$REPO/releases" ;;
esac

if [ -z "$DIR" ]; then
  if [ -w "/usr/local/bin" ] 2>/dev/null; then DIR=/usr/local/bin; else DIR="$HOME/.local/bin"; fi
fi
case "$DIR" in
  /*) ;;
  *) die "--dir needs an absolute path, got $DIR" ;;
esac
DIR="${DIR%/}"

if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$VERSION" ] || die "could not find the latest release; pass --version"
fi
number="${VERSION#v}"

if [ -z "$FORCE" ] && have "$BINARY"; then
  current=$("$BINARY" version 2>/dev/null | head -n 1 | awk '{print $2}' || true)
  if [ "$current" = "$number" ] || [ "$current" = "$VERSION" ]; then
    say "rill $VERSION is already installed at $(command -v "$BINARY")"
    exit 0
  fi
fi

archive="rill_${number}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$VERSION"
tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT INT TERM

say "downloading rill $VERSION for $os/$arch"
curl -fsSL "$base/$archive" -o "$tmp/$archive" || die "no archive for $os/$arch in $VERSION"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || die "could not download checksums.txt"

if have sha256sum; then
  sum=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif have shasum; then
  sum=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else
  die "neither sha256sum nor shasum is available; refusing to install unverified"
fi
want=$(awk -v name="$archive" '$2 == name || $2 == "*" name {print $1}' "$tmp/checksums.txt" | head -n 1)
[ -n "$want" ] || die "$archive is not listed in checksums.txt"
[ "$sum" = "$want" ] || die "checksum mismatch for $archive; refusing to install"
say "checksum ok"

if have cosign; then
  if curl -fsSL "$base/checksums.txt.pem" -o "$tmp/checksums.txt.pem" &&
     curl -fsSL "$base/checksums.txt.sig" -o "$tmp/checksums.txt.sig"; then
    cosign verify-blob "$tmp/checksums.txt" \
      --certificate "$tmp/checksums.txt.pem" \
      --signature "$tmp/checksums.txt.sig" \
      --certificate-identity-regexp "https://github\.com/$REPO/\.github/workflows/release\.yml@.*" \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com >/dev/null 2>&1 &&
      say "signature ok" || die "the signature on checksums.txt did not verify"
  elif [ -n "$REQUIRE_SIGNATURE" ]; then
    die "no signature published for $VERSION and --require-signature was given"
  else
    say "no signature published for this release"
  fi
elif [ -n "$REQUIRE_SIGNATURE" ]; then
  die "cosign is not installed and --require-signature was given"
else
  say "cosign is not installed, so the signature was not checked"
fi

tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY" || die "could not unpack $archive"
chmod +x "$tmp/$BINARY" || die "could not make $BINARY executable"
mkdir -p "$DIR" || die "could not create $DIR"
if [ -w "$DIR" ]; then
  mv "$tmp/$BINARY" "$DIR/$BINARY" || die "could not move $BINARY into $DIR"
elif have sudo; then
  say "$DIR needs elevated permissions"
  sudo mv "$tmp/$BINARY" "$DIR/$BINARY" || die "could not move $BINARY into $DIR"
else
  die "$DIR is not writable and sudo is not available; pass --dir"
fi

say "installed rill $VERSION to $DIR/$BINARY"
case ":$PATH:" in
  *":$DIR:"*) ;;
  *) say ""; say "$DIR is not on your PATH. Add it:"; say "  export PATH=\"$DIR:\$PATH\"" ;;
esac
