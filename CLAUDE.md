# quota

Personal CLI + SwiftBar plugin. Spec: `docs/projects/001-quota-handoff.md`.

Phase 2 Claude switch is live: `save` / `use` / `push` / `ls`. Future sessions only. Do not implement Codex/Grok switch or the auto-handoff shim until Claude switch is trusted.

Tools: Claude, Codex, Grok. Independent. Usage is per account.

Hosts come from `~/.config/quota/hosts.json` (`quota init`, `quota hosts`). Default if missing is this Mac only.

Never log or commit tokens. Identity extractors must not print JWTs. Remote SSH returns emails and plans only. `~/.config/quota/accounts/` is gitignored.

Tests: `go test ./...`. No live usage calls in CI.

Install: `./install.sh`. GitHub: `abhirupghosh/quota`.
