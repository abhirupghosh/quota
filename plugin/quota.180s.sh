#!/bin/sh
# <swiftbar.title>quota</swiftbar.title>
# <swiftbar.hideAbout>true</swiftbar.hideAbout>
# <swiftbar.hideRunInTerminal>true</swiftbar.hideRunInTerminal>
# <swiftbar.hideSwiftBar>true</swiftbar.hideSwiftBar>
# <swiftbar.hideDisablePlugin>true</swiftbar.hideDisablePlugin>
# <swiftbar.hideLastUpdated>true</swiftbar.hideLastUpdated>
# Refresh every 3 minutes. Title is this Mac's worst usage percent.

set -e

resolve_bin() {
	if [ -n "${QUOTA_BIN:-}" ] && [ -x "$QUOTA_BIN" ]; then
		echo "$QUOTA_BIN"
		return
	fi
	self=$0
	if [ -L "$self" ]; then
		self=$(readlink "$self")
	fi
	root=$(CDPATH= cd -- "$(dirname "$self")/.." && pwd)
	if [ -x "$root/bin/quota" ]; then
		echo "$root/bin/quota"
		return
	fi
	if [ -x "$HOME/.quota/bin/quota" ]; then
		echo "$HOME/.quota/bin/quota"
		return
	fi
	# Not `command -v quota`: /usr/bin/quota is the Unix disk-quota tool.
	for c in "$HOME/.local/bin/quota" "$HOME/go/bin/quota" /opt/homebrew/bin/quota /usr/local/bin/quota; do
		if [ -x "$c" ]; then
			echo "$c"
			return
		fi
	done
}

bin=$(resolve_bin || true)
if [ -z "$bin" ] || [ ! -x "$bin" ]; then
	echo "C— X— G— | sfimage=gauge.with.dots.needle.33percent color=#8E8E93"
	echo "---"
	echo "quota binary not found | color=red disabled=true"
	echo "Run install.sh from the quota repo | disabled=true"
	exit 0
fi
exec "$bin" status --swiftbar
