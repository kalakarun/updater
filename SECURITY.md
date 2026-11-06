# Security scope

This updater accepts metadata signed by the single embedded Ed25519 public key A and installs only an archive matching the authenticated size and SHA-256. An empty development key or CDN host list blocks production operations.

Private key A can authorize root-executed code. Keep it off servers and out of the repository, with a protected encrypted backup on the publisher side. There is no standby signing key. Replacing a lost or compromised key requires independently trusted distribution of an updater containing the replacement public key. Initial updater distribution must be independently trusted. Expiry depends on host time; sequence state must not be reset to bypass checks.

The reduced flow has no TUF roles, threshold approval, application health API, or automatic application rollback. A failing run.sh may have already changed files. See docs/installer-contract.md for the exact installation boundary.
