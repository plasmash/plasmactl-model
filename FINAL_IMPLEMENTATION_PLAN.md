# plasmactl-package Layout Detection - FINAL Implementation Plan

## The Correct Understanding

### Output Structure
- **Target directory**: `.plasma/package/compose/merged/` (base output)
- **Components go to**: `.plasma/package/compose/merged/src/` (normalized)
- **Normalization**: ALL packages → src/ (regardless of their source layout)

### The Flow

```
Input Packages:
├── old-package/ (legacy: platform/, interaction/ at root)
└── new-package/ (modern: src/platform/, src/interaction/)

Compose Process:
1. Detect: Does package have src/?
   - YES → Read from package/src/
   - NO  → Read from package/ (root)

2. Write: Always to .plasma/package/compose/merged/src/
   - old-package: platform/ → .plasma/package/compose/merged/src/platform/
   - new-package: src/platform/ → .plasma/package/compose/merged/src/platform/

Output: .plasma/package/compose/merged/src/
  ├── platform/      (merged from all packages, normalized)
  ├── interaction/
  └── ...
```

### Why This Matters

**Benefit**: Can reuse old packages without maintaining two layouts
- Old packages (no src/) still work
- Read from their root
- Write to src/ anyway
- Output is always consistent

## Implementation

### Change 1: Update Constants

**File**: `compose/compose.go` (lines 18-24)

```go
const (
	// MainDir is a compose directory.
	MainDir = ".plasma/package/compose"
	// BuildDir is a result directory of compose action.
	BuildDir = MainDir + "/image"  // Base output (NOT including src/)
	composeFile = "compose.yaml"
	dirPermissions = 0755
)
```

**Note**: We output to `.plasma/package/compose/merged` but will adjust the actual write path to `image/src/` in the builder.

### Change 2: Add Layout Detection Functions

**File**: `compose/builder.go` (add after line 122)

```go
// detectPackageLayout checks if package uses src/ structure
// Returns the path to read components from
func detectPackageLayout(pkgPath string) (componentsPath string, isModern bool) {
	srcPath := filepath.Join(pkgPath, "src")
	if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
		if hasLayerDirectories(srcPath) {
			return srcPath, true  // Modern: read from src/
		}
	}
	return pkgPath, false  // Legacy: read from root
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
```

### Change 3: Update createBuilder to Add /src to Target

**File**: `compose/builder.go` (line 144-155)

```go
func createBuilder(c *Composer, targetDir, sourceDir string, packages []*Package) *Builder {
	// Normalize output to src/ subdirectory
	targetDir = filepath.Join(targetDir, "src")

	return &Builder{
		c.WithLogger,
		c.WithTerm,
		c.pwd,
		targetDir,  // Now points to .plasma/package/compose/merged/src/
		sourceDir,
		false,
		false,
		packages,
	}
}
```

### Change 4: Update build() to Use Layout Detection

**File**: `compose/builder.go` (around line 270)

Change this:
```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])
packageFs := os.DirFS(pkgPath)
```

To this:
```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])

// Detect layout and read from correct location
componentsPath, isModern := detectPackageLayout(pkgPath)
if isModern {
	b.Log().Debugf("Package %s: modern layout (reading from src/)", pkgName)
} else {
	b.Log().Debugf("Package %s: legacy layout (reading from root, normalizing to src/)", pkgName)
}

packageFs := os.DirFS(componentsPath)
```

## What This Achieves

### Example 1: Legacy Package
```
Input: .plasma/package/compose/packages/old-package/v1.0.0/
  ├── platform/
  │   └── applications/
  └── interaction/

Detection: No src/ → read from root
Output: .plasma/package/compose/merged/src/
  ├── platform/
  │   └── applications/
  └── interaction/
```

### Example 2: Modern Package
```
Input: .plasma/package/compose/packages/new-package/prepare/
  └── src/
      ├── platform/
      └── interaction/

Detection: Has src/ → read from src/
Output: .plasma/package/compose/merged/src/
  ├── platform/
  └── interaction/
```

### Example 3: Mixed Packages
```
Input:
  ├── old-package/v1.0.0/
  │   └── platform/applications/ (at root)
  └── new-package/prepare/
      └── src/platform/services/ (in src/)

Detection:
  - old-package: read from root
  - new-package: read from src/

Output: .plasma/package/compose/merged/src/
  └── platform/
      ├── applications/ (from old-package root)
      └── services/ (from new-package src/)

Result: Both merged into src/, normalized!
```

## Complete Implementation Steps

### Step 1: Create Branch
```bash
cd ~/Sources/plasmactl-package
git checkout -b feature/layout-detection
git status
```

### Step 2: Update Constants
Edit `compose/compose.go` lines 18-20:
```go
const (
	MainDir = ".plasma/package/compose"
	BuildDir = MainDir + "/image"
	composeFile = "compose.yaml"
	dirPermissions = 0755
)
```

### Step 3: Add Detection Functions
Add to `compose/builder.go` after line 122:
```go
func detectPackageLayout(pkgPath string) (componentsPath string, isModern bool) {
	srcPath := filepath.Join(pkgPath, "src")
	if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
		if hasLayerDirectories(srcPath) {
			return srcPath, true
		}
	}
	return pkgPath, false
}

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
```

### Step 4: Update createBuilder
Edit `compose/builder.go` line 144-155:
```go
func createBuilder(c *Composer, targetDir, sourceDir string, packages []*Package) *Builder {
	// Normalize output to src/ subdirectory
	targetDir = filepath.Join(targetDir, "src")

	return &Builder{
		c.WithLogger,
		c.WithTerm,
		c.pwd,
		targetDir,
		sourceDir,
		false,
		false,
		packages,
	}
}
```

### Step 5: Update build()
Edit `compose/builder.go` around line 270:
```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])

// Detect layout and read from correct location
componentsPath, isModern := detectPackageLayout(pkgPath)
if isModern {
	b.Log().Debugf("Package %s: modern layout (reading from src/)", pkgName)
} else {
	b.Log().Debugf("Package %s: legacy layout (reading from root, normalizing to src/)", pkgName)
}

packageFs := os.DirFS(componentsPath)
```

### Step 6: Test
```bash
cd ~/Sources/ski-platform
git checkout prepare
plasmactl package:compose

# Verify structure
ls .plasma/package/compose/merged/
# Expected: src/ directory

ls .plasma/package/compose/merged/src/
# Expected: platform/, interaction/, etc.
```

### Step 7: Commit
```bash
cd ~/Sources/plasmactl-package
git add -A
git commit -m "feat: normalize all packages to src/ layout in compose output

- Output to .plasma/package/compose/merged/src/ (always)
- Detect per-package layout (src/ vs root)
- Read legacy packages from root
- Read modern packages from src/
- Normalize all to src/ in output
- Enables reusing old packages with new layout"
```

## Summary

**What we're doing**:
- ✅ Always output to `.plasma/package/compose/merged/src/`
- ✅ Detect package layout (has src/ or not)
- ✅ Read from correct location (src/ or root)
- ✅ Normalize everything to src/ in output
- ✅ Profit from old packages without maintaining two layouts

**What changes**:
- Constants: `.plasma/package/compose/merged` (base)
- createBuilder: Add `/src` to targetDir
- Detection: Check if package has src/
- build(): Use detected path for reading

**Total changes**: ~40 lines of code across 2 files

Ready to implement this normalized approach?
