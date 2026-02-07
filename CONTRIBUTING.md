# Contributing to tote

Thank you for considering contributing to tote. This project handles a sharp problem — emergency image salvage in Kubernetes — and contributions must respect the constraints that come with it.

## Ground rules

1. **tote is an emergency tool, not normal infrastructure.** Any feature that makes tote feel like "standard plumbing" is going in the wrong direction.

2. **Detection before action.** Every new capability should detect and report before automating. If it can't be reversed, it needs explicit consent.

3. **Loud by default.** tote must never operate silently. Every action produces events, metrics, and logs.

4. **Opt-in only.** No feature should activate without explicit user annotation or configuration. Default behavior is "do nothing."

## Development setup

```bash
git clone https://github.com/ppiankov/tote.git
cd tote
make deps
make build
make test
```

### Requirements

- Go 1.24+
- golangci-lint

### Verify before submitting

```bash
make test         # must pass with -race
make lint         # must pass clean
make vet          # must pass clean
make fmt          # format code
```

All four must pass. Pull requests with failing tests or lint errors will not be reviewed.

## Code conventions

- **Go files**: `snake_case.go`
- **Packages**: short, single-word names (`detector`, `resolver`, `inventory`)
- **Entry point**: `cmd/tote/main.go` is minimal — all logic lives in `internal/`
- **No `init()` functions** unless absolutely necessary
- **No global mutable state** — configuration and dependencies are struct fields
- **Error handling**: always check returned errors, never suppress
- **Comments**: explain "why" not "what"
- **Tests**: mandatory for all new code, alongside the source file

## Commit messages

Format: `type: concise imperative statement`

```
feat: add namespace-level rate limiting
fix: handle nil pod annotations
test: add inventory edge cases for empty nodes
docs: update RBAC requirements
refactor: extract digest parsing to resolver
chore: update controller-runtime to v0.24
```

- One line, max 72 characters
- Lowercase after the colon, no period
- Say what changed, not every detail of how

## Pull requests

1. Fork the repository and create a feature branch from `main`
2. Make your changes — keep them focused and small
3. Write tests for new functionality
4. Run the full verification suite (test, lint, vet)
5. Open a PR with a clear title and description

### PR description should include

- **What** the change does (one sentence)
- **Why** the change is needed
- **How** to test it (if not obvious from the test suite)

## What we will not accept

- Features that make tote operate silently or automatically without opt-in
- Tag-based image matching (digest-only is a hard requirement)
- Changes that weaken the double opt-in (namespace + pod annotations)
- Modifications to denied namespaces behavior (kube-system is always excluded)
- Backwards-compatibility hacks — if something is unused, delete it

## Reporting issues

Use [GitHub Issues](https://github.com/ppiankov/tote/issues). Include:

- Kubernetes version
- tote version (`tote --version`)
- What you expected to happen
- What actually happened
- Relevant events or logs

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
