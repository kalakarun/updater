# Current scope and remaining information

The user-supplied flow image takes precedence over the earlier expanded TUF prompt, and the subsequent single-key request removes key B. The original Word requirements still supply the signed JSON schema, limits, version encoding, and static build requirements.

Implemented flow: signed metadata download, key-A-only verification, strict rules and expiry, persisted sequence, installed-version comparison, exact archive size/hash, safe extraction, one shell invocation, and atomic version recording after exit zero.

Removed: TUF metadata and libraries, root-key roles, threshold signatures, signature collection, rollout cohorts, independent health/version queries, phase-based installers, application rollback, transaction journals, and their old tests/documentation.

Still needed from the product owner:

1. Production run.sh requirements. A copy-only sample is now provided at examples/test-package/run.sh; it does not install a working cPGuard application. See docs/test-package.md for the inspected local sample and packaging procedure.
2. Confirmed metadata URL and exact allowed archive/CDN hostnames.
3. Public key A, generated and held on the publisher side. Never send the private key or passphrase.
4. Installed paths for both updater binaries, the cron schedule, and the log destination; the initial installer creates cron and selects the architecture.
5. Supported Linux distributions/versions and disposable native amd64/arm64 test hosts for package/OS compatibility checks.

No independent version API or multi-person root-key custody plan is required for this narrowed implementation. Script success is determined by exit status. Atomic application installation, rollback, OS-specific file selection, and self-replacement must be handled by run.sh if needed.
