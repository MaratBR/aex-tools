#!/bin/sh
# Builds the Linux app and its plugins into dist/.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# Most current distributions use WebKit2GTK 4.1. Older ones can set
# WEBKIT_TAG=webkit2_40 when building against WebKit2GTK 4.0.
webkit_tag=${WEBKIT_TAG:-webkit2_41}
case "$webkit_tag" in
    webkit2_40|webkit2_41) ;;
    *) echo "WEBKIT_TAG must be webkit2_40 or webkit2_41" >&2; exit 2 ;;
esac

# Check the headers and tools before building anything. pkg-config also checks
# the libraries required by each module, rather than just package names.
webkit_version=4.1
soup_module=libsoup-3.0
if [ "$webkit_tag" = webkit2_40 ]; then
    webkit_version=4.0
    soup_module=libsoup-2.4
fi

missing_dependencies() {
    if ! command -v "${CC:-cc}" >/dev/null 2>&1; then
        echo "C compiler (${CC:-cc})"
    fi
    if ! command -v pkg-config >/dev/null 2>&1; then
        echo "pkg-config"
    else
        for module in gtk+-3.0 gio-unix-2.0 "webkit2gtk-$webkit_version" "$soup_module"; do
            if ! pkg-config --exists "$module"; then
                echo "$module development files"
            fi
        done
    fi
}

as_root() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    elif command -v sudo >/dev/null 2>&1; then
        sudo "$@"
    else
        echo "Installing packages needs root access; install them as root and rerun this script." >&2
        exit 1
    fi
}

missing=$(missing_dependencies)
if [ -n "$missing" ]; then
    printf 'Missing Linux build dependencies:\n%s\n' "$missing"
    if command -v apt-get >/dev/null 2>&1; then
        manager=apt-get
        set -- build-essential pkg-config libgtk-3-dev libglib2.0-dev "libwebkit2gtk-$webkit_version-dev"
        if [ "$webkit_version" = 4.1 ]; then
            set -- "$@" libsoup-3.0-dev
        else
            set -- "$@" libsoup2.4-dev
        fi
    elif command -v dnf >/dev/null 2>&1; then
        manager=dnf
        set -- gcc pkgconf-pkg-config gtk3-devel glib2-devel "webkit2gtk$webkit_version-devel"
        if [ "$webkit_version" = 4.1 ]; then
            set -- "$@" libsoup3-devel
        else
            set -- "$@" libsoup-devel
        fi
    else
        echo "No supported package manager (apt-get or dnf). Install the dependencies above and rerun." >&2
        exit 1
    fi
    printf 'Packages: %s\n' "$*"
    printf 'Install these with %s? [y/N] ' "$manager"
    answer=
    if ! IFS= read -r answer; then
        answer=
    fi
    case "$answer" in
        y|Y|yes|YES|Yes)
            if [ "$manager" = apt-get ]; then
                as_root apt-get update
            fi
            as_root "$manager" install -y "$@"
            ;;
        *) echo "Install the dependencies and rerun this script." >&2; exit 1 ;;
    esac
    missing=$(missing_dependencies)
    if [ -n "$missing" ]; then
        printf 'Build dependencies still unavailable:\n%s\nCheck CC and PKG_CONFIG_PATH.\n' "$missing" >&2
        exit 1
    fi
fi

mkdir -p dist/plugins
go build -trimpath -ldflags '-s -w' -o dist/plugins/ ./plugins/...

hashes=$(find dist/plugins -maxdepth 1 -type f -print0 | sort -z | xargs -0 sha256sum | cut -d ' ' -f 1 | paste -sd,)
built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags="-s -w -X aex/internal/plugin.preApproved=$hashes -X aex/internal/about.builtAt=$built_at"
go build -tags "desktop,production,$webkit_tag" -trimpath -ldflags "$ldflags" -o dist/aex .

echo "Built dist/aex and dist/plugins/"
