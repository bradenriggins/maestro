# Contributing

Thank you for considering contributing to Maestro. This document covers setup, code standards, and the PR process.

## Development Setup

1. Fork the repository
2. Clone your fork: `git clone https://github.com/bradenmweight/maestro.git`
3. Add the upstream remote: `git remote add upstream https://github.com/bradenmweight/maestro.git`
4. Install dependencies: `go mod download`
5. Build and run: `go build -o maestro . && ./maestro version`

### Prerequisites

- Go 1.23+ (toolchain pinned to 1.24.1)
- tmux
- gh (GitHub CLI)
- jq
- Claude Code and/or Codex CLI

## Code Standards

### Formatting

All Go code must be formatted with `gofmt` before submitting:

```bash
gofmt -w $(rg --files -g '*.go')
```

### Vetting

```bash
go vet ./...
```

### Testing

```bash
go test ./...
```

Please include tests for new features and bug fixes. The project has test coverage across all major packages.

## PR Process

1. Create a feature branch from `main`
2. Make your changes with focused, well-described commits
3. Ensure `gofmt`, `go vet`, and `go test ./...` all pass
4. Open a pull request against `main` with a clear description of what changed and why
5. Sign the CLA when prompted by the CLA assistant bot

## Questions

Open an issue for questions about contributing or the codebase.
