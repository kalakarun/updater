# Verification of the signed JSON flow

The previous TUF and two-key test records are superseded. The key-A-only signed JSON flow was verified on 2026-11-06 in the existing Ubuntu WSL amd64 environment with Go 1.27.3.

| Check | Result |
| --- | --- |
| Full Linux test suite with coverage | PASS, all packages |
| Linux and Windows go vet | PASS |
| govulncheck v1.9.0 | No vulnerabilities found |
| go mod verify | All modules verified |
| Static Linux amd64 and arm64 builds | PASS, CGO_ENABLED=0; ELF audit finds no interpreter/shared libraries |
| macOS amd64 and arm64 publisher builds | PASS, cross-compiled; native macOS execution remains untested |
| Single-key configuration | Key A accepted; removed key_b rejected; other signers rejected |
| Copy-only sample using the supplied archive | PASS; all 1,588 copied app files verified byte-for-byte in a native Linux temporary directory |
| Application-owned version file | PASS as Linux root with UID 999-owned application directories and version file; atomic replacement preserves owner, group and permissions |

The workflow package has 87.4% statement coverage in this run. Coverage is evidence of exercised branches, not proof that every failure or OS combination is safe. The runtime dependency graph contains no TUF, sigstore, or securesystemslib modules.

Coverage includes the single trusted key A, rejection of an unknown key, changed payloads, each payload validation rule, duplicate/missing/unknown JSON, malformed signature encoding, expiry and future issuance, sequence rollback/conflicting equality/retry, package size/hash failure, unsafe archives, and installer gating. The previous two-key results are superseded by the single-key validation recorded below.

Workflow fixtures check successful version recording, equal/older version no-op, check-only behavior, missing/corrupt installed version/state, installer failure, retry of an accepted release, temporary-file cleanup, unexpected script version writes, and a persistence failure after version rename.

Version-file fixtures use native temporary Linux directories, never the installed cPGuard path. They cover descriptor-relative access, rejection of symlinks/hardlinks, unexpected owners, writable/special modes, FIFOs without blocking, relocated parents, bounded reads, missing-file restoration and replaced temporary-file cleanup. Workflow tests verify that reads, stats, writes and failure restoration all use the configured version-file callbacks, and that a changed version baseline stops installation before run.sh.

Installer timeout and child-process tests run on Linux using temporary scripts. Publisher tests use ephemeral test-only keys and verify encrypted-key round trips and renewal preserving artifact identity.

No production endpoints or application installers are used. A mock installer is not proof of cPGuard OS compatibility. Native arm64/macOS execution, the real run.sh, cron setup, CA-bundle removal, root-owned deployment-path lock behavior, and supported Linux distributions still require integration acceptance.

The Windows development environment previously failed localhost TLS certificate validation and test-executable cleanup. Linux WSL is used for the complete fixture suite without disabling certificate verification.

The sample's mode-0700 check initially failed on Windows-mounted temporary directories. The test fixture now uses a native Linux /tmp directory; the installer permission check was retained. The actual sample payload was only copied, never executed. No remote server changes or production-path installations were performed.
