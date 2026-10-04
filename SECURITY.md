# Security policy

Security fixes target the latest release. Update before reporting a potential issue.

Please report vulnerabilities privately using [GitHub private vulnerability reporting](https://github.com/piyush-gambhir/clarity-cli/security/advisories/new). Do not include real Clarity tokens, customer recordings, or other private data in a public issue.

This CLI stores project tokens locally in plaintext with owner-only file permissions on Unix; Windows uses the user's directory ACLs. Environment credentials are supported for secret-manager integration. Logs and profile inspection omit credentials. The CLI does not forward credentials through HTTP redirects.

All Clarity operations in this CLI read remote data. Authentication commands change local configuration, and `update` replaces the local executable after checksum verification. Release checksums detect corruption or mismatched downloads; they are not a separate publisher signature.

Release builds contact GitHub (`api.github.com`) at most once a day to check for a newer release, and only when running in an interactive terminal. The request sends no Clarity data or credentials. It never runs when stderr is not a terminal, when `CI` is set, with `--quiet`, or for development builds, and `CLARITY_NO_UPDATE_NOTIFIER=1` or `NO_UPDATE_NOTIFIER=1` turns it off. Otherwise GitHub is contacted only by `install.sh` and `clarity update`.

Microsoft controls the upstream APIs. Report Clarity service vulnerabilities through Microsoft's security reporting process rather than this repository.
