# cPGuard signed package updater

This project contains the production updater and publisher tools using one trusted Ed25519 signing key, Key A.

## Source layout

- `cmd/cpguard-updater/main.go` and `support.go`: updater command, configuration and result logging.
- `cmd/cpguard-sign/main.go`: publisher key generation, signing and renewal.
- `cmd/cpguard-audit/main.go`: static Linux binary verification used by the build script.
- `internal/`: shared runtime packages; retain all platform-specific source files.
- `go.mod` and `go.sum`: pinned module dependencies.
- `internal/site/site.json`: embedded public key, metadata URL and allowed archive hosts.

Test source, dummy installers, test-only tools and the manual-test profile have been removed. The obsolete `cpguardtest` build tag is rejected rather than silently selecting production paths.

## Build

Go 1.27.3 is required. From the project root on Linux:

```sh
mkdir -p dist
CGO_ENABLED=0 go build -mod=readonly -buildvcs=false -trimpath -ldflags="-s -w -buildid=" -o dist/cpguard-sign ./cmd/cpguard-sign
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 go build -mod=readonly -buildvcs=false -trimpath -ldflags="-s -w -buildid=" -o dist/cpguard-updater-amd64 ./cmd/cpguard-updater
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM64=v8.0 go build -mod=readonly -buildvcs=false -trimpath -ldflags="-s -w -buildid=" -o dist/cpguard-updater-arm64 ./cmd/cpguard-updater
go vet ./...
CGO_ENABLED=0 go run ./cmd/cpguard-audit dist/cpguard-updater-amd64 dist/cpguard-updater-arm64
```

From PowerShell, `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1` builds static Linux amd64/arm64 updaters and macOS amd64/arm64 signers, plus module/source inventories, build records and a static ELF audit. No automated tests remain in this production-only source tree; build and static checks are not substitutes for regression tests.

The release linker flags remove symbol/DWARF debug information, not TLS or update verification. The audited Go 1.27.3 Linux amd64 release build is about 7.02 MiB. Equivalent byte validators remove the regexp dependency, and installer output is streamed without retaining an unused copy. Other architectures/toolchains can differ. Stripping makes debugger analysis less convenient and does not by itself reduce runtime memory use. The PowerShell build script already uses these flags.

The current `internal/site/site.json` fetches signed metadata from `https://updates.opsshield.com/cpguard/updates.json` and allows archive downloads only from `repo.opsshield.com`. Sign reviewed packages for `https://repo.opsshield.com/latest/app.tar.gz`. The existing public key A is unchanged; independently verify its suitability before production deployment. Never put a private signing key or passphrase in the source tree, updater binary or download server.

## Update flow

The root updater locks protected state, downloads and verifies signed metadata, validates expiry and sequence, and compares the release with `/opt/cpguard/app/setup/swversion`. A newer release is downloaded, matched against the signed exact size and SHA-256, and safely extracted. Only then is `/bin/sh ./run.sh` called once without arguments. The version is recorded atomically after a successful script exit; temporary files are cleaned up.

Updater commands are `bootstrap`, `check`, `apply` and `version`; no argument defaults to `apply`. Initialize fresh protected state with `bootstrap` only once. Never delete existing state or lower the installed version to bypass a rejection. Each check/apply appends its UTC result to `/opt/cpguard/logs/install_log`, while retaining JSON console output.

Signing commands are `keygen`, `make` and `renew`. Private Key A uses passphrase-encrypted age/PKCS#8 storage. The updater needs only its embedded public key.

A failing run.sh may already have changed application files; this reduced flow cannot undo them. There is no automatic application rollback, independent health API or multi-key threshold policy. A compromised signer or root account is outside the update authentication guarantee.

- [Configuration and publishing](docs/operations.md)
- [Protocol and limits](docs/architecture.md)
- [run.sh expectations](docs/installer-contract.md)
- [Remaining information](docs/requirements.md)
- [Security scope](SECURITY.md)
- [Audit findings and validation](docs/audit.md)
