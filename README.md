# plasmactl-model

A [Launchr](https://github.com/launchrctl/launchr) plugin for [Plasmactl](https://github.com/plasmash/plasmactl) that manages model composition and preparation for Plasma platforms.

## Overview

`plasmactl-model` handles the composition phase of the Plasma deployment lifecycle. It fetches packages, merges them into a unified model, prepares the runtime environment for Ansible, and creates distributable bundles.

## Features

- **Package Composition**: Fetch and merge packages from compose.yaml
- **Runtime Preparation**: Transform composed model for Ansible deployment
- **Bundle Creation**: Create distributable `.pm` (Platform Model) artifacts
- **Release Management**: Tag and publish model releases

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

### model:prepare

Prepare the composed model for Ansible deployment:

```bash
plasmactl model:prepare
```

Options:
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

Create a git tag with changelog and optionally upload artifact:

```bash
# Preview changelog
plasmactl model:release --preview

# Create tag only
plasmactl model:release --skip-upload

# Create tag and upload to forge
plasmactl model:release --token <your-pat>
```

Options:
- `--tag`: Custom version tag (default: auto-increment)
- `--preview`: Preview changelog without creating tag
- `--skip-upload`: Create tag only, skip forge release
- `--token`: API token for forge (GitHub/GitLab/Gitea)

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

## Workflow Example

```bash
# 1. Compose packages
plasmactl model:compose

# 2. Prepare for deployment
plasmactl model:prepare

# 3. Create bundle (optional)
plasmactl model:bundle

# 4. Deploy
plasmactl platform:deploy dev
```

## File Extensions

| Extension | Name | Purpose |
|-----------|------|---------|
| `.pm` | Platform Model | Composed bundle artifact |
| `.pi` | Platform Image | Bootable VM image (future) |

## Documentation

- [Plasmactl](https://github.com/plasmash/plasmactl) - Main CLI tool
- [plasmactl-package](https://github.com/plasmash/plasmactl-package) - Package management
- [Plasma Platform](https://plasma.sh) - Platform documentation

## License

[European Union Public License 1.2 (EUPL-1.2)](LICENSE)
