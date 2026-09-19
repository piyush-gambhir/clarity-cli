# Security policy

Security fixes target the latest release. Update before reporting a potential issue.

Please report vulnerabilities privately using [GitHub private vulnerability reporting](https://github.com/piyush-gambhir/clarity-cli/security/advisories/new). Do not include real Clarity tokens, customer recordings, or other private data in a public issue.

This CLI stores project tokens locally in plaintext with owner-only file permissions on Unix; Windows uses the user's directory ACLs. Environment credentials are supported for secret-manager integration. Logs and profile inspection omit credentials. The CLI does not forward credentials through HTTP redirects.

All Clarity operations in this CLI read remote data. Authentication commands change local configuration, and `update` replaces the local executable after checksum verification. Release checksums detect corruption or mismatched downloads; they are not a separate publisher signature.

Microsoft controls the upstream APIs. Report Clarity service vulnerabilities through Microsoft's security reporting process rather than this repository.
