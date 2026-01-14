# plasmactl-package Layout Detection - CORRECT Implementation

## The CORRECT Understanding

### Output: .plasma/package/compose/merged/

**We merge EVERYTHING from packages**, not just layers:

### Modern Package (has src/)
```
Input: package/
  ├── src/
  │   ├── platform/
  │   └── interaction/
  ├── env/
  ├── chassis.yaml
  └── other-files

Output: .plasma/package/compose/merged/
  ├── src/
  │   ├── platform/      (merged from package/src/)
  │   └── interaction/
  ├── env/               (merged from package/ root)
  ├── chassis.yaml       (merged from package/ root)
  └── other-files        (merged from package/ root)

Action: Copy everything as-is
```

### Legacy Package (no src/)
```
Input: package/
  ├── platform/          (layers at root!)
  ├── interaction/
  ├── env/
  ├── chassis.yaml
  └── other-files

Output: .plasma/package/compose/merged/
  ├── src/               (NEW: created for normalization)
  │   ├── platform/      (MOVED from package root)
  │   └── interaction/   (MOVED from package root)
  ├── env/               (merged from package/ root)
  ├── chassis.yaml       (merged from package/ root)
  └── other-files        (merged from package/ root)

Action:
1. Detect layers at root (platform/, interaction/, etc.)
2. Move those layers to src/ subdirectory
3. Copy everything else as-is
```

### Mixed Packages
```
Input:
  ├── modern-package/
  │   ├── src/platform/
  │   └── env/
  └── legacy-package/
      ├── platform/        (at root)
      └── env/

Output: .plasma/package/compose/merged/
  ├── src/
  │   └── platform/        (merged from both: modern/src/ + legacy/ root)
  └── env/                 (merged from both packages)
```

## The Key Insight

**Layers** = `platform/`, `interaction/`, `integration/`, `cognition/`, `conversation/`, `stabilization/`, `foundation/`

**For each package**:
1. If package has `src/` → Copy layers from `src/` to output `src/`
2. If package has NO `src/` → Move layers from root to output `src/`
3. **Always**: Copy non-layer files/dirs from package root to output root

## Implementation Strategy

### Current Problem with Existing Code

The current `builder.go` walks the package and copies everything flat. We need to:
1. **Detect layout** before walking
2. **Adjust paths** based on layout:
   - Modern: Read from package root, copy as-is
   - Legacy: Read from package root, but redirect layers to src/

### Solution: Path Mapping

Instead of detecting once and changing the base path, we need to **map each file's destination** based on whether it's a layer or not.

## Implementation

### Change 1: Update Constants

**File**: `compose/compose.go` (lines 18-20)

```go
const (
	MainDir = ".plasma/package/compose"
	BuildDir = MainDir + "/image"  // Just /image, not /image/src
	composeFile = "compose.yaml"
	dirPermissions = 0755
)
```

### Change 2: Add Helper Functions

**File**: `compose/builder.go` (add after line 122)

```go
// Layer names that should be in src/
var layerNames = map[string]bool{
	"platform":      true,
	"interaction":   true,
	"integration":   true,
	"cognition":     true,
	"conversation":  true,
	"stabilization": true,
	"foundation":    true,
}

// isLayerDirectory checks if a path is a layer directory
func isLayerDirectory(path string) bool {
	// Get the first segment of the path
	segments := strings.Split(filepath.Clean(path), string(filepath.Separator))
	if len(segments) == 0 {
		return false
	}
	firstSegment := segments[0]
	return layerNames[firstSegment]
}

// hasModernLayout checks if package has src/ directory with layers
func hasModernLayout(pkgPath string) bool {
	srcPath := filepath.Join(pkgPath, "src")
	stat, err := os.Stat(srcPath)
	if err != nil || !stat.IsDir() {
		return false
	}

	// Check if src/ contains any layer directories
	for layerName := range layerNames {
		layerPath := filepath.Join(srcPath, layerName)
		if _, err := os.Stat(layerPath); err == nil {
			return true
		}
	}
	return false
}

// adjustDestinationPath adjusts destination based on package layout
// For legacy packages: layers go to src/, others stay at root
// For modern packages: everything copied as-is
func adjustDestinationPath(path string, isModernLayout bool) string {
	if isModernLayout {
		// Modern: keep path as-is
		return path
	}

	// Legacy: if it's a layer, prefix with src/
	if isLayerDirectory(path) {
		return filepath.Join("src", path)
	}

	// Non-layer: keep at root
	return path
}
```

### Change 3: Update build() Method

**File**: `compose/builder.go` (around line 269-315)

Find this section where packages are processed:
```go
for i := 0; i < len(items); i++ {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		pkgName := items[i]
		if pkgName != DependencyRoot {
			pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])
			packageFs := os.DirFS(pkgPath)
```

Add layout detection right after `pkgPath`:
```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])

// NEW: Detect package layout
isModern := hasModernLayout(pkgPath)
if isModern {
	b.Log().Debugf("Package %s: modern layout (has src/)", pkgName)
} else {
	b.Log().Debugf("Package %s: legacy layout (normalizing layers to src/)", pkgName)
}

packageFs := os.DirFS(pkgPath)
```

### Change 4: Update Entry Creation

Find where entries are added (around line 285):
```go
finfo, _ := d.Info()
entry := &fsEntry{Prefix: pkgPath, Path: path, Entry: finfo, Excluded: false, From: pkgName}
```

We need to adjust the path for legacy packages. Change the section to:
```go
finfo, _ := d.Info()

// NEW: Adjust path based on layout
adjustedPath := adjustDestinationPath(path, isModern)

entry := &fsEntry{
	Prefix:   pkgPath,
	Path:     adjustedPath,  // Use adjusted path
	Entry:    finfo,
	Excluded: false,
	From:     pkgName,
}
```

But wait - we need `isModern` to be available in the walkDir closure. Let's capture it:

**Better approach for Change 3 & 4**:

```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])

// Detect package layout
isModern := hasModernLayout(pkgPath)
if isModern {
	b.Log().Debugf("Package %s: modern layout (has src/)", pkgName)
} else {
	b.Log().Debugf("Package %s: legacy layout (normalizing layers to src/)", pkgName)
}

packageFs := os.DirFS(pkgPath)
strategies, ok := ps[pkgName]

// Capture isModern in closure
err = fs.WalkDir(packageFs, ".", func(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	// Skip .git folder from packages
	if strings.HasPrefix(path, gitPrefix) {
		return nil
	}

	var conflictReslv mergeConflictResolve
	finfo, _ := d.Info()

	// NEW: Adjust destination path based on layout
	adjustedPath := adjustDestinationPath(path, isModern)

	entry := &fsEntry{
		Prefix:   pkgPath,
		Path:     adjustedPath,  // Use adjusted path
		Entry:    finfo,
		Excluded: false,
		From:     pkgName,
	}

	if !ok {
		// No strategies for package. Proceed with default merge.
		entriesTree, conflictReslv = addEntries(entriesTree, entriesMap, entry, adjustedPath)
	} else {
		entriesTree, conflictReslv = addStrategyEntries(strategies, entriesTree, entriesMap, entry, adjustedPath)
	}

	if b.logConflicts && !finfo.IsDir() {
		b.logConflictResolve(conflictReslv, adjustedPath, pkgName, entriesMap[adjustedPath])
	}

	return nil
})
```

## Examples of What This Does

### Example 1: Modern Package
```
Input: .plasma/package/compose/packages/plasma-core/prepare/
  ├── src/
  │   ├── platform/applications/
  │   └── interaction/services/
  ├── env/ski-dev/
  └── chassis.yaml

Process:
  - Detect: HAS src/ → isModern = true
  - src/platform/applications/ → src/platform/applications/ (no adjustment)
  - src/interaction/services/ → src/interaction/services/ (no adjustment)
  - env/ski-dev/ → env/ski-dev/ (no adjustment)
  - chassis.yaml → chassis.yaml (no adjustment)

Output: .plasma/package/compose/merged/
  ├── src/
  │   ├── platform/applications/
  │   └── interaction/services/
  ├── env/ski-dev/
  └── chassis.yaml
```

### Example 2: Legacy Package
```
Input: .plasma/package/compose/packages/old-package/v1.0.0/
  ├── platform/applications/      (layer at root!)
  ├── interaction/services/       (layer at root!)
  ├── env/dev/
  └── config.yaml

Process:
  - Detect: NO src/ → isModern = false
  - platform/applications/ → src/platform/applications/ (ADJUSTED!)
  - interaction/services/ → src/interaction/services/ (ADJUSTED!)
  - env/dev/ → env/dev/ (not a layer, stays at root)
  - config.yaml → config.yaml (not a layer, stays at root)

Output: .plasma/package/compose/merged/
  ├── src/                        (CREATED for layers!)
  │   ├── platform/applications/
  │   └── interaction/services/
  ├── env/dev/
  └── config.yaml
```

### Example 3: Mixed Packages
```
Input:
  ├── modern-pkg/
  │   ├── src/platform/app1/
  │   └── env/
  └── legacy-pkg/
      ├── platform/app2/          (at root)
      └── env/

Process:
  - modern-pkg: src/platform/app1/ → src/platform/app1/
  - modern-pkg: env/ → env/
  - legacy-pkg: platform/app2/ → src/platform/app2/ (ADJUSTED!)
  - legacy-pkg: env/ → env/

Output: .plasma/package/compose/merged/
  ├── src/
  │   └── platform/
  │       ├── app1/               (from modern)
  │       └── app2/               (from legacy, normalized!)
  └── env/                        (merged from both)
```

## Implementation Steps

### Step 1: Create Branch
```bash
cd ~/Sources/plasmactl-package
git checkout -b feature/layout-detection
```

### Step 2: Update compose.go
Change BuildDir constant to `.plasma/package/compose/merged`

### Step 3: Add Helper Functions to builder.go
Add `layerNames`, `isLayerDirectory()`, `hasModernLayout()`, `adjustDestinationPath()`

### Step 4: Update build() Method
Add layout detection and use `adjustDestinationPath()` for entries

### Step 5: Test
```bash
cd ~/Sources/ski-platform
git checkout prepare
plasmactl package:compose

ls .plasma/package/compose/merged/
# Expected: src/, env/, chassis.yaml, etc.

ls .plasma/package/compose/merged/src/
# Expected: platform/, interaction/, etc.
```

### Step 6: Commit
```bash
git add -A
git commit -m "feat: normalize legacy packages to src/ layout

- Detect modern (has src/) vs legacy (no src/) packages
- Modern packages: copy everything as-is
- Legacy packages: move layers to src/, keep other files at root
- Output always has src/ with all layers
- Non-layer files stay at root level"
```

## Summary

**What we're doing**:
- ✅ Output to `.plasma/package/compose/merged/` (not `/src`)
- ✅ Merge EVERYTHING from packages (not just layers)
- ✅ Detect per-package if it has src/
- ✅ Modern: Copy as-is
- ✅ Legacy: Move layers to src/, keep others at root
- ✅ Result: Normalized output with src/ + root files

**Total changes**: ~60 lines across 2 files

This is the correct, complete approach!
