# plasmactl-model

A [Launchr](https://github.com/launchrctl/launchr) plugin for [Plasmactl](https://github.com/plasmash/plasmactl) that manages model composition and preparation for Plasma platforms.

## Overview

`plasmactl-model` handles the composition phase of the Plasma deployment lifecycle. It fetches packages, merges them into a unified model, prepares the runtime environment for Ansible, creates distributable bundles, and manages releases.

## Features

- **Package Composition**: Fetch and merge packages from compose.yaml
- **Package Management**: Add, update, and delete package dependencies
- **Runtime Preparation**: Transform composed model for Ansible deployment
- **Bundle Creation**: Create distributable `.pm` (Platform Model) artifacts
- **Release Management**: Tag and publish model releases with automatic changelog generation

## Commands

### model:compose

Compose packages from compose.yaml:

```bash
plasmactl model:compose
```

Options:
- `-w, --working-dir`: Directory for temporary files
- `-s, --skip-not-versioned`: Skip unversioned files from source
- `--conflicts-verbosity`: Log file conflicts during composition
- `--clean`: Clean working directory before composing
- `-i, --interactive`: Interactive mode for conflict resolution

### model:add

Add a new package dependency:

```bash
plasmactl model:add --package plasma-work --url https://github.com/plasmash/pla-work.git --ref v1.0.0
```

Options:
- `--package`: Package name
- `--url`: Git repository URL
- `--ref`: Git reference (branch, tag, or commit)
- `--type`: Source type (default: git)
- `--strategy`: Merge strategy
- `--strategy-path`: Paths for strategy
- `--allow-create`: Create compose.yaml if it doesn't exist

### model:update

Update an existing package dependency:

```bash
plasmactl model:update --package plasma-core --ref v2.0.0
```

Options:
- `--package`: Package name to update
- `--url`: New Git repository URL
- `--ref`: New Git reference
- `--type`: New source type
- `--strategy`: Merge strategy
- `--strategy-path`: Paths for strategy

### model:delete

Remove package dependencies:

```bash
plasmactl model:delete --packages plasma-legacy
plasmactl model:delete --packages pkg1 --packages pkg2
```

Options:
- `--packages`: Package names to delete (can be specified multiple times)

### model:prepare

Prepare the composed model for Ansible deployment:

```bash
plasmactl model:prepare
```

Options:
- `--compose-dir`: Custom compose directory (default: `.plasma/compose/merged`)
- `--prepare-dir`: Custom prepare directory (default: `.plasma/prepare`)
- `--clean`: Remove existing prepare directory before preparing

This command:
- Copies composed model to `.plasma/prepare/`
- Generates Ansible collection structure with `roles/` directories
- Creates `ansible.cfg` and required symlinks
- Renames `config/` to `group_vars/` for Ansible compatibility

### model:bundle

Create a Platform Model (.pm) artifact for distribution:

```bash
plasmactl model:bundle
```

Creates a distributable archive in `dist/` directory as `{name}-{version}.pm`.

### model:release

Create a git tag with changelog and optionally create a forge release:

```bash
# Preview changes without making any modifications
plasmactl model:release --dry-run

# Bump patch version (default)
plasmactl model:release

# Bump minor version
plasmactl model:release minor

# Bump major version
plasmactl model:release major

# Explicit version
plasmactl model:release v2.0.0

# Create tag only, skip forge release
plasmactl model:release --tag-only

# Create release with specific token
plasmactl model:release --token ghp_xxxx
```

Arguments:
- `version`: Bump type (patch, minor, major) or explicit version (v1.2.3). Defaults to patch bump.

Options:
- `--dry-run`: Preview changelog and actions without making changes
- `--tag-only`: Create and push git tag only, skip forge release
- `--forge-url`: Forge URL for credentials (auto-detected from git remote)
- `--token`: API token (falls back to GITHUB_TOKEN/GITLAB_TOKEN/GITEA_TOKEN env vars, or keyring)

Supported forges:
- GitHub (github.com and GitHub Enterprise)
- GitLab (gitlab.com and self-hosted)
- Gitea
- Forgejo (codeberg.org and self-hosted)

The changelog is automatically generated from conventional commits since the last tag.

## Composition Process

```
compose.yaml → model:compose → model:prepare → model:bundle
                    ↓               ↓               ↓
            .plasma/compose/  .plasma/prepare/   dist/*.pm
```

1. **Compose**: Fetch packages and merge into unified model
2. **Prepare**: Transform for Ansible (add roles/, group_vars/, etc.)
3. **Bundle**: Create distributable artifact

## Configuration

### compose.yaml

Define package dependencies:

```yaml
name: my-platform
dependencies:
  - name: plasma-core
    source:
      type: git
      ref: v1.0.0
      url: https://github.com/plasmash/pla-plasma.git
  - name: plasma-work
    source:
      type: git
      ref: v1.5.0
      url: https://github.com/plasmash/pla-work.git
```

## Directory Structure

After composition and preparation:

```
.plasma/
├── compose/
│   ├── packages/         # Downloaded packages
│   └── merged/           # Merged model
└── prepare/              # Ansible-ready model
    ├── ansible.cfg
    ├── library/
    ├── foundation/
    │   ├── foundation.yaml
    │   └── group_vars/
    └── interaction/
        ├── interaction.yaml
        └── group_vars/
```

## Project Structure

```
plasmactl-model/
├── plugin.go                        # Plugin registration
├── actions/
│   ├── add/
│   │   ├── add.yaml                 # Action definition
│   │   └── add.go                   # Implementation
│   ├── bundle/
│   │   ├── bundle.yaml
│   │   └── bundle.go
│   ├── compose/
│   │   ├── compose.yaml
│   │   └── compose.go
│   ├── delete/
│   │   ├── delete.yaml
│   │   └── delete.go
│   ├── prepare/
│   │   ├── prepare.yaml
│   │   └── prepare.go
│   ├── release/
│   │   ├── release.yaml
│   │   └── release.go
│   └── update/
│       ├── update.yaml
│       └── update.go
└── internal/
    ├── compose/                     # Package composition engine
    │   ├── compose.go
    │   ├── download_manager.go
    │   ├── files_crawler.go
    │   └── ...
    └── release/                     # Release management
        ├── changelog.go             # Conventional commits parsing
        ├── forge.go                 # GitHub/GitLab/Gitea API
        ├── git.go                   # Git operations
        └── semver.go                # Semantic versioning
```

## Workflow Example

```bash
# 1. Add a new package dependency
plasmactl model:add --package plasma-work --url https://github.com/plasmash/pla-work.git --ref v1.0.0

# 2. Compose packages
plasmactl model:compose

# 3. Prepare for deployment
plasmactl model:prepare

# 4. Create bundle (optional)
plasmactl model:bundle

# 5. Create release
plasmactl model:release minor

# 6. Deploy
plasmactl platform:deploy dev
```

## File Extensions

| Extension | Name | Purpose |
|-----------|------|---------|
| `.pm` | Platform Model | Composed bundle artifact |
| `.pi` | Platform Image | Bootable VM image (future) |

## Documentation

- [Plasmactl](https://github.com/plasmash/plasmactl) - Main CLI tool
- [plasmactl-component](https://github.com/plasmash/plasmactl-component) - Component management
- [Plasma Platform](https://plasma.sh) - Platform documentation

## License

[European Union Public License 1.2 (EUPL-1.2)](LICENSE)
