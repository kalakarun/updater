# Configuration and publisher workflow

## Build configuration

Edit `internal/site/site.json` before the production build:

- `key_a`: the single base64-encoded 32-byte Ed25519 public key.
- `metadata_url`: currently the original `https://downloads.opsshield.com/cpguard/update_check.json`; confirm this location.
- `allowed_cdn_hosts`: actual archive hostnames only, without schemes, ports, or paths.

These values are embedded in both binaries, not accepted from runtime flags or downloaded JSON. The key_b field has been removed and is rejected as unknown. Private key A and its passphrase must never enter the repository, binaries, or download servers. An empty development key or host list prevents production operations.

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

The existing authenticated initial installer must select x86_64 -> cpguard-updater-amd64 or aarch64 -> cpguard-updater-arm64 using uname -m, and reject other architectures. The updater's installed directory and cron schedule remain to be supplied; do not install a speculative path or schedule.

Run the selected binary's `bootstrap` command once to create protected sequence state. Then cron runs the selected binary with no arguments or with `apply`, as root. `check` verifies metadata and compares versions without downloading or installing an archive. `version` only reports build/platform information.

Logs use structured stderr; configure the agreed protected log file or collector in the initial installer/cron integration. The updater does not install cron entries or invoke cron/uname itself.

Existing state must not be reset automatically. For upgrades from the previous TUF development build, preserve its files and explicitly plan migration; no force/reset command is provided.
