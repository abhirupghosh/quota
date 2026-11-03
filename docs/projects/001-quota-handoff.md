# 001. Quota handoff

Status: approved
Date: 2026-08-15
Mode: builder
Brainstorm: docs/projects/001-quota-handoff-brainstorm.md
ADR: docs/decisions/0001-host-local-shim-and-swiftbar.md
GitHub: pending

## Problem

You run Claude and Codex on four machines (this Mac, `ubuntu-box`, `hetzner`, `hermes`) against a pool of paid subscriptions. The 5-hour limit kills a live session mid-task. Today the recovery is `claude /login` / `codex logout` + `codex login`, then you hunt the transcript and restart by hand. Dormant chats (idle at a prompt, or already finished) must not move. Asking you to restart the dead session is the thing this tool exists to remove.

Evidence: current local identities are Claude Max 20x and ChatGPT Pro as `you@work.example`; switching already happens via login/logout (zsh history). SSH aliases and a Tailscale tailnet (`<tailnet>`) already exist. `hermes` is the tailnet node `hermes-node` (`hermes-node.<tailnet>.ts.net`, `<tailscale-ip>`). `~/.ssh/config` still points that alias at the public IP `<public-ip>`; the tool should use MagicDNS.

## Goal

Shipped in phases. Later phases do not start until the earlier one is something you actually use.

**Phase 1 (now) — see what is live**
1. For each host (this Mac, `ubuntu-box`, `hetzner`, `hermes-node` / hermes), show the active Claude, Codex, **and Grok** login (email + plan).
2. For each of those accounts, show live usage. Claude and Codex: 5-hour and weekly. Grok: whatever the billing/credits API actually exposes (percent of current period, plus extra windows if present). Same account on three hosts is fetched once.
3. Mac menu bar title: `C62 X81 G71`. Dropdown is the per-host grid.

**Phase 2 — manual switch**
4. `quota use --host <h> --claude|--codex|--grok <name>` writes that host's creds. No process restart yet. Prove this before any auto hop.

**Phase 3 — auto-handoff**
5. A live session that hits the 5-hour (or Grok equivalent) wall continues on the next account in the pinned order, in the same pane. Dormant chats stay put. Works if this Mac is shut.

6. Enrolling a new account (Phase 2+): login on this Mac, `quota save <name>`, `quota push`.

## Non-goals

- Cursor, Conductor, VS Code, Claude desktop, or any IDE-attached session.
- Waiting out a quota reset and auto-resuming later.
- Pairing Claude, Codex, and Grok into one slot. They switch independently.
- Killing or listing processes for you to pick. No checklist UI.
- Phase 2/3 work (manual switch, shims, auto-handoff) before Phase 1 is useful.
- Public-IP SSH. Control plane is Tailscale only.
- Sharing this tool, a homebrew tap, or an app-store build.
- Copying session transcripts between machines.
- Changing how you type `claude` / `codex` beyond a PATH shim with the same argv.

## User Value

You. One operator. The 40-minute remote agent that dies on a 5-hour wall comes back on the next subscription in the same tmux pane. Usage is glanceable so you can hop before the wall, or see why a hop failed.

## Scope

Chosen approach from the brainstorm: **CLI + SwiftBar**. Phase 1 is read-only status. Host-local shim (ADR 0001) is Phase 3.

Supported tools: **Claude, Codex, Grok**. Independent on every host.

Usage is **per account**. The grid is host × tool → identity, then that identity's bars. If hetzner and this Mac share `you@work.example` Claude, one usage fetch, two rows.

### Compliance

No host-repo constitution. Binding rules are the agreed premises in the brainstorm (numbered 1–10) and ADR 0001.

### Implementation plan

**Phase 1 — status (this slice)**

1. Repo bootstrap: git at `~/code/quota`, Go CLI, `README.md`, `CLAUDE.md`.
2. Default hosts: `mac` (local), `hetzner`, `ubuntu-box` (port 2222 from SSH config), `hermes` → `hermes-node`.
3. Identity:
   - Claude: `~/.claude.json` `oauthAccount` (email, org type, rate-limit tier). Token from `~/.claude/.credentials.json` or macOS Keychain service `Claude Code-credentials` at *runtime only*.
   - Codex: `~/.codex/auth.json` JWT email + plan.
   - Grok: `~/.grok/auth.json` email, team_id, expires_at.
   - Remotes: Tailscale SSH, identity fields only. Do not pull token files back to the Mac.
4. Usage (once per unique account email+tool):
   - Claude: `GET https://api.anthropic.com/api/oauth/usage` (`five_hour`, `seven_day`).
   - Codex: `GET https://chatgpt.com/backend-api/wham/usage` (`primary_window` / `secondary_window`).
   - Grok: `GET https://cli-chat-proxy.grok.com/v1/billing?format=credits` (`creditUsagePercent`, period limits). Grok is not a 5-hour/weekly pair; show the real fields.
   - If a remote identity has no matching local token, show identity and `usage: n/a`.
5. `quota status` (table) and `quota status --json`. SwiftBar plugin every 3 minutes. Title `C{n} X{n} G{n}` from this Mac.
6. Cache usage 3 minutes. Never log tokens.

**Phase 2 — manual switch** (after Phase 1 is in daily use)

7. Snapshot store + `quota save` / `use` / `push` for Claude, Codex, and Grok. Future sessions only.

**Phase 3 — auto-handoff** (after Phase 2 is trusted)

8. Spike real 5-hour / Grok-limit error strings and resume argv.
9. PATH shim on each host. Failover + same-pane resume. Dormant ignored.

**Still later**

10. Docs: `README.md`, `CLAUDE.md`. No encyclopedia.

### Suggested CLI surface (Phase 1)

```
quota status
quota status --json
quota status --host mac
quota status --swiftbar
```

Phase 2/3 add `save`, `use`, `push`, `handoff`, `shim`.

## Tests

Phase 1 minimum:

| Layer | What |
| --- | --- |
| Unit | Parse Claude `five_hour`/`seven_day`, Codex `primary_window`/`secondary_window`, Grok billing percent. Identity extractors on token-free fixtures. |
| Integration | `quota status --json` against fixture homes (`QUOTA_HOME_*`). No live network in CI. |
| Manual | `quota status` on this Mac shows three tools. With Tailscale up, remotes show emails. SwiftBar title matches local percents. |

Do not hit live usage endpoints in CI. Fixture the payloads.

## Risks

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Claude/Codex 5-hour UI or resume argv changes | High | High | Spike first. Match exact strings/argv in one module. Fixture from real scrollback. |
| Process sitting at a prompt re-reads creds after we swap host default | Med | High | Spike. If they re-read, isolate the handed-off process with `CLAUDE_CONFIG_DIR` / Codex equivalent so dormant panes keep old tokens. |
| Usage APIs unofficial or need a warm interactive token | Med | Med | Spike with saved snapshots. If weekly is missing, show 5-hour only and treat weekly as "unknown = allowed" until proven. |
| `~/.ssh/config` `hermes` still uses public IP `<public-ip>` | High | Low | Tool config uses `hermes-node`. Optionally retarget the SSH alias to MagicDNS. Check :22 vs 2222. |
| Resume attaches the same transcript to a different billing identity | High | Med | Accepted. That is the product. Do not invent a new session. |
| PATH shim shadows the real binary incorrectly | Med | High | Install records absolute path to the real binary. Shim never execs itself. |
| Token files on remotes | High | High | 0600, allowlisted files, never git, never logs. Push over Tailscale only. |
| SwiftBar not installed / plugin API quirks | Low | Low | Document cask install. CLI remains fully usable without the bar. |

## ADR Needed?

Yes. `docs/decisions/0001-host-local-shim-and-swiftbar.md` — host-local shim owns failover; v1 UI is CLI + SwiftBar, not a native app and not Mac-orchestrated SSH.

## Exit Criteria

**Phase 1**
- [ ] `quota status` prints Claude, Codex, and Grok for this Mac (identity + usage or a clear reason usage is missing).
- [ ] Remotes over Tailscale SSH show at least identity. Shared accounts reuse the Mac usage fetch.
- [ ] SwiftBar title is `C{n} X{n} G{n}` from this Mac. Dropdown is the host grid.
- [ ] No tokens in git or in `quota status` output.
- [ ] README + CLAUDE.md on disk.

**Phase 2/3** (not this slice)
- [ ] Manual `quota use` on a host, next session is the new account.
- [ ] Auto-handoff on a real 5-hour wall; dormant pane untouched.

## Documentation impact

| Doc | Action |
| --- | --- |
| `README.md` | Create |
| `CLAUDE.md`, `AGENTS.md` | Create |
| `docs/decisions/0001-host-local-shim-and-swiftbar.md` | Create (this plan) |
| `docs/projects/001-quota-handoff-spike.md` | Create during spike |
| Architecture / encyclopedia | None. Personal CLI, no shared platform docs. |

## The Assignment (from brainstorm, still blocking a confident shim)

1. Confirm `ssh` to hermes via `hermes-node` (and the port). Optional: retarget `Host hermes` in `~/.ssh/config` from `<public-ip>` to that MagicDNS name.
2. Save a real Claude 5-hour error + resume command, and the Codex equivalent.
3. Snapshot the second Claude and second Codex logins on this Mac. Do not commit them.
