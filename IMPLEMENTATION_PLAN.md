# plasmactl-package Layout Detection Implementation Plan

## Code Review Summary

### Current Architecture

**File Structure**:
```
plasmactl-package/
├── compose/
│   ├── compose.go         # Main Composer with Install() method
│   ├── builder.go         # Builder with build() - does the actual merging
│   ├── yaml.go            # Package/Dependency structs
│   ├── downloadManager.go # Downloads packages from git/http
│   ├── git.go            # Git operations
│   ├── http.go           # HTTP operations
│   └── forms.go          # Interactive forms
└── plugin.go              # Launchr plugin entry point
```

**Current Flow**:
1. `Composer.Install()` is called (compose.go:148)
2. Downloads packages to `.plasma/package/compose/packages/{packageName}/{target}/`
3. Creates `Builder` with:
   - `targetDir`: `.plasma/package/compose/merged/` (hardcoded constant)
   - `sourceDir`: `.plasma/package/compose/packages/`
   - `packages`: Downloaded package list
4. `Builder.build()` walks through packages and merges to `.plasma/package/compose/merged/`

**Key Constants** (compose.go:18-24):
```go
const (
    MainDir = ".plasma/package/compose"
    BuildDir = MainDir + "/build"  // Currently: .plasma/package/compose/merged
    composeFile = "compose.yaml"
    dirPermissions = 0755
)
```

**Merging Logic** (builder.go:183-345):
- Walks platform dir first (domain repo)
- Then walks each package in dependency order
- Copies files to `targetDir` (.plasma/package/compose/merged)
- Handles merge strategies and conflicts
- **Key**: Line 270 `pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])`
  - This is where package content is read from
  - Currently reads from package root directly

## Implementation Strategy

### Where to Add Layout Detection

**Option 1: In Builder.build()** (Recommended)
- Detect layout for each package as it's being merged
- Adjust source path based on layout
- Minimal changes, localized impact

**Option 2: In DownloadManager**
- Detect layout after download
- Store layout info in Package struct
- More invasive, affects yaml.go

**Option 3: Change Output Constants**
- Change `BuildDir` from `.plasma/package/compose/merged` to `.plasma/package/compose/merged`
- Detect if ANY package is modern, output to `src/` subdirectory
- Affects all users immediately

### Recommended Approach: Hybrid

1. **New output structure** (if any package is modern):
   ```
   .plasma/package/compose/merged/src/  # Modern output
   ```
   vs
   ```
   .plasma/package/compose/merged/  # Legacy output (all packages are legacy)
   ```

2. **Layout detection per package**:
   ```go
   // In builder.go, around line 270
   pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])

   // NEW: Detect layout
   componentsPath := detectPackageLayout(pkgPath)
   packageFs := os.DirFS(componentsPath)
   ```

3. **Helper function**:
   ```go
   func detectPackageLayout(pkgPath string) string {
       srcPath := filepath.Join(pkgPath, "src")
       if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
           // Check if src/ contains layer directories
           if hasLayerDirectories(srcPath) {
               return srcPath  // Modern: use src/
           }
       }
       return pkgPath  // Legacy: use root
   }

   func hasLayerDirectories(path string) bool {
       layers := []string{"platform", "interaction", "integration",
                         "cognition", "conversation", "stabilization", "foundation"}
       for _, layer := range layers {
           layerPath := filepath.Join(path, layer)
           if _, err := os.Stat(layerPath); err == nil {
               return true
           }
       }
       return false
   }
   ```

## Detailed Implementation Steps

### Step 1: Add Layout Detection Functions

**File**: `compose/builder.go` (add after line 122)

```go
// detectPackageLayout checks if package uses src/ structure
func detectPackageLayout(pkgPath string) (string, string) {
	srcPath := filepath.Join(pkgPath, "src")
	if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
		if hasLayerDirectories(srcPath) {
			return srcPath, "modern"
		}
	}
	return pkgPath, "legacy"
}

// hasLayerDirectories checks for platform layer directories
func hasLayerDirectories(path string) bool {
	layers := []string{
		"platform", "interaction", "integration",
		"cognition", "conversation", "stabilization", "foundation",
	}
	for _, layer := range layers {
		layerPath := filepath.Join(path, layer)
		if _, err := os.Stat(layerPath); err == nil {
			return true
		}
	}
	return false
}

// hasModernPackages checks if any package uses modern layout
func hasModernPackages(packages []*Package, sourceDir string, targetsMap map[string]string) bool {
	for _, pkg := range packages {
		pkgPath := filepath.Join(sourceDir, pkg.GetName(), targetsMap[pkg.GetName()])
		if _, layout := detectPackageLayout(pkgPath); layout == "modern" {
			return true
		}
	}
	return false
}
```

### Step 2: Update Builder Struct

**File**: `compose/builder.go` (line 124-134)

Add field to track if we're using modern output:

```go
type Builder struct {
	action.WithLogger
	action.WithTerm

	platformDir      string
	targetDir        string
	sourceDir        string
	skipNotVersioned bool
	logConflicts     bool
	packages         []*Package
	modernOutput     bool  // NEW: Track if outputting to src/
}
```

### Step 3: Update createBuilder Function

**File**: `compose/builder.go` (line 144-155)

```go
func createBuilder(c *Composer, targetDir, sourceDir string, packages []*Package) *Builder {
	// NEW: Determine if we need modern output structure
	targetsMap := getTargetsMap(packages)
	useModern := hasModernPackages(packages, sourceDir, targetsMap)

	// Adjust targetDir if using modern layout
	if useModern {
		targetDir = filepath.Join(targetDir, "src")
	}

	return &Builder{
		c.WithLogger,
		c.WithTerm,
		c.pwd,
		targetDir,
		sourceDir,
		false,
		false,
		packages,
		useModern,  // NEW
	}
}
```

### Step 4: Update build() Method

**File**: `compose/builder.go` (around line 270)

Change this:
```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])
packageFs := os.DirFS(pkgPath)
```

To this:
```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])

// NEW: Detect and handle layout
componentsPath, layout := detectPackageLayout(pkgPath)
if layout == "modern" {
	b.Log().Debugf("Package %s: modern layout (src/)", pkgName)
} else {
	b.Log().Debugf("Package %s: legacy layout (root)", pkgName)
}

packageFs := os.DirFS(componentsPath)
```

### Step 5: Update Output Constants

**File**: `compose/compose.go` (lines 18-24)

Change BuildDir to match new .plasma structure:

```go
const (
	// MainDir is a compose directory.
	MainDir = ".plasma/package/compose"  // Changed from .compose
	// BuildDir is a result directory of compose action.
	BuildDir       = MainDir + "/image"  // Changed from /build
	composeFile    = "compose.yaml"
	dirPermissions = 0755
)
```

### Step 6: Copy Platform Files

**File**: `compose/builder.go` (after line 345, in build() method)

Add logic to copy platform-specific files to root (not src/):

```go
// After all packages are merged...

// Copy platform-specific files to output root (if modern layout)
if b.modernOutput {
	platformFiles := []string{"env", "chassis.yaml", "compose.yaml"}
	outputRoot := filepath.Dir(b.targetDir)  // Go up from /src to root

	for _, file := range platformFiles {
		srcPath := filepath.Join(b.platformDir, file)
		if _, err := os.Stat(srcPath); err == nil {
			destPath := filepath.Join(outputRoot, file)
			// Copy file or directory
			if info, _ := os.Stat(srcPath); info.IsDir() {
				// Copy directory recursively
				err := copyDir(srcPath, destPath)
				if err != nil {
					b.Log().Warnf("Failed to copy %s: %v", file, err)
				}
			} else {
				// Copy file
				err := fcopy(srcPath, destPath)
				if err != nil {
					b.Log().Warnf("Failed to copy %s: %v", file, err)
				}
			}
		}
	}
}

return nil
```

### Step 7: Add copyDir Helper

**File**: `compose/builder.go` (after the build() method)

```go
// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return fcopy(path, dstPath)
	})
}
```

## Testing Plan

### Test 1: Legacy Package Only
```yaml
# compose.yaml
dependencies:
  - name: old-package
    source:
      ref: v1.0.0
      url: https://example.com/old-package.git
```

**Expected**:
- Output to `.plasma/package/compose/merged/` (legacy path)
- Components at root level
- No src/ directory

### Test 2: Modern Package Only
```yaml
dependencies:
  - name: new-package
    source:
      ref: prepare
      url: https://github.com/plasmash/plasma-core.git
```

**Expected**:
- Output to `.plasma/package/compose/merged/src/`
- Components inside src/
- Platform files (env/, chassis.yaml) at `.plasma/package/compose/merged/` root

### Test 3: Mixed Packages
```yaml
dependencies:
  - name: plasma-core
    source:
      ref: prepare  # Modern
      url: https://github.com/plasmash/plasma-core.git
  - name: old-package
    source:
      ref: v1.0.0   # Legacy
      url: https://example.com/old-package.git
```

**Expected**:
- Output to `.plasma/package/compose/merged/src/`
- All components merged into src/ (normalized)
- Platform files at root

## Backward Compatibility

### For Skilld Teams

**No changes needed** if using only legacy packages:
- Still outputs to `.plasma/package/compose/merged/`
- No src/ directory created
- Everything works as before

### Migration Path

**When switching to modern packages**:
1. Add one modern package to dependencies
2. Compose automatically switches to modern output
3. Update downstream tools to look in `.plasma/package/compose/merged/` instead of `.plasma/package/compose/merged/`
4. Prepare action handles both layouts automatically

## Implementation Checklist

- [ ] Add `detectPackageLayout()` function
- [ ] Add `hasLayerDirectories()` helper
- [ ] Add `hasModernPackages()` check
- [ ] Update `Builder` struct with `modernOutput` field
- [ ] Update `createBuilder()` to detect and set modernOutput
- [ ] Update `build()` to use `detectPackageLayout()` per package
- [ ] Update constants (MainDir, BuildDir)
- [ ] Add platform files copying logic
- [ ] Add `copyDir()` helper
- [ ] Add logging for layout detection
- [ ] Test with legacy packages only
- [ ] Test with modern packages only
- [ ] Test with mixed packages
- [ ] Update README.md
- [ ] Update documentation

## Questions / Decisions

1. **Should we keep backward compatibility with `.plasma/package/compose/merged`?**
   - YES: If all packages are legacy, output to `.plasma/package/compose/merged`
   - This ensures Skilld teams don't break

2. **What if platform repo also has src/ directory?**
   - Platform repo (domain repo) is different from packages
   - Platform repo files should be copied from root, not src/
   - Only check package layouts, not platform layout

3. **Should we add a flag to force legacy/modern output?**
   - Not needed initially - automatic detection is sufficient
   - Can add later if users request it

4. **What about .plasma/package/compose/packages/ directory?**
   - Keep as-is - this is just storage for downloaded packages
   - Layout detection happens at merge time, not download time

## Next Steps

1. **Create feature branch**: `feature/layout-detection`
2. **Implement step by step** following this plan
3. **Test each step** before moving to next
4. **Document changes** in commit messages
5. **Update README** when complete

Ready to implement?
