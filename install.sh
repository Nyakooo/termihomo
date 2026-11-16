#!/bin/sh
set -eu

repo=Nyakooo/termihomo
case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) printf '%s\n' 'On Windows, use install.ps1.' >&2; exit 1 ;;
esac
if [ "$os" = linux ] && { [ -n "${TERMUX_VERSION:-}" ] || [ "$(uname -o 2>/dev/null || true)" = Android ]; }; then
    os=android
fi
case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) printf '%s\n' 'Termihomo is available for AMD64 and ARM64.' >&2; exit 1 ;;
esac
if [ "$os" = android ] && [ "$arch" != arm64 ]; then
    printf '%s\n' 'Termux requires Android ARM64.' >&2
    exit 1
fi

asset="termihomo-$os-$arch.tar.gz"
member=termihomo
dest=${TERMIHOMO_INSTALL_DIR:-"$HOME/.local/bin"}
if [ "$os" = android ]; then
    member=termihomo-android-arm64
    dest=${TERMIHOMO_INSTALL_DIR:-"${PREFIX:?Termux PREFIX is required}/bin"}
fi

tmp=$(mktemp -d)
staged=
cleanup() {
    if [ -n "$staged" ]; then rm -f "$staged"; fi
    rm -f "$tmp/$asset" "$tmp/SHA256SUMS"
    rmdir "$tmp"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

# Resolve the public release link once, then fetch both assets from that tag.
if release=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest" 2>/dev/null) &&
    [ "${release%/*}" = "https://github.com/$repo/releases/tag" ]; then
    tag=${release##*/}
    public=1
else
    if ! command -v gh >/dev/null 2>&1; then
        printf '%s\n' 'Release unavailable. For a private repository, install GitHub CLI and sign in or set GH_TOKEN.' >&2
        exit 1
    fi
    tag=$(gh api "repos/$repo/releases/latest" --jq .tag_name)
    public=0
fi
if ! printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
    printf '%s\n' 'GitHub did not return a stable Termihomo release.' >&2
    exit 1
fi
if [ "$public" = 1 ]; then
    base="https://github.com/$repo/releases/download/$tag"
    curl -fsSL "$base/$asset" -o "$tmp/$asset"
    curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
else
    gh release download "$tag" --repo "$repo" --dir "$tmp" --pattern "$asset" --pattern SHA256SUMS
fi

expected=$(awk -v name="$asset" '
    NF == 2 && ($2 == name || $2 == "*" name) { hash = $1; count++ }
    END { if (count != 1 || length(hash) != 64 || hash !~ /^[0-9A-Fa-f]+$/) exit 1; print tolower(hash) }
' "$tmp/SHA256SUMS") || { printf '%s\n' 'Invalid SHA256SUMS.' >&2; exit 1; }

if command -v sha256sum >/dev/null 2>&1; then
    output=$(sha256sum "$tmp/$asset")
else
    output=$(shasum -a 256 "$tmp/$asset")
fi
if [ "${output%% *}" != "$expected" ]; then
    printf '%s\n' 'Termihomo SHA-256 verification failed.' >&2
    exit 1
fi

mkdir -p "$dest"
dest=$(cd "$dest" && pwd -P)
staged=$(mktemp "$dest/.termihomo-install.XXXXXXXX")
tar -xOzf "$tmp/$asset" "$member" > "$staged"
test -s "$staged"
chmod 755 "$staged"
test ! -d "$dest/termihomo"
mv -f "$staged" "$dest/termihomo"
staged=
printf 'Installed Termihomo %s at %s\n' "$tag" "$dest/termihomo"
case ":$PATH:" in
    *":$dest:"*) ;;
    *) printf 'Add %s to PATH to run termihomo by name.\n' "$dest" ;;
esac
