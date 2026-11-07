# Code audit and optimization

Reviewed on 2026-11-06. Scope: publisher/signing, signed metadata, HTTPS downloads, sequence state, extraction, installer execution, version access and result logging. This is a source-level review with isolated checks, not a guarantee of risk-free updates or a complete independent penetration test.

## Findings addressed

| Severity | Finding | Change |
| --- | --- | --- |
| Medium, reliability | Publisher outputs were written directly to their final names; interruption could expose a partial key or signed JSON and block a retry. | Same-directory private temporary file, file sync, atomic no-overwrite publication and directory sync. |
| Low | General file reads had a check/open race; publisher hashing accepted special files and could block on a FIFO. | Bounded regular-file reads, Linux/macOS no-follow and nonblocking open, identity checks, and observable size/mtime change detection. Parent directories must still be protected/private. |
| Low, availability | Permanent TLS/redirect errors were classified as retryable merely because they were wrapped URL errors. | Retry only selected transient network failures and HTTP 429/500-599, within existing time/attempt limits. |
| Low, availability | Archive extraction did not observe cancellation and obscured underlying copy/sync/close failures. | Context-aware compressed/decompressed reads, incomplete-output cleanup and joined I/O/cleanup errors. |
| Low, resource usage | Installer output retained an unused copy, and discarded chunks could still produce empty log events. | Stream only the bounded output and stop forwarding empty, discarded chunks. |
| Low, reliability | An exceptional version-temp stat failure could leave a temporary file. | Close and remove it on that failure path. |

Additional hardening clears partially read passphrases and partially decrypted key data on errors. Memory clearing remains best effort in Go.

## Optimizations

- Equivalent fixed-format byte validators replace version/digit/SHA-256 regular expressions; `regexp` and `regexp/syntax` are no longer updater dependencies. Signature, hash, expiry, replay and path checks remain enabled.
- Removed an unused installer-output copy, avoiding retention of up to two MiB of stdout/stderr data plus buffer capacity overhead per installation.
- Removed one redundant version-file sync; the final contents/ownership/mode sync and parent-directory sync remain.
- Release builds retain `CGO_ENABLED=0`, `-trimpath`, `-s -w` and static Linux executables. Stripping debug information is not a runtime-memory optimization.

Measured with Go 1.27.3 and the same stripped release flags: Linux amd64 decreased from 7,598,204 to 7,356,540 bytes (7.02 MiB), a net saving of 241,664 bytes (3.18%) after the hardening changes. The current Linux arm64 build is 6,815,868 bytes (6.50 MiB). Build outputs and static ELF records are in `dist/`.

## Validation

- Pinned Go 1.27.3; module verification, Linux package builds, Linux/Windows vet and current Go vulnerability-database scan pass. The scan reports no known vulnerabilities for the analyzed build; it is not proof of absence.
- Cross-builds pass for Linux amd64/arm64 updaters and macOS amd64/arm64 signers; Linux ELF checks confirm static binaries. The Linux amd64 `version` smoke check passes without accessing installation paths.
- Temporary, isolated regression checks cover parser equivalence (over 180,000 generated/random cases), signed JSON rejection, sequence state, retry classification and actual attempts, archive authentication/cleanup, extraction cancellation/I/O errors, version-file path protections, atomic publisher output and installer output/timeout limits.
- Eleven isolated updater integration scenarios pass, including available -> installed -> up-to-date, signature/hash/size/expiry rejection, replay/equivocation, extraction cancellation, and installer failure or forbidden version-marker changes. Only a harmless local dummy installer was executed.
- Temporary checks use generated keys and disposable directories, not the installed cPGuard version, production state or signing keys. No persistent tests are restored to the production-only tree, as requested; future changes still need regression acceptance.
- Native macOS and Linux arm64 execution, real package installation, power-loss/storage fault injection and race-detector testing are not covered. Cross-compilation is not runtime testing.

## Remaining deployment risks

1. At review time, `internal/site/site.json` contained the development public key and GitHub endpoints. On 2026-11-07 the endpoints were changed to `https://updates.opsshield.com/cpguard/updates.json` and archive host `repo.opsshield.com`, retaining Key A. Independently verify production trust and endpoint publication before release; endpoint reachability is not package or installer approval.
2. A compromised signing key can authorize root code. Keep the private key off client/CDN servers; this single-key flow has no independent approval or standby recovery key.
3. `run.sh` is trusted root code. A failure can occur after application changes; this flow has no application rollback. Later logging/fsync failures also cannot undo a completed installation.
4. Application-owned `swversion` and logs are mutable by the cpguard account. They are not independent, tamper-proof security records. Protected sequence state does not prove the installed application version.
5. The updater executable, cron configuration and sequence state must be root-protected and outside the application-writable tree. Initial distribution and deployment permissions are not established by this audit.
6. CDN compromise can deny service or replay metadata within existing freshness/state limits. Correct time, timely metadata renewal and preservation of sequence state remain required.

No confirmed signature-verification or archive-path escape bypass was identified in the reviewed flow. Do not interpret that as a guarantee against all attacks.
