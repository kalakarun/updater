# Configuration and publisher workflow

## Build configuration

Edit `internal/site/site.json` before the production build:

- `key_a`: the single base64-encoded 32-byte Ed25519 public key.
- `metadata_url`: currently the GitHub test URL `https://raw.githubusercontent.com/kalakarun/uptest/main/cpguard/updates.json`; replace it with the confirmed production URL before deployment.
- `allowed_cdn_hosts`: actual archive hostnames only, without schemes, ports, or paths.

These values are embedded in the updater binaries, not accepted from runtime flags or downloaded JSON. The key_b field has been removed and is rejected as unknown. Private key A and its passphrase must never enter the repository, binaries, or download servers. An empty development key or host list prevents production operations.

The current configuration still trusts the supplied development public key and allows raw.githubusercontent.com. These settings were not changed by the source cleanup and must be reviewed before production deployment.

### Updater source files

The production command uses only `cmd/cpguard-updater/main.go` and `cmd/cpguard-updater/support.go`, plus its imported `internal/` packages. `support.go` combines the production configuration, safe version/log opening and result formatting. Test files, dummy package helpers, test guides and the manual-test profile have been removed. The static audit tool and build script remain because they verify the production binaries.

Build the package with `go build ./cmd/cpguard-updater`, not `go build main.go`. The old `cpguardtest` tag is explicitly rejected so an outdated test command cannot silently build a production updater. Stop using any previously built manual-test binaries. There are no automated tests in this production-only tree; use build, vet and static audit checks, and retain independent regression/integration acceptance before release.

If the server sync does not delete removed files, they must also be removed from the server. The local cleanup does not modify the remote host. After syncing the new source, the following cleanup is limited to retired project test assets and obsolete command files; it never resets application versions, signing keys, logs or updater state:

```sh
cd /etc/updateGoscript &&
test -f cmd/cpguard-updater/support.go &&
test -f go.mod &&
find cmd internal -type f -name '*_test.go' -delete &&
rm -f -- cmd/cpguard-updater/profile_production.go \
  cmd/cpguard-updater/result_log.go cmd/cpguard-updater/version.go \
  cmd/cpguard-updater/profile_cpguardtest.go \
  cmd/cpguard-test-package/main.go internal/testpackage/package.go \
  examples/test-package/run.sh \
  scripts/verify.ps1 scripts/verify-linux.sh scripts/test-linux.sh \
  docs/test-package.md docs/github-test.md docs/security-testing.md docs/verification.md \
  coverage.out dist/cpguard-test-package dist/cpguard-updater-test \
  dist/cpguard-updater-test-amd64 dist/cpguard-updater-test-arm64 \
  dist/cpguard-test.tar.gz dist/github-cpg-1.0.1.tar.gz
```

Any resulting empty helper directories can be removed with `rmdir`; leave nonempty directories for inspection instead of deleting them recursively. Production builds remain in `dist/`. Existing source caches, Git history and editor/SFTP settings were retained.

## Key generation on the Mac

Run these commands in a private directory outside this repository on the publisher's Mac.

```text
cpguard-sign keygen --out key-a.age --public key-a.public.txt
```

The tool prompts for and confirms a passphrase of at least 16 bytes. Use a unique, strong random passphrase. Public output is base64 suitable for the build configuration. Private output is age scrypt-encrypted PKCS#8. Decryption occurs in memory; Go memory clearing is best effort and does not protect a compromised workstation or its swap/crash dumps. Maintain protected encrypted backups on the publisher side.

Only signatures matching key A are accepted. No three-key root ceremony or multiple release approvals are required by this flow. Maintain an encrypted backup: there is no standby key, and changing the trusted public key requires independently authenticated updater distribution.

## Make a release

Build one tar.gz containing a regular run.sh at the top level and all required OS/CPU files. Choose a version and a publisher sequence greater than all previously published sequences. Keep a release ledger; the tool does not allocate global sequence numbers.

```text
cpguard-sign make --archive RELEASE.tar.gz --url CONFIRMED_HTTPS_ARCHIVE_URL --version 5.89.00 --sequence 1042 --key key-a.age --out update_check.json
```

This hashes the archive, records its exact size, constructs the payload, signs it, and checks its own signature. Default validity is seven days; `--valid-for 24h` can shorten it. Existing output files are never overwritten.

Upload the exact archive bytes to the signed URL first. Then replace update_check.json atomically on the metadata server. Do not edit a signed payload or change the archive after signing. Automated uploading/publishing is outside this local tool.

## Renew before expiry

```text
cpguard-sign renew --in update_check.json --sequence 1043 --key key-a.age --out update_check.renewed.json
```

Renew verifies the previous envelope with key A, preserves version/URL/hash/size, increases the sequence, and refreshes dates. Renew before expiry and use the same key that signed the input. If metadata has already expired, use make again against the reviewed original archive with a higher sequence. Metadata signed by any other key is rejected by the updater.

Some operator must renew metadata even when there is no new application release. Expired metadata stops updates. There is no separate renewal signing role.

## Customer installation and cron

The existing authenticated initial installer must select x86_64 -> cpguard-updater-amd64 or aarch64 -> cpguard-updater-arm64 using uname -m, and reject other architectures. The updater's installed directory and cron schedule remain to be supplied; do not install a speculative path or schedule. Install the updater binary and cron configuration in root-owned, non-symlink paths that are not writable by the cpguard account or other unprivileged users. Do not place the root-executed updater in the cpguard-writable application tree. Protecting these deployment paths is the initial installer's responsibility, not an executable-path check performed by the running updater.

Run the selected binary's `bootstrap` command once to create protected sequence state. Then cron runs the selected binary with no arguments or with `apply`, as root. `check` verifies metadata and compares versions without downloading or installing an archive; accepting a new sequence still updates protected state. `version` reports the build/platform, profile, version-file path, state-directory path and result-log path without checking for updates or writing a log entry.

### Application version ownership

The production updater reads `/opt/cpguard/app/setup/swversion`. `/opt` and its ancestors must remain root-owned, non-symlink and not group/world writable. Below that anchor, the `cpguard/app/setup` directories and regular `swversion` file may belong to root or the UID resolved from the local `cpguard` account. Group/world-writable paths, symlinks and unsupported file types are rejected. If the account does not exist, only root-owned paths are accepted. No ownership change to the installed cPGuard tree is required.

Version access uses pinned directory descriptors, checked path identities and bounded no-follow reads rather than relaxing the root-only state checks. Replacement preserves the version file's original owner, permissions and newline style. `run.sh` must leave `swversion` and its parent directories unchanged; the updater records the new version only after the script succeeds. This does not make the application-owned version a root-trusted security record: the cpguard account can still change its reported version. See `architecture.md` for that trust boundary.

`/var/lib/cpguard-updater` and its sequence state remain root-only and must not be moved beneath the application tree. Do not reset existing state to bypass an error. Cleanup of repository test assets does not authorize deleting existing server version, log or updater state files.

Installed `58826` means `5.88.26`, which is newer than signed `1.0.1`. A check/apply therefore reports `up_to_date` for that release without running its installer. Never lower the real version to force an installation.

### Update result log

Each `check` or `apply` invocation appends one result to `/opt/cpguard/logs/install_log`, in addition to the existing JSON stderr/stdout output. `bootstrap` and `version` commands do not write update-result entries. Timestamps are UTC RFC3339, without fractional seconds:

```text
2026-11-06T12:23:13Z sw-updater update available
2026-11-06T12:23:14Z sw-updater success
2026-11-06T12:23:15Z sw-updater not available
2026-11-06T12:23:16Z sw-updater failed: metadata signature does not match key A
```

`available` maps to `update available`, `up_to_date` to `not available`, and `installed` to `success`. A failure includes its reason, normalized to a single printable line and limited to 2,048 bytes including any truncation marker. Installer stdout/stderr remains in structured stderr, not this result file. The plain result does not include downloaded payloads or private signing material.

`/opt/cpguard/logs` must already exist. `/opt` remains root-protected; application directories and the log may belong to root or the local cpguard UID, without group/world write permissions or special modes. Symlinks, hardlinks and non-regular log files are rejected. A missing log file is exclusively created with mode 0644 (subject to the process umask); an existing file is appended without truncating it or changing its owner/mode. Descriptor-relative access, identity checks, bounded locking and file sync constrain root writes. Rotation to another safe regular file between invocations is supported. The cpguard account can still modify an application-owned log, so it is not a tamper-proof audit record.

The log is opened and validated before update state changes, downloads or installation. An opening failure stops the update and is reported on stderr; there is no alternate-path fallback. Later append/sync/close failures also return a nonzero exit code, but cannot undo an installation that already completed. Errors after installation explicitly identify that it already completed. Argument, unsupported-platform and privilege failures that occur before log setup are reported on stderr only.

An equal or older release logs `not available`; never reset version or sequence state just to reproduce `success`.

The updater does not install cron entries or invoke cron/uname itself. The initial installer remains responsible for the agreed cron schedule, existing log directory and any external log rotation/collection.

Existing state must not be reset automatically. For upgrades from the previous TUF development build, preserve its files and explicitly plan migration; no force/reset command is provided.
