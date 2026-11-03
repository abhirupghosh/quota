# 001. Quota handoff — Brainstorm

Status: ready-for-plan
Date: 2026-08-15
Mode: builder
Related spec: docs/projects/001-quota-handoff.md (pending — created by /plan)

Working name: `quota` (CLI + SwiftBar plugin). Rename is cheap.

## Problem Statement

You run Claude and Codex on four machines: this Mac, `ubuntu-box`, `hetzner`, and `hermes`. Each tool has multiple paid subscriptions. The 5-hour limit kills a live session mid-task. Today you `claude /login` / `codex logout` + `codex login`, then try to find the transcript and continue it. Dormant chats (idle at a prompt, or already finished) should stay put. Restarting the session yourself is too much effort.

The tool's job is: the one in-flight session that just hit the 5-hour wall continues on the next subscription, in the same pane, without you doing anything. A Mac menu bar shows live 5-hour and weekly usage so you can see the pool and force a hop if auto-detect misses.

## What Makes This Cool

The laptop can be shut. The host that is running Claude still fails over. You look up and the same conversation is talking again on the next account. Dormant panes do not twitch.

## Constraints

- Personal operator tool. One user. Minimalist.
- Hosts: `macbook`, `ubuntu-box`, `hetzner`, `hermes` (= tailnet node `hermes-node`, MagicDNS `hermes-node.<tailnet>.ts.net`, `<tailscale-ip>`).
- All control-plane traffic is Tailscale only (`<tailnet>`). No public-IP path in the tool.
  - Visible on tailnet: Mac, `ubuntu-box`, `hetzner`, `hermes-node` (hermes).
  - `~/.ssh/config` `Host hermes` still uses public IP `<public-ip>`. Tool SSH should use MagicDNS. Confirm whether :22 is intercepted (ubuntu box needed 2222).
- `ubuntu-box` SSH is port 2222 (Tailscale SSH on :22 hangs headless tools).
- Claude Max 20x and ChatGPT Pro on this Mac are currently `you@work.example`. Other accounts are enrolled later via "login, then save current as…".
- v1 session surface: remote tmux + this Mac's terminal/tmux. Not Cursor, Conductor, or Claude desktop.
- Do not invent a process killer UI. Restart is an implementation detail inside the shim.

## Premises

Agreed 2026-08-15:

1. The job is mid-session 5-hour handoff. Dormant chats are never killed or resumed.
2. A PATH shim on each host is how you launch `claude` / `codex`. You still type the same commands.
3. That host-local shim detects the 5-hour wall, fails over to the next account in a pinned order that still has 5-hour and weekly room, and continues THAT session in the same pane.
4. If every account is exhausted: one notification, error stays in the pane, no retry loop.
5. Named account snapshots are saved from a login on this Mac, then pushed to hosts. Claude and Codex switch independently. As many accounts as you want.
6. Handoff also updates that host's default account so the next new session does not reuse the dead subscription.
7. Menu bar: always show this Mac's Claude + Codex 5-hour remaining. Dropdown has every account's 5-hour and weekly bars, per-host current identity, manual switch, and a backup "Handoff now."
8. v1 does not cover IDE/desktop and does not wait out a quota reset.
9. The Mac menu bar is not in the rescue path. Handoff is host-local so a closed lid does not drop a remote session.
10. Control plane (enroll, push snapshots, poll usage, manual switch) talks to remotes only over Tailscale SSH.

## Approaches Considered

### Approach A: CLI + SwiftBar

Summary: One small CLI installed on every host. It is the `claude`/`codex` shim, the snapshot store, the usage poller, and the Tailscale pusher. A SwiftBar plugin on the Mac reads `quota status --json` and draws `C62 X81`.

Effort: S
Risk: Low
Pros:
- Fastest path that still matches the premises.
- Handoff is testable from a terminal before any menu exists.
- Menu bar is a skin. Can be replaced later without rewriting failover.
Cons:
- Depends on SwiftBar (not installed on this Mac today).
- Plugin menus look like a plugin.
- Keychain integration is clumsier than a native app.
Reuses: existing SSH config + Tailscale MagicDNS; real `claude` / `codex` binaries behind the shim.

### Approach B: Native Swift menu bar + same CLI shim

Summary: Same host-local shim and snapshot store. Mac UI is a real `MenuBarExtra` app with Keychain.

Effort: M
Risk: Low
Pros: Better everyday glance and "Save current as…" UX.
Cons: Two codebases. Slower first useful day.
Reuses: Approach A shim unchanged.

### Approach C: One Go/Rust binary everywhere

Summary: Single artifact is the Linux shim and the Mac tray.

Effort: M
Risk: Med
Pros: One install story.
Cons: Tray polish on macOS fights you. Worse glance than SwiftBar or Swift.

## Recommended Approach

**A — CLI + SwiftBar.** The failure that matters is "session hit the 5-hour wall and did not continue." That lives in the shim. Prove it on `hetzner` first. Put a SwiftBar skin on it. If the glance bothers you, swap the skin for Approach B without touching failover.

### v1 shape (feeds /plan)

- CLI name working: `quota`
- Commands (indicative): `quota save <name> --claude|--codex`, `quota ls`, `quota use --host <h> --claude <name>`, `quota push`, `quota status`, `quota shim claude -- …`
- PATH: `~/.local/bin/claude` and `…/codex` are shims that exec the real binaries and watch that one child.
- Active vs dormant: in-flight request (or a just-observed 5-hour error) = active. Sitting at a prompt = dormant. Ignore dormant.
- On 5-hour error: pick next account in pinned order with remaining 5-hour AND weekly; write that host's current creds; resume the same session id in the same tmux pane / tty; update host default.
- Snapshots live on the Mac, encrypted at rest (Keychain or file+chmod). `quota push` copies them to each host over Tailscale SSH.
- SwiftBar plugin: title `C{pct} X{pct}` from local 5-hour remaining. Menu: account bars (5h + weekly), per-host current Claude/Codex, manual switch, Handoff now, last handoff result.

## Open Questions

- Exact 5-hour error text / exit behavior for current Claude Code and Codex. The shim must match reality, not folklore.
- Exact resume argv: Claude `--resume <session-id>` vs `--continue`. Codex equivalent. Must be captured from a real hit, not guessed.
- Hermes SSH port: does `hermes-node` need 2222 the way `ubuntu-box` does?
- Usage endpoints: Claude OAuth usage (5h + weekly) and ChatGPT/Codex usage. Confirm they work with saved snapshots without an interactive session.
- Snapshot format: which files are sufficient (`~/.claude.json` oauth blob vs full file; `~/.codex/auth.json`). Do not copy session transcripts.
- SwiftBar install is acceptable? (cask, not present now.)
- Per-process creds vs host-global file swap. Host-global is simpler; dormant processes already have tokens in memory so they should stay on the old account until they exit. Confirm that Claude/Codex do not re-read creds mid-prompt.

## Success Criteria

- Hit a 5-hour wall in a tmux pane on `hetzner`. The same conversation continues on the next Claude (or Codex) account in that pane. You do not type anything.
- A second, idle `claude` pane on that host is unchanged.
- Mac lid closed: handoff on `hetzner` still works.
- Menu bar on this Mac always shows two local 5-hour numbers. Opening it shows weekly + remote current identities, no more than a few seconds stale.
- Enroll a third Claude account with "login, save current as…, push." Next handoff can use it. No code change.
- All accounts exhausted: one notification, original error left in the pane, no retry loop.
- No public IP appears in tool config. Hermes is addressed as `hermes-node`.

## Distribution Plan

- Personal only. No app store, no homebrew tap required for v1.
- Mac: install SwiftBar, drop the plugin into SwiftBar's plugin dir, put `quota` on PATH (`~/.local/bin`).
- Remotes: `quota install-host` over Tailscale SSH copies the binary, writes the two shims, and syncs snapshots.
- Updates: replace the binary and re-push. No daemon on the remotes beyond the shim wrapping each session.

## The Assignment

Before /plan or any code:

1. Confirm `ssh` to hermes via `hermes-node` (and the port). Optional: retarget `Host hermes` from `<public-ip>` to that MagicDNS name.
2. Capture, from a real Claude 5-hour hit and a real Codex 5-hour hit (or a saved terminal scrollback if you have one): the visible error string, whether the process exits or sits there, and the exact command that resumes that conversation.
3. On this Mac, log the second Claude account in once, copy the oauth fields aside, log back into the work account. Same for the second Codex account. That is the first pair of snapshots. Do not commit them anywhere.
