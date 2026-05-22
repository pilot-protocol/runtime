# Contributing

Thanks for your interest in contributing to `runtime` — Pilot Protocol runtime — glue layer that wires handshake, policy, identity, and event/stream subsystems into a daemon.

## Quick start

```bash
git clone https://github.com/pilot-protocol/runtime.git
cd runtime
go test -race ./...
```

## Pull requests

1. Open an issue first for non-trivial changes so design can be discussed.
2. Branch off `main`; keep changes focused and self-contained.
3. Tests are required for new behavior; passing CI is required to merge.
4. Coverage should not regress (Codecov reports per-PR delta).
5. Conventional commit style is preferred (`feat:`, `fix:`, `docs:`, `chore:`, …) but not enforced.

## Code of conduct

Be respectful and constructive. Project maintainers will moderate.

## License

By contributing you agree your contributions will be released under the
project's license (AGPL-3.0-or-later — see `LICENSE`).
