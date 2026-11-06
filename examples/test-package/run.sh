#!/bin/sh
set -eu
umask 077

if [ "$#" -ne 0 ]; then
    printf '%s\n' 'This test installer takes no arguments.' >&2
    exit 1
fi

system=$(uname -s)
machine=$(uname -m)
if [ "$system" != Linux ]; then
    printf 'Unsupported test OS: %s\n' "$system" >&2
    exit 1
fi
case "$machine" in
    x86_64) architecture=amd64 ;;
    aarch64) architecture=arm64 ;;
    *) printf 'Unsupported test CPU: %s\n' "$machine" >&2; exit 1 ;;
esac

if [ ! -d ./app ] || [ -L ./app ]; then
    printf '%s\n' 'Expected a regular app directory beside run.sh.' >&2
    exit 1
fi
unsupported=$(find ./app ! -type f ! -type d -print -quit)
if [ -n "$unsupported" ]; then
    printf '%s\n' 'The test payload contains a link or unsupported file type.' >&2
    exit 1
fi

test_root=${CPGUARD_TEST_ROOT:-/var/lib/cpguard-updater-test}
case "$test_root" in
    /*/cpguard-updater-test) ;;
    *) printf '%s\n' 'Test destination must be an absolute path ending in /cpguard-updater-test.' >&2; exit 1 ;;
esac
if [ -L "$test_root" ]; then
    printf '%s\n' 'Refusing a symlink as the test destination.' >&2
    exit 1
fi
parent=$(dirname "$test_root")
physical_parent=$(cd "$parent" && pwd -P)
if [ "$parent" != "$physical_parent" ]; then
    printf '%s\n' 'Test destination parent must be canonical and must not contain symlinks.' >&2
    exit 1
fi
if [ ! -d "$test_root" ]; then
    mkdir -m 700 "$test_root"
fi
if [ -L "$test_root" ] || [ ! -d "$test_root" ] || [ "$(stat -c %u "$test_root")" != "$(id -u)" ] || [ "$(stat -c %a "$test_root")" != 700 ]; then
    printf '%s\n' 'Test destination must be an owned private directory with mode 0700.' >&2
    exit 1
fi

staging=$(mktemp -d "$test_root/release.XXXXXX")
completed=0
cleanup() {
    if [ "$completed" -eq 0 ]; then
        case "$staging" in "$test_root"/release.*) rm -rf -- "$staging" ;; esac
    fi
}
trap cleanup 0
trap 'exit 1' 1 2 15

cp -R ./app "$staging/app"
printf 'purpose=copy-only-updater-test\nos=%s\narchitecture=%s\n' "$system" "$architecture" > "$staging/installed.txt"
completed=1
printf 'Test installation completed: %s\n' "$staging"
printf '%s\n' 'Application files were copied only; no application code or services were started.'
