# Contributing

Thanks for wanting to help. `quota` is small on purpose, so the bar for new features is "does this make the menu more useful without making it noisier?"

## Setup

```bash
git clone https://github.com/abhirupghosh/quota.git
cd quota
make test
make build   # bin/quota
```

Go 1.23+, standard library only. Please don't add dependencies without opening an issue first.

To try your build in the menu bar, run `./install.sh` from your checkout. It links SwiftBar to your `bin/quota`.

## Ground rules

- **Never log, print or commit tokens.** Identity extractors return emails and plan names, never JWTs or refresh tokens. Tests use fake data.
- **No live network calls in tests.** CI must pass offline.
- **Remote hosts return emails and plans only.** Anything that runs over SSH must keep that promise.
- Run `gofmt -w .` and `go vet ./...` before you push. CI checks both.
- Keep the SwiftBar title short. macOS hides menu bar items that get too wide.

## Adding a tool or provider

1. Read the login in `internal/identity` (email + plan only).
2. Fetch usage in `internal/usage` and return `Window`s with a label (`5h`, `wk`, `credits`, …).
3. Mirror both in the remote Python in `internal/status/status.go` so SSH hosts report it too.
4. Add a letter for it in `internal/status/swiftbar.go`.

## Pull requests

- One change per PR, with a line on *why*.
- Add or update a test when you change behaviour.
- Screenshots help for anything that changes the menu.

Design notes live in [`docs/`](docs/).
