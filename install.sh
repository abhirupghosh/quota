#!/usr/bin/env bash
# Install quota CLI + SwiftBar plugin on this Mac.
#
#   curl -fsSL https://raw.githubusercontent.com/abhirupghosh/quota/main/install.sh | bash
#
# or, from a checkout:  ./install.sh
#
# Env overrides: QUOTA_DIR (default ~/.quota), QUOTA_REPO, SWIFTBAR_PLUGINS.
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "quota's installer targets macOS (SwiftBar menu bar)." >&2
  echo "On Linux you can still build the CLI: go build -o ~/.local/bin/quota ./cmd/quota" >&2
  exit 1
fi

REPO_SLUG="${QUOTA_REPO:-abhirupghosh/quota}"
PLUGIN_DIR="${SWIFTBAR_PLUGINS:-$HOME/Library/Application Support/SwiftBarPlugins}"

here="$(CDPATH= cd -- "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"

if [[ -f "$here/go.mod" ]] && grep -q 'module github.com/abhirupghosh/quota' "$here/go.mod"; then
  INSTALL_DIR="$here"
else
  INSTALL_DIR="${QUOTA_DIR:-$HOME/.quota}"
  if [[ ! -f "$INSTALL_DIR/go.mod" ]]; then
    echo "cloning $REPO_SLUG → $INSTALL_DIR"
    if command -v gh >/dev/null 2>&1; then
      gh repo clone "$REPO_SLUG" "$INSTALL_DIR"
    else
      git clone "https://github.com/${REPO_SLUG}.git" "$INSTALL_DIR"
    fi
  fi
  here="$INSTALL_DIR"
fi

mkdir -p "$here/bin"

build_from_source() {
  echo "building $here/bin/quota"
  (cd "$here" && go build -ldflags "-s -w -X main.version=$(git -C "$here" describe --tags --always 2>/dev/null || echo 0.1.0)" -o bin/quota ./cmd/quota)
}

download_release() {
  local os arch asset
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$(uname -m)" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64) arch=amd64 ;;
    *) echo "unsupported arch: $(uname -m)" >&2; return 1 ;;
  esac
  asset="quota-${os}-${arch}"
  echo "downloading $asset from GitHub Releases"
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    gh release download --repo "$REPO_SLUG" --pattern "$asset" -O "$here/bin/quota" --clobber
  else
    curl -fsSL "https://github.com/${REPO_SLUG}/releases/latest/download/${asset}" -o "$here/bin/quota"
  fi
  chmod +x "$here/bin/quota"
}

if command -v go >/dev/null 2>&1; then
  build_from_source
elif download_release; then
  :
else
  echo "no release binary found. Install Go 1.23+ (brew install go) and re-run." >&2
  exit 1
fi

mkdir -p "$HOME/.local/bin"
ln -sf "$here/bin/quota" "$HOME/.local/bin/quota"

mkdir -p "$PLUGIN_DIR"
ln -sf "$here/plugin/quota.180s.sh" "$PLUGIN_DIR/quota.180s.sh"
chmod +x "$here/plugin/quota.180s.sh" "$here/bin/quota"

# Writes a Mac-only hosts.json if none exists. Add SSH hosts later.
"$here/bin/quota" init >/dev/null

echo
echo "installed:"
echo "  CLI     $here/bin/quota"
echo "  PATH    $HOME/.local/bin/quota  (symlink)"
echo "  plugin  $PLUGIN_DIR/quota.180s.sh"
echo "  hosts   $HOME/.config/quota/hosts.json"
echo
echo "Next:"
echo "  1. (optional) Add SSH hosts to ~/.config/quota/hosts.json — see hosts.example.json"
echo "  2. brew install --cask swiftbar   # if SwiftBar is not installed"
echo "  3. Open SwiftBar and pick: $PLUGIN_DIR"
echo "  4. quota status"
echo
if ! echo ":$PATH:" | grep -q ":$HOME/.local/bin:"; then
  echo "Put ~/.local/bin on PATH if you want 'quota' in a terminal."
  echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
  echo
fi
if [[ ! -d /Applications/SwiftBar.app ]]; then
  echo "SwiftBar is not in /Applications. Install it with:"
  echo "  brew install --cask swiftbar"
  echo
fi
echo "This CLI is named quota and will shadow /usr/bin/quota (disk quotas) when ~/.local/bin is first on PATH."
