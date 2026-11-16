#!/bin/sh
set -eu

asset=$1
case "${asset##*/}" in
    termihomo-android-*) member=termihomo-android-arm64; export TERMUX_VERSION=smoke ;;
    *) member=termihomo ;;
esac
fixture=$(mktemp -d)
trap 'find "$fixture" -depth -delete' EXIT
export TERMIHOMO_TEST_FIXTURE="$fixture"
export TERMIHOMO_INSTALL_DIR="$fixture/install"
cp "$asset" "$fixture/"
mkdir "$fixture/bin"
cat > "$fixture/bin/curl" <<'MOCK'
#!/bin/sh
set -eu
case "$2" in
    */releases/download/v1.2.3/*) test "$3" = -o; cp "$TERMIHOMO_TEST_FIXTURE/${2##*/}" "$4" ;;
    *)
        test "$6" = 'https://github.com/Nyakooo/termihomo/releases/latest'
        printf '%s' 'https://github.com/Nyakooo/termihomo/releases/tag/v1.2.3'
        ;;
esac
MOCK
chmod +x "$fixture/bin/curl"
export PATH="$fixture/bin:$PATH"
if [ "$member" = termihomo-android-arm64 ]; then
    # The release runner stages the Android artifact without executing it.
    cat > "$fixture/bin/uname" <<'MOCK'
#!/bin/sh
case "$1" in
    -s) printf 'Linux\n' ;;
    -m) printf 'aarch64\n' ;;
    *) exit 1 ;;
esac
MOCK
    chmod +x "$fixture/bin/uname"
fi
(
    cd "$fixture"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${asset##*/}" > SHA256SUMS
    else
        shasum -a 256 "${asset##*/}" > SHA256SUMS
    fi
)
sh ./install.sh
tar -xOzf "$asset" "$member" > "$fixture/expected"
cmp "$fixture/expected" "$TERMIHOMO_INSTALL_DIR/termihomo"
printf 'invalid binary' > "$fixture/${asset##*/}"
if sh ./install.sh; then
    printf 'Installer accepted a checksum mismatch\n' >&2
    exit 1
fi
cmp "$fixture/expected" "$TERMIHOMO_INSTALL_DIR/termihomo"
