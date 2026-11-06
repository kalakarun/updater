# GitHub HTTPS sample test

The GitHub blob links are browser pages, not artifact download URLs. This test uses raw.githubusercontent.com directly, without enabling redirects, relaxing TLS, accepting unsigned files or changing the single-Key-A policy.

The inspected archive URL is:

```text
https://raw.githubusercontent.com/kalakarun/uptest/403e6a06a197e2a0ba1b4f24ba9724c3ef47f6c6/cpg-1.0.1.tar.gz
```

It is 284 bytes with SHA-256 aa38e471520f2f2bc22f80cb23caf6dcd1dde6ec5d55003d8fc61165c887ba2f. It contains only a 196-byte run.sh that appends a sample message to run.log beside the script. It is not the earlier 28 MB app/ copy-only package. Do not repack or edit this archive before signing it.

The configured metadata URL tracks the repository's main branch:

```text
https://raw.githubusercontent.com/kalakarun/uptest/main/cpguard/updates.json
```

At inspection, updates.json at commit 403e6a06a197e2a0ba1b4f24ba9724c3ef47f6c6 contained only {}. It is not signed metadata and must be rejected. Updating the repository creates a new commit; the old commit URL will continue serving {}. To pin metadata instead, set metadata_url to the new raw commit URL after publishing, then rebuild.

## Build on the Linux test server

Sync the updated source, including internal/site/site.json and internal/testpackage/github_linux_test.go, before building. Keep the existing public Key A in site.json; no private key belongs there.

```sh
cd /etc/updateGoscript
export GOTOOLCHAIN=go1.27.3
umask 077
mkdir -p dist

CGO_ENABLED=0 go build -mod=readonly -trimpath \
  -o dist/cpguard-sign ./cmd/cpguard-sign
CGO_ENABLED=0 go build -mod=readonly -trimpath \
  -o dist/cpguard-updater ./cmd/cpguard-updater
```

## Download, check and sign the exact published archive

Run each command separately and stop on any failure. No private key or passphrase is sent to GitHub.

```sh
ARCHIVE_URL=https://raw.githubusercontent.com/kalakarun/uptest/403e6a06a197e2a0ba1b4f24ba9724c3ef47f6c6/cpg-1.0.1.tar.gz

curl --fail --show-error --proto '=https' --max-time 60 \
  --max-filesize 1048576 --output dist/github-cpg-1.0.1.tar.gz "$ARCHIVE_URL"

printf '%s  %s\n' \
  aa38e471520f2f2bc22f80cb23caf6dcd1dde6ec5d55003d8fc61165c887ba2f \
  dist/github-cpg-1.0.1.tar.gz | sha256sum --check

./dist/cpguard-sign make \
  --archive dist/github-cpg-1.0.1.tar.gz \
  --url "$ARCHIVE_URL" \
  --version 1.0.1 --sequence 1 --valid-for 24h \
  --key /root/cpguard-publisher-test/key-a.age \
  --out /root/cpguard-publisher-test/updates-github-1.json
```

Reuse the existing test key; choose a new output filename if it already exists. This sequence is for fresh, isolated test state, not for replacing an existing production sequence ledger. The offline.invalid metadata cannot be reused because the signed archive URL and hash are different.

## Publish the signed JSON and run the isolated online test

Upload only updates-github-1.json to kalakarun/uptest as cpguard/updates.json on main. Never upload key-a.age or its passphrase. Publish the exact JSON without editing its payload/signature. Allow GitHub's raw-content cache to update before testing.

```sh
test -f internal/testpackage/github_linux_test.go &&
CPGUARD_GITHUB_TEST=1 go test ./internal/testpackage \
  -run '^TestGitHubSignedSampleFlow$' -count=1 -v
```

This opt-in test uses the actual HTTPS clients and shared updater service to download, authenticate and safely extract the archive. It only executes the exact inspected sample run.sh, using the production installer runner. The test prints the sample run.log before extraction cleanup, checks available -> installed -> up_to_date and records a version only in a private native /tmp directory. Temporary sequence/version/output files are removed at test completion. It does not invoke the production CLI's root/path/lock/bootstrap checks.

The isolated initial version is 1.0.0 (10000), so a signed 1.0.1 records 10001. The real installed cPGuard version 5.88.26 (58826) is not changed. A production check/apply correctly treats 1.0.1 as older; never lower the real swversion to force this sample to install, and never delete real updater state to rerun a test.

Ordinary go test ./... skips the live network test. It still exercises the GitHub URL policy, sample artifact guard and safe installer fixture. Run the live test within metadata validity; expired or unsigned metadata is an error, not a reason to bypass checks.

## Run the actual CLI manually with a dummy version file

An existing cPGuard installation may own /opt/cpguard/app/setup/swversion and its application directories as the cpguard account. The production build supports that layout without changing ownership: /opt stays root-protected, application entries must belong to root or the local cpguard UID, and symlinks and group/world write permissions are rejected. Version operations use held directory descriptors rather than untrusted pathname writes. Sequence state remains root-protected. Because cpguard can change its own version file, that file is application status, not a security authority against local account compromise. Do not recursively chown the installation or weaken the state checks.

For this sample, build a separate cpguardtest profile. It retains root-owned path validation, signature/expiry/sequence checks, HTTPS policy and safe extraction. Its version/state paths are fixed outside /opt/cpguard; they cannot be selected by downloaded metadata or runtime flags. This profile only permits the inspected GitHub archive URL, its exact size/hash and the exact log-only run.sh. It is not a general sandbox for arbitrary installers and must not be deployed as a production updater.

Sync the complete updated cmd/cpguard-updater directory before building:

```sh
cd /etc/updateGoscript
CGO_ENABLED=0 go build -mod=readonly -trimpath -tags cpguardtest \
  -o dist/cpguard-updater-test ./cmd/cpguard-updater

./dist/cpguard-updater-test version
```

The output must show profile=manual-test and paths beneath /var/lib/cpguard-updater-manual-test. The normal build still uses its original production paths.

As root, initialize fresh dummy data exactly once. If the test root already exists, stop and inspect it instead of resetting the version or deleting state. The exclusive file creation prevents overwriting a pre-existing version file.

```sh
TEST_ROOT=/var/lib/cpguard-updater-manual-test

if [ -e "$TEST_ROOT" ] || [ -L "$TEST_ROOT" ]; then
    echo "STOP: test directory already exists; preserve its version and state."
else
    install -d -o root -g root -m 700 "$TEST_ROOT" "$TEST_ROOT/setup" &&
    (umask 077; set -C; printf '10000\n' > "$TEST_ROOT/setup/swversion") &&
    ./dist/cpguard-updater-test bootstrap
fi
```

After successful initialization, check and apply using the test binary only:

```sh
./dist/cpguard-updater-test check &&
./dist/cpguard-updater-test apply &&
cat /var/lib/cpguard-updater-manual-test/setup/swversion &&
./dist/cpguard-updater-test apply
```

Expected: available, installed, dummy version 10001, then up_to_date. The real /opt/cpguard version and /var/lib/cpguard-updater state are not used or modified. The sample writes run.log beside its extracted run.sh, so that log is removed with normal extraction cleanup after success; an installed result and the dummy version record demonstrate completion. Ordinary go test ./... tests the production profile; also run go test -tags cpguardtest ./cmd/cpguard-updater to exercise the manual profile's artifact/installer guards.
