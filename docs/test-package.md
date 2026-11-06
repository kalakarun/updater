# Copy-only sample update

The inspected local file is `C:\kala\update\app .tar` (there is a space before `.tar`). Despite its extension, its bytes are gzip-compressed tar. Its listing contains 1,747 entries under app/: 159 directories and 1,588 regular files. No top-level run.sh or links were shown in the listing. The packaging helper performs complete bounded validation before producing a test package; it does not execute payload files.

## Build the test package locally

From the repository root, using the pinned Go toolchain and workspace cache settings documented in README:

```powershell
go run ./cmd/cpguard-test-package --archive 'C:/kala/update/app .tar' --out dist/cpguard-test.tar.gz
```

The helper accepts plain tar or gzip, requires an app/ payload, rejects links/traversal/duplicates/conflicts/unsafe modes and excessive sizes, and adds `examples/test-package/run.sh` at the package root. Original input is not modified. Existing output is never overwritten. If output already exists, choose a new filename.

The generated tar.gz contains run.sh and app/. Its SHA-256 and compressed size must be signed using key A only after packaging; the source app .tar and the generated test package have different hashes. Publish the generated tar.gz, not the original file, for an updater test.

The prepared `dist/cpguard-test.tar.gz` is 28,336,200 bytes, with SHA-256 `26fb3b9cc733be9de62fe3d4dfc3b4234008c631544de8956b3eb9645f476951`. It contains 1,588 app files totaling 93,539,425 uncompressed bytes plus the test installer and directories. It is unsigned; no release-signing private key is included. Linux validation on 2026-11-06 verified every copied app file byte-for-byte in a temporary test destination.

## What the sample script does

The updater invokes `/bin/sh ./run.sh` without arguments. The script detects Linux amd64/arm64, validates a directory-only/regular-file-only app payload, and copies app/ into a fresh release directory beneath `/var/lib/cpguard-updater-test`. It writes installed.txt with the detected OS/architecture and prints the destination. It starts no application, PHP code, executable, service, cron job, or database migration and does not write swversion or production cPGuard paths.

On a failed copy, it removes only its newly created release directory. Successful test releases remain for inspection. Bootstrap/cron/signing and version recording remain the updater's responsibility.

The shell requires standard Linux tools: uname, find, dirname, stat, id, mkdir, mktemp, cp, and rm. These are sample installer dependencies, not additional commands invoked by the Go updater.

For a manual non-root test, create a private parent directory, extract the generated package there, enter its package root, and explicitly set `CPGUARD_TEST_ROOT` to an owned absolute directory named cpguard-updater-test under that parent. The parent must be canonical and not use symlinks; an existing destination must be owned by the invoking user and mode 0700. The production updater uses a controlled environment, so this manual override is not propagated during its normal execution.

Do not run the production updater against the live /opt/cpguard version file just to test this sample. A production HTTPS end-to-end run still needs an agreed disposable host, HTTPS metadata/archive URLs, and public key A. No private signing key is generated or shipped with the test archive.

## Offline signed-flow test

No HTTPS server or CDN upload is needed for the offline integration test. It exercises the shared updater service with local metadata/archive inputs and private native Linux /tmp version/state/output directories. The real installed cPGuard and production updater state are not read or written. Only the repository's byte-exact copy-only run.sh is allowed to execute.

Build the dummy package and sign it using the existing test key A. Adjust /etc/update/app.tar to the actual archive filename. The reserved offline.invalid URL is a schema-valid identifier only: the test never downloads it. This metadata is for offline testing, not publication or the production updater.

```sh
./dist/cpguard-test-package \
  --archive /etc/update/app.tar \
  --out dist/cpguard-offline-test.tar.gz

./dist/cpguard-sign make \
  --archive dist/cpguard-offline-test.tar.gz \
  --url https://offline.invalid/cpguard-dummy-test.tar.gz \
  --version 5.89.00 --sequence 1 --valid-for 24h \
  --key /root/cpguard-publisher-test/key-a.age \
  --out /root/cpguard-publisher-test/update_check-offline-1.json

CPGUARD_SAMPLE_ARCHIVE="$PWD/dist/cpguard-offline-test.tar.gz" \
CPGUARD_SAMPLE_METADATA=/root/cpguard-publisher-test/update_check-offline-1.json \
CPGUARD_SAMPLE_PUBLIC_KEY=/root/cpguard-publisher-test/key-a.public.txt \
  go test ./internal/testpackage -run '^TestOfflineSignedSampleFlow$' -count=1 -v
```

Use fresh output filenames if they already exist. Run within 24 hours, with a correct server clock. The integration test verifies check/install/no-op behavior (temporary swversion 58826 to 58900), signature failure, changed archive bytes, truncated archives and cleanup. The signing private key/passphrase is only used by cpguard-sign, never by the test.

This test substitutes local file transport and an isolated dummy invocation. It does not exercise real HTTPS downloads, certificate validation or the production command's root/path/lock checks and installer environment. Do not run production bootstrap/apply for this offline test. Production behavior and embedded site configuration are unchanged.
