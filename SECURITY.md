# Security policy

## Supported versions

Security fixes are provided for the latest released minor version. Pre-release builds and source snapshots are supported on a best-effort basis.

## Reporting a vulnerability

Do not open a public issue for suspected vulnerabilities and do not include real credentials, Keychain contents, connection strings, or sensitive logs in any report.

Use [GitHub private vulnerability reporting](https://github.com/theronburger/key-session/security/advisories/new). Include the affected version, macOS version and architecture, impact, safe reproduction steps, and any proposed mitigation. Use synthetic secrets only.

The project aims to acknowledge a complete report within three business days. Details stay private until a fix and coordinated disclosure plan are ready.

## Release trust

Official release archives are Developer ID-signed and Apple-notarized. Each release also includes SHA-256 checksums, a CycloneDX SBOM, and a GitHub artifact attestation. Verify all four when the distribution channel is untrusted.
