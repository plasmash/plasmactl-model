# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`plasmactl-model` is a Go-based [Launchr](https://github.com/launchrctl/launchr) plugin for Plasmactl that manages Plasma platform model composition. It handles fetching packages, merging them into unified models, preparing runtime environments for Ansible, creating distributable bundles (.pm files), and managing releases.

## Build & Development Commands

Always use `make` targets instead of direct `go` commands — they install required tooling (gotestfmt, golangci-lint) automatically and ensure consistent build flags.

```bash
make deps          # Install Go dependencies
make build         # Build launchr binary to bin/launchr (CGO_ENABLED=0)
make test          # Run all tests
make test-short    # Run short tests only
make lint          # Run golangci-lint with auto-fix
make clean         # Remove bin/ directory
make all           # deps + test-short + build
DEBUG=1 make build # Build with debug symbols
```

## Local Development

This is a plugin, not a standalone binary. The `go.mod` has `replace` directives pointing to sibling directories that must all be cloned as siblings before anything will compile:

```bash
git clone git@github.com:launchrctl/launchr.git ../launchr
git clone git@github.com:plasmash/plasmactl-component.git ../plasmactl-component
git clone git@github.com:plasmash/plasmactl-platform.git ../plasmactl-platform
```

`make build` also requires a `cmd/launchr/` directory. Symlink it from the launchr repo (already in `.gitignore`):
```bash
ln -s ../launchr/cmd cmd
```

## Architecture

### Plugin Entry Point

`plugin.go` implements `launchr.Plugin`, embeds all action YAML definitions via `//go:embed actions/*/*.yaml`, and registers 10 CLI actions. Each action receives a logger (`action.WithLogger`), terminal (`action.WithTerm`), and optionally a keyring.

### Action Pattern

Each action lives in `actions/<name>/` with two files:
- `<name>.yaml` — Action definition (flags, arguments) loaded by launchr
- `<name>.go` — Implementation struct with `Execute(ctx)` and `Result()` methods

All actions return structured JSON results via `Result()`. Actions: add, bundle, compose, list, prepare, query, release, remove, show, update.

### Core Business Logic (`internal/`)

- **`internal/compose/`** — Package composition engine
  - `compose.go` — `Composer` orchestrates download + merge
  - `builder.go` — Merging with configurable strategies (overwrite-local-file, remove-extra-local-files, ignore-extra-package-files, filter-package-files)
  - `download_manager.go` — Fetches packages via git or HTTP
  - `forms.go` — Interactive forms (charmbracelet/huh) for package operations
  - `git.go` / `http.go` — Source-specific download implementations

- **`internal/release/`** — Release management
  - `forge.go` — Unified API for GitHub, GitLab, Gitea, and Forgejo
  - `changelog.go` — Conventional commits parsing for changelog generation
  - `semver.go` — Semantic versioning with bump types
  - `git.go` — Git tag/branch operations

### Public API (`pkg/model/`)

`pkg/model/model.go` defines core types: `Composition`, `Dependency`, `Package`, `Source`, `Strategy`.

### Prepare Action Embedded Resources

`actions/prepare/` embeds Ansible templates (`ansible.cfg.tmpl`, `galaxy.yml.tmpl`) and a Python library of custom Ansible modules/plugins. Transforms the composed model into an Ansible-ready directory structure with roles/, group_vars/, and generated configuration.

## Key Conventions

- All git repository operations use `git.PlainOpenWithOptions()` with `EnableDotGitCommonDir: true` to support git worktrees.
- Paths like `.plasma/compose/` and `.plasma/prepare/` are configurable via CLI flags (`--working-dir`, `--compose-dir`, `--prepare-dir`), not hardcoded.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (fix:, feat:, etc.).

## Linting

golangci-lint with: dupl, errcheck, goconst, gosec, govet, ineffassign, revive, staticcheck, unused, and goimports formatter. Config in `.golangci.yaml`.
