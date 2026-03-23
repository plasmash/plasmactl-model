# Integration Tests

This directory contains integration tests for the plasmactl-model plugins, using the
[testscript](https://github.com/rogpeppe/go-internal) framework.

## Running Tests

```bash
# Run all integration tests
make test

# Run integration tests keeping $WORK dir for inspection
make test-integration

# Filter to a specific test by name (filename without .txtar)
make test-integration TEST=build_lock
```

## Custom Testscript Commands

### `txtproc` - Text Processing

| Operation       | Usage                                                                |
|-----------------|----------------------------------------------------------------------|
| `replace`       | `txtproc replace 'old' 'new' input.txt output.txt`                   |
| `replace-regex` | `txtproc replace-regex 'pattern' 'replacement' input.txt output.txt` |
| `remove-lines`  | `txtproc remove-lines 'pattern' input.txt output.txt`                |
| `remove-regex`  | `txtproc remove-regex 'pattern' input.txt output.txt`                |
| `extract-lines` | `txtproc extract-lines 'pattern' input.txt output.txt`               |
| `extract-regex` | `txtproc extract-regex 'pattern' input.txt output.txt`               |

### `sleep` - Execution Delay

```bash
sleep <duration>   # e.g. sleep 500ms, sleep 1s, sleep 2m
```

### `dlv` - Delve Debugger

```bash
dlv launchr   # requires binary compiled with -gcflags="all=-N -l"
```

### Enhanced `kill`

Extends the default `kill` with POSIX signal support:

```bash
kill bg-name
kill -TERM bg-name
```

---

## model:compose

Tests for the `model:compose` plugin live in `testdata/compose/` as `.txtar` files.

After a `test-integration` run, inspect `$WORK/platform/.plasma/model/compose/merged/` for build artifacts.

### Test Layout

Each test uses:
- `$WORK/src/<pkg>/` — package source directories
- `$WORK/platform/` — domain directory; compose runs here via `exec sh -c 'cd platform && launchr model:compose --interactive=false'`

### Download

| File                              | What it tests                                                                              |
|-----------------------------------|--------------------------------------------------------------------------------------------|
| `download_flat.txtar`             | Two flat packages via `path` source — both appear in the build                             |
| `download_deep_deps.txtar`        | Deep chain `root → pkg-a → lib-common → lib-base` — all transitive deps downloaded         |
| `download_version_conflict.txtar` | Same package required with different `ref` by two packages — error with `version conflict` |

### Build

| File                              | What it tests                                                                           |
|-----------------------------------|-----------------------------------------------------------------------------------------|
| `build_conflict_last_wins.txtar`  | Same file in two packages — package declared last in YAML wins                          |
| `build_platform_wins.txtar`       | Same file in package and platform dir — platform always wins                            |
| `build_diamond.txtar`             | Diamond dep (`pkg-a` and `pkg-b` both depend on `lib-common`) — dedup + last-wins order |
| `build_conflicts_verbosity.txtar` | `--conflicts-verbosity` flag — conflict lines printed to stdout                         |
| `build_clean.txtar`               | `--clean` flag — packages dir removed; build dir always recreated (stale files gone)    |
| `build_lock.txtar`                | Lock file written to build dir with transitive deps, topological order, correct paths   |

### Strategies

| File                                  | What it tests                                                                                      |
|---------------------------------------|----------------------------------------------------------------------------------------------------|
| `strategy_filter.txtar`               | `filter-package-files` — only whitelisted paths from package enter the build                       |
| `strategy_filter_last_wins.txtar`     | `filter-package-files` + last-wins — later package overwrites within its whitelist                 |
| `strategy_ignore.txtar`               | `ignore-extra-package-files` — matching package files are skipped entirely                         |
| `strategy_ignore_keeps_earlier.txtar` | `ignore-extra-package-files` — earlier package file preserved when later package ignores that path |
| `strategy_remove_local.txtar`         | `remove-extra-local-files` — matching platform files excluded from the build                       |

### Writing Tests

Tests use [txtar format](https://pkg.go.dev/github.com/rogpeppe/go-internal/txtar) to bundle a script with its files.
Use `! exec` for commands expected to fail. See existing tests for examples.
