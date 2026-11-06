#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
: "${CPGUARD_GO:?Set CPGUARD_GO to the pinned Linux Go executable}"
export GOTOOLCHAIN=local
export GOMODCACHE="$PWD/.cache/mod"
export GOCACHE="$PWD/.cache/linux-build"
export GOPATH="$PWD/.cache/linux-gopath"
export GOTMPDIR="$PWD/.cache/linux-tmp"
mkdir -p "$GOTMPDIR"
"$CPGUARD_GO" test -count=1 -coverprofile=coverage.out ./...
"$CPGUARD_GO" vet ./...
"$CPGUARD_GO" run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
