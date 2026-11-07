# Current scope and remaining information

The user-supplied flow image takes precedence over the earlier expanded TUF prompt, and the subsequent single-key request removes key B. The original Word requirements still supply the signed JSON schema, limits, version encoding, and static build requirements.

Implemented flow: signed metadata download, key-A-only verification, strict rules and expiry, persisted sequence, installed-version comparison, exact archive size/hash, safe extraction, one shell invocation, and atomic version recording after exit zero.

Removed: TUF metadata and libraries, root-key roles, threshold signatures, signature collection, rollout cohorts, independent health/version queries, phase-based installers, application rollback, transaction journals, and their old tests/documentation.

Still needed from the product owner:

1. Production run.sh requirements and a reviewed real application installer. The dummy installer and test-package tooling have been removed; no production run.sh is supplied by this source tree.
2. Deployment validation that `https://updates.opsshield.com/cpguard/updates.json` directly serves signed JSON and `https://repo.opsshield.com/latest/app.tar.gz` serves its exact matching archive. These URLs and the archive host have been supplied and configured; valid publication and cache behavior still need acceptance.
3. Public key A, generated and held on the publisher side. Never send the private key or passphrase.
4. Installed paths for both updater binaries and the cron schedule; the initial installer creates cron and selects the architecture. The agreed result log is now `/opt/cpguard/logs/install_log`; its directory must already exist with safe ownership/permissions.
5. Supported Linux distributions/versions and disposable native amd64/arm64 test hosts for package/OS compatibility checks.

No independent version API or multi-person root-key custody plan is required for this narrowed implementation. Script success is determined by exit status. Atomic application installation, rollback, OS-specific file selection, and self-replacement must be handled by run.sh if needed.
