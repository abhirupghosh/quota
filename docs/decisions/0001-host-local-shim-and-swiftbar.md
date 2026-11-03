# 0001. Host-local shim owns failover; v1 UI is CLI + SwiftBar

Date: 2026-08-15
Status: accepted
GitHub: none
Supersedes: none

## Context

We need mid-session 5-hour handoff for Claude and Codex across this Mac, `ubuntu-box`, `hetzner`, and `hermes` (tailnet node `hermes-node`). A Mac menu bar should show usage and let us enroll or force a hop. The laptop will not always be awake. Dormant chats must not move.

Three build paths were on the table: a small CLI plus SwiftBar, a native Swift menu bar plus the same CLI, or one Go/Rust binary that is both tray and shim.

## Decision

The process that is already running Claude or Codex on that host performs the handoff. A PATH shim wraps `claude` / `codex`, detects that child's 5-hour wall, swaps to the next saved account, and resumes that session in the same pane.

The Mac menu bar is a control plane and glance only. v1 is a CLI (`quota`) plus a SwiftBar plugin that reads `quota status --json`. We do not put the Mac on the rescue path. We do not build a native Swift app in v1. We do not scrape tmux panes from the outside.

## Consequences

### Positive

- A closed Mac does not drop a remote session.
- Failover is testable from a terminal before any menu exists.
- The menu bar can be replaced later (native Swift) without rewriting handoff.
- Dormant panes are ignored because the shim only watches its own child.

### Negative

- Every host must install the shim. Sessions started with a raw binary path bypass handoff.
- SwiftBar is a third-party dependency and is not installed on this Mac yet.
- Keychain and menu polish are worse than a native app.

### Neutral

- Claude and Codex stay independent. Account count is unbounded.
- Control plane SSH is Tailscale only.
- Hermes is in v1 as MagicDNS `hermes-node`. `~/.ssh/config` may still use the public IP; the tool does not.

## Alternatives considered

- **Mac-orchestrated SSH handoff.** Simpler brain, fails when the lid is shut. Rejected.
- **Native Swift menu bar in v1.** Better glance, two codebases, slower first useful day. Deferred. The shim stays the same if we do this later.
- **One Go/Rust tray+shim binary.** One artifact, weak macOS tray. Rejected for v1.
- **Outside tmux/TTY scraper, no shim.** Zero remote install, brittle when error copy changes. Rejected.
