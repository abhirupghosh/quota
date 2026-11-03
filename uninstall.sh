#!/usr/bin/env bash
# Remove the quota symlinks and SwiftBar plugin. Leaves ~/.config/quota alone
# (it holds your hosts and account snapshots) unless you pass --purge.
set -euo pipefail

PLUGIN_DIR="${SWIFTBAR_PLUGINS:-$HOME/Library/Application Support/SwiftBarPlugins}"

rm -f "$HOME/.local/bin/quota" "$PLUGIN_DIR/quota.180s.sh"
echo "removed ~/.local/bin/quota and the SwiftBar plugin"

if [[ "${1:-}" == "--purge" ]]; then
  rm -rf "$HOME/.config/quota"
  echo "removed ~/.config/quota (hosts + account snapshots)"
else
  echo "kept ~/.config/quota — run with --purge to delete hosts and account snapshots"
fi

if [[ -d "$HOME/.quota" ]]; then
  echo "the checkout at ~/.quota is still there: rm -rf ~/.quota"
fi
