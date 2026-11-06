# Signed JSON protocol

This uses the original document's Ed25519 signed-envelope flow, narrowed to the user's single-key policy: only public key A may authenticate a release. There is no fallback key, root/targets/timestamp role, or threshold signature.

## Envelope

update_check.json has exactly two non-null fields:

```json
{"payload":"BASE64_PAYLOAD_BYTES","signature":"BASE64_ED25519_SIGNATURE"}
```

Verify the 64-byte Ed25519 signature over the original decoded payload bytes before interpreting that payload. Do not reserialize downloaded payloads for verification.

The signed payload contains exactly product, version, sequence, issued_at, expires_at, and file. See `examples/payload.json` for its shape; it is an unsigned example, not a publishable release.

Product is cpguard. Version is three decimal components, with major 1..99 and minor/patch 0..99. Sequence is a positive uint64 integer. UTC dates use exact seconds ending in Z; validity is positive and no more than seven days. Expired metadata and issuance more than five minutes ahead of local time are rejected.

The file object contains exactly url, sha256, and size. The digest is 64 lowercase hex characters; size is 1..1,073,741,824 bytes. The archive URL must use HTTPS, an exact compiled allowed CDN hostname, and default port or 443, without credentials, query, or fragment. Metadata requests use their own compiled server URL and do not expand the CDN host list.

Both JSON levels reject duplicate, unknown, missing, incorrectly cased, null, or incorrectly typed fields. The parser also rejects invalid UTF-8, trailing JSON, and excessive nesting.

## Sequence state

`/var/lib/cpguard-updater/state.json` records sequence and the SHA-256 of the accepted payload. State is synced and atomically replaced before comparing the application version or starting a download. Lower sequences fail; an equal sequence is accepted only for the identical payload, allowing retries after download or installation failure. A higher sequence is persisted even if a later step fails.

First installation must invoke bootstrap explicitly. Missing or malformed existing state fails closed. Never delete state to bypass a sequence rejection. This code refuses legacy TUF/transaction state; migration from the previous development build requires an explicit reviewed procedure and a current trusted sequence baseline.

## Installation and limits

The installed integer version encodes major*10000 + minor*100 + patch. Missing, empty, or nonnumeric swversion stops installation. Writes preserve the original owner, permissions, and LF/CRLF/no-newline style, using a same-directory temporary file, fsync, rename, and parent-directory sync.

Metadata is capped at 262,144 bytes, with 32 levels of JSON nesting and a 60-second request budget. Transient failures have at most three attempts within that budget. Archives have a 15-minute timeout and no within-run retries. Both downloads reject redirects and content encoding and require normal TLS certificate checks. The updater embeds maintained fallback CA roots for systems without a CA bundle.

Archive extraction permits only regular files and directories. It rejects absolute/traversal paths, links, devices, unsafe mode bits, duplicate paths, conflicts, missing top-level regular run.sh, and excessive sizes. Limits are 1,073,741,824 bytes per file, 4,294,967,296 total declared extracted bytes, 100,000 entries, and 4,096 bytes per path. Linux capacity checks retain a 67,108,864-byte margin. Archive ownership is ignored; extracted files belong to the updater.

run.sh has a ten-minute timeout. Its stdout and stderr are each bounded to 1,048,576 bytes and logged. Timeout handling kills the process group; the script must not detach installer children. The updater itself calls no external program other than /bin/sh.

## Security boundary

A compromised CDN cannot replace the package or its signed hash without a trusted signing key. It can block downloads or replay still-valid metadata within the limits of the client's recorded state. Expiry assumes a reasonable host clock. Initial updater binaries and their embedded public keys need independently trusted distribution.

Compromise of private key A can authorize a malicious release. One key reduces the number of secrets to manage but is not risk-free, and there is no spare key for recovery. A signed run.sh executes as root. There is no application rollback or crash-recovery journal in this reduced flow. Root compromise, irreversible installer changes, and storage failure are outside its guarantees.
