# Security policy

`quota` handles OAuth credentials for Claude, Codex and Grok, so security reports are very welcome.

## Reporting

Please **don't open a public issue** for a vulnerability. Use GitHub's [private vulnerability reporting](https://github.com/abhirupghosh/quota/security/advisories/new) instead. You'll get a reply within a few days.

## What counts

- Any path where a token, refresh token or JWT is printed, logged, cached in plain text outside `~/.config/quota/accounts/`, or sent anywhere other than the provider that issued it.
- Remote SSH scripts returning more than emails and plan names.
- File permission problems on `~/.config/quota/`.
- Command injection through host names, account names or SwiftBar parameters.

## What quota stores

| Path | Contents | Mode |
| --- | --- | --- |
| `~/.config/quota/hosts.json` | Host names and SSH aliases | `0600` |
| `~/.config/quota/accounts/` | Saved Claude login snapshots (OAuth refresh tokens) | `0700` |
| `~/.config/quota/cache/` | Usage percentages and emails, no tokens | `0600` |

Account snapshots are only copied off your Mac, over SSH, when you run `quota push` or `quota use --host`.
