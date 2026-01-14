# plasmactl-package

A [Launchr](https://github.com/launchrctl/launchr) plugin for [Plasmactl](https://github.com/plasmash/plasmactl) that manages multi-package composition for Plasma platforms.

## Overview

`plasmactl-package` enables building Plasma platforms from multiple packages and repositories. It recursively fetches dependencies defined in `compose.yaml` files and intelligently merges them into a unified platform structure.

## Features

- **Multi-Package Composition**: Combine components from multiple repositories
- **Recursive Dependency Resolution**: Automatically fetch and process package dependencies
- **Smart File Merging**: Configurable strategies for handling file conflicts
- **Version Control Integration**: Support for Git branches, tags, and HTTP sources
- **Interactive Mode**: Prompt for credentials when needed

## Usage

### Basic Composition

```bash
plasmactl package:compose
```

### Advanced Options

```bash
plasmactl package:compose \
  --working-dir ./custom-dir \
  --skip-not-versioned \
  --conflicts-verbosity \
  --interactive=false
```

### Command Options

- `-w, --working-dir`: Directory for temporary files (default: `.plasma/compose/packages`)
- `-s, --skip-not-versioned`: Skip unversioned files from source (git only)
- `--conflicts-verbosity`: Log file conflicts during composition
- `--interactive`: Enable interactive credential prompts (default: `true`)

## Configuration File

### `compose.yaml` Format

Define platform dependencies in a `compose.yaml` file:

```yaml
name: my-platform
version: 1.0.0
dependencies:
  - name: pla-plasma
    source:
      type: git
      ref: main  # branch or tag
      url: https://github.com/plasmash/pla-plasma.git

  - name: custom-package
    source:
      type: git
      ref: v2.1.0
      url: https://github.com/example/custom-package.git
      strategy:
        - name: overwrite-local-file
          path:
            - config/override.yaml
        - name: ignore-extra-package-files
          path:
            - library/inventories/*.yaml
```

### Merge Strategies

Control how files are merged when conflicts occur:

#### `overwrite-local-file`
Replace local files with package files at specified paths:
```yaml
strategy:
  - name: overwrite-local-file
    path:
      - config/settings.yaml
      - templates/*.j2
```

#### `remove-extra-local-files`
Remove local files not present in the package:
```yaml
strategy:
  - name: remove-extra-local-files
    path:
      - generated/
```

#### `ignore-extra-package-files`
Preserve local files, ignore package versions:
```yaml
strategy:
  - name: ignore-extra-package-files
    path:
      - library/inventories/**/*.yaml
      - secrets/vault.yaml
```

#### `filter-package-files`
Selectively include package files:
```yaml
strategy:
  - name: filter-package-files
    path:
      - specific/required/*.yaml
```

**Default Behavior**: Without strategies, local files take precedence over package files.

## Composition Process

1. **Check Cache**: Verify if packages exist locally and are up-to-date
2. **Fetch Packages**: Download from Git, HTTP, or other sources
3. **Extract Contents**: Unpack packages to working directory
4. **Process Dependencies**: Recursively process `compose.yaml` files
5. **Merge Filesystems**: Combine packages using configured strategies
6. **Repeat**: Process all dependencies recursively

## Package Management Commands

### Add Package

```bash
# Interactive mode
plasmactl package:add

# With flags
plasmactl package:add \
  --package my-package \
  --url https://github.com/example/package.git \
  --ref v1.0.0 \
  --type git
```

### Update Package

```bash
# Interactive mode
plasmactl package:update

# With flags
plasmactl package:update \
  --package my-package \
  --url https://github.com/example/package.git \
  --ref v2.0.0
```

### Delete Package

```bash
plasmactl package:delete my-package other-package
```

### Strategy Examples

Add package with merge strategies:

```bash
# Single strategy
plasmactl package:add \
  --package my-package \
  --url https://github.com/example/package.git \
  --ref main \
  --strategy overwrite-local-file \
  --strategy-path "path1|path2"

# Multiple strategies
plasmactl package:add \
  --package my-package \
  --url https://github.com/example/package.git \
  --ref main \
  --strategy overwrite-local-file,remove-extra-local-files \
  --strategy-path "path1|path2,path3|path4"

# Multiple strategy definitions
plasmactl package:add \
  --package my-package \
  --url https://github.com/example/package.git \
  --ref v1.0.0 \
  --strategy overwrite-local-file \
  --strategy-path "path1|path2" \
  --strategy remove-extra-local-files \
  --strategy-path "path3|path4"
```

## Workflow Example

```bash
# 1. Define dependencies in compose.yaml

# 2. Fetch and compose all packages
plasmactl package:compose --conflicts-verbosity

# 3. Verify composition
ls .plasma/compose/merged/

# 4. Proceed with bump and deployment
plasmactl component:bump && plasmactl component:sync
plasmactl platform:ship dev platform.interaction.observability
```

## Source Types

### Git Repositories
```yaml
source:
  type: git
  ref: main  # or tag: v1.0.0
  url: https://github.com/organization/repo.git
```

### HTTP Archives
```yaml
source:
  type: http
  url: https://example.com/package.tar.gz
```

## Best Practices

1. **Pin Versions**: Use specific tags or commit SHAs instead of branch names for reproducible builds
2. **Document Strategies**: Comment why specific merge strategies are used
3. **Version Control**: Commit `compose.yaml` to track platform composition
4. **Test Locally**: Run `package:compose` before `component:bump` to catch issues early

## Documentation

- [Plasmactl](https://github.com/plasmash/plasmactl) - Main CLI tool
- [Plasma Platform](https://plasma.sh) - Platform documentation
- [Full Merge Strategy Examples](https://github.com/plasmash/plasmactl-package/blob/main/example/compose.example.yaml)

## License

Apache License 2.0
