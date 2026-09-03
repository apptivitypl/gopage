#!/bin/sh
# Remove rill and, if you ask, everything it has cached.
#
#   curl -fsSL https://raw.githubusercontent.com/apptivitypl/rill/main/uninstall.sh | sh
#   ... | sh -s -- --purge
set -eu

BINARY="rill"
DIR="${RILL_INSTALL_DIR:-}"
PURGE="${RILL_PURGE:-}"
YES="${RILL_YES:-}"

say() { printf '%s\n' "$*"; }
die() { printf 'uninstall: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

usage() {
  cat <<'EOF'
usage: uninstall.sh [--dir PATH] [--purge] [--yes]

  --dir PATH   remove the binary from here instead of searching PATH
  --purge      also remove the cache rill downloads its toolchains into
  --yes        do not ask

  Projects you created are never touched.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dir)   DIR="${2:-}"; [ -n "$DIR" ] || die "--dir needs a path"; shift 2 ;;
    --purge) PURGE=1; shift ;;
    --yes|-y) YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option $1" ;;
  esac
done

if [ -n "$DIR" ]; then
  target="$DIR/$BINARY"
  [ -e "$target" ] || die "no $BINARY in $DIR"
else
  target=$(command -v "$BINARY" 2>/dev/null || true)
  [ -n "$target" ] || { say "rill is not on your PATH; nothing to remove"; exit 0; }
fi

version=$("$target" version 2>/dev/null | head -n 1 || echo "rill")
say "found $version at $target"

if [ "${target#"$(go env GOPATH 2>/dev/null || echo /nonexistent)/bin"}" != "$target" ]; then
  say ""
  say "that copy came from 'go install'. Removing the file is enough, but"
  say "'go install github.com/apptivitypl/rill/cmd/rill@latest' would bring it back."
fi

case "$(uname -s)" in
  Darwin) cache="$HOME/Library/Caches/rill" ;;
  *) cache="${XDG_CACHE_HOME:-$HOME/.cache}/rill" ;;
esac

if [ -z "$YES" ]; then
  say ""
  say "about to remove:"
  say "  $target"
  [ -n "$PURGE" ] && [ -d "$cache" ] && say "  $cache"
  printf 'continue? [y/N] '
  read -r answer </dev/tty 2>/dev/null || answer=n
  case "$answer" in y|Y|yes|YES) ;; *) say "nothing was removed"; exit 0 ;; esac
fi

if [ -w "$(dirname "$target")" ]; then
  rm -f "$target"
elif have sudo; then
  sudo rm -f "$target"
else
  die "$(dirname "$target") is not writable and sudo is not available"
fi
say "removed $target"

if [ -n "$PURGE" ]; then
  if [ -d "$cache" ]; then
    rm -rf "$cache"
    say "removed $cache"
  else
    say "no cache at $cache"
  fi
fi

say ""
say "projects you created were left alone. To remove one, delete its directory."
