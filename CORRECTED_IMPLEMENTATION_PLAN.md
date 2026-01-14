# plasmactl-package Layout Detection - CORRECTED Implementation Plan

## Key Corrections

### 1. This is for Open Source, Not Skilld
- This plasmactl-package repo is for **plasmash/open source**
- Skilld teams use their own version (skilld-labs)
- **No backward compatibility needed** - we can break things

### 2. Always Output to Modern Structure
- **ALWAYS** output to `.plasma/package/compose/merged/src/`
- No conditional logic based on package types
- Clean, simple, consistent

### 3. Compose Does NOT Handle Platform Files
- Compose only merges **package components**
- **Prepare** handles platform-specific files (env/, chassis.yaml, ansible.cfg)
- Compose doesn't know about or touch these files

## Simplified Architecture

### What Compose Does
```
Input: Downloaded packages in .plasma/package/compose/packages/
  ├── package1/target/ (legacy: platform/, interaction/ at root)
  └── package2/target/ (modern: src/platform/, src/interaction/)

Output: .plasma/package/compose/merged/src/
  ├── platform/      (merged from all packages)
  ├── interaction/
  └── ...
```

### What Compose Does NOT Do
- ❌ Does not handle env/ directory
- ❌ Does not handle chassis.yaml
- ❌ Does not handle compose.yaml
- ❌ Does not create ansible.cfg
- ❌ Does not create symlinks

**Those are prepare's job!**

## Simplified Implementation

### Step 1: Update Output Constants

**File**: `compose/compose.go` (lines 18-24)

```go
const (
	// MainDir is a compose directory.
	MainDir = ".plasma/package/compose"  // Changed from .compose
	// BuildDir is a result directory of compose action.
	BuildDir = MainDir + "/image/src"  // ALWAYS output to src/
	composeFile = "compose.yaml"
	dirPermissions = 0755
)
```

### Step 2: Add Layout Detection Functions

**File**: `compose/builder.go` (add after line 122)

```go
// detectPackageLayout checks if package uses src/ structure
func detectPackageLayout(pkgPath string) string {
	srcPath := filepath.Join(pkgPath, "src")
	if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
		if hasLayerDirectories(srcPath) {
			return srcPath  // Modern: read from src/
		}
	}
	return pkgPath  // Legacy: read from root
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

### Step 3: Update build() Method to Detect Per-Package Layout

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
componentsPath := detectPackageLayout(pkgPath)
if componentsPath != pkgPath {
	b.Log().Debugf("Package %s: modern layout (src/)", pkgName)
} else {
	b.Log().Debugf("Package %s: legacy layout (root)", pkgName)
}

packageFs := os.DirFS(componentsPath)
```

That's it! Simple and clean.

## What This Does

### For Legacy Package
```
Input: .plasma/package/compose/packages/old-package/v1.0.0/
  ├── platform/
  ├── interaction/
  └── ...

Detection: No src/ directory → read from root
Copy to: .plasma/package/compose/merged/src/platform/, src/interaction/
```

### For Modern Package
```
Input: .plasma/package/compose/packages/new-package/prepare/
  └── src/
      ├── platform/
      ├── interaction/
      └── ...

Detection: Has src/ directory → read from src/
Copy to: .plasma/package/compose/merged/src/platform/, src/interaction/
```

### For Mixed Packages
```
Input:
  ├── old-package/v1.0.0/platform/ (legacy)
  └── new-package/prepare/src/platform/ (modern)

Detection:
  - old-package: read from root
  - new-package: read from src/

Output: .plasma/package/compose/merged/src/
  └── platform/ (merged from both)
```

## Flow Diagram

```
plasmactl package:compose
  ↓
Download packages to .plasma/package/compose/packages/{name}/{target}/
  ↓
For each package:
  ├─ Detect layout (has src/ or not?)
  ├─ Read components from correct location
  └─ Merge to .plasma/package/compose/merged/src/
  ↓
Done! Output: .plasma/package/compose/merged/src/{layers}/

---

plasmactl prepare (separate step)
  ↓
Input: .plasma/package/compose/merged/src/{layers}/
  ↓
Copy platform files (env/, chassis.yaml if they exist)
Create ansible.cfg
Create symlinks
Generate galaxy.yml
  ↓
Output: .plasma/prepare/ (Ansible-ready)
```

## Implementation Steps

### Step 1: Update Constants (2 minutes)
```bash
cd ~/Sources/plasmactl-package
git checkout -b feature/layout-detection
```

Edit `compose/compose.go` lines 18-24:
```go
const (
	MainDir = ".plasma/package/compose"
	BuildDir = MainDir + "/image/src"
	composeFile = "compose.yaml"
	dirPermissions = 0755
)
```

### Step 2: Add Detection Functions (5 minutes)

Add to `compose/builder.go` after line 122:
```go
func detectPackageLayout(pkgPath string) string {
	srcPath := filepath.Join(pkgPath, "src")
	if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
		if hasLayerDirectories(srcPath) {
			return srcPath
		}
	}
	return pkgPath
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

### Step 3: Use Detection in build() (2 minutes)

Edit `compose/builder.go` line 270:
```go
pkgPath := filepath.Join(b.sourceDir, pkgName, targetsMap[pkgName])

// NEW: Detect and use correct path
componentsPath := detectPackageLayout(pkgPath)
if componentsPath != pkgPath {
	b.Log().Debugf("Package %s: modern layout (reading from src/)", pkgName)
} else {
	b.Log().Debugf("Package %s: legacy layout (reading from root)", pkgName)
}

packageFs := os.DirFS(componentsPath)  // Changed from pkgPath
```

### Step 4: Test (10 minutes)

Create test with ski-platform prepare branch.

### Step 5: Commit (1 minute)

```bash
git add -A
git commit -m "feat: add layout detection and output to .plasma/package/compose/merged/src/

- Always output to .plasma/package/compose/merged/src/ directory
- Detect per-package layout (src/ vs root)
- Read components from correct location
- Support mixed modern and legacy packages"
```

## Testing

### Test Case 1: Modern Package (pla-plasma prepare branch)
```bash
cd ~/Sources/ski-platform
git checkout prepare
plasmactl package:compose

# Verify:
ls .plasma/package/compose/merged/
# Expected: src/ directory

ls .plasma/package/compose/merged/src/
# Expected: platform/, interaction/, etc.
```

### Test Case 2: Check Logs
```bash
plasmactl package:compose --debug

# Should see:
# Package plasma-core: modern layout (reading from src/)
# OR
# Package plasma-core: legacy layout (reading from root)
```

## What We're NOT Doing

❌ **No backward compatibility** - This is open source version, break away from .plasma/package/compose/merged
❌ **No platform file handling** - That's prepare's job
❌ **No conditional output paths** - Always .plasma/package/compose/merged/src/
❌ **No checking if packages are modern** - Just detect per-package where to read from

## Benefits

✅ **Simple**: Just 3 small changes
✅ **Clean**: Always same output structure
✅ **Flexible**: Handles both layout types automatically
✅ **Separation**: Compose does compose, prepare does prepare
✅ **Forward-compatible**: Ready for open source adoption

## Total Changes

- **1 file changed for constants**: compose.go (2 lines)
- **1 file changed for detection**: builder.go (~30 lines)
- **Total**: ~32 lines of code

Very minimal, very focused, very clean.

Ready to implement?
