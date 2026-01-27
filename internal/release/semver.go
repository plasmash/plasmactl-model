package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	HasVPrefix bool
}

var semverRegex = regexp.MustCompile(`^(v)?(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?$`)

// ParseVersion parses a semver string into a Version struct
func ParseVersion(s string) (*Version, error) {
	matches := semverRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid semver: %s", s)
	}

	major, _ := strconv.Atoi(matches[2])
	minor, _ := strconv.Atoi(matches[3])
	patch, _ := strconv.Atoi(matches[4])

	return &Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: strings.TrimPrefix(matches[5], "-"),
		HasVPrefix: matches[1] == "v",
	}, nil
}

// String returns the version as a string
func (v *Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.HasVPrefix {
		s = "v" + s
	}
	return s
}

// Compare compares two versions
// Returns 1 if v > other, -1 if v < other, 0 if equal
func (v *Version) Compare(other *Version) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}
	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}
	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}
	// Prerelease comparison: no prerelease > prerelease
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease < other.Prerelease {
		return -1
	}
	if v.Prerelease > other.Prerelease {
		return 1
	}
	return 0
}

// BumpType represents the type of version bump
type BumpType string

const (
	BumpPatch BumpType = "patch"
	BumpMinor BumpType = "minor"
	BumpMajor BumpType = "major"
)

// Bump returns a new version bumped by the given type
func (v *Version) Bump(bumpType BumpType) *Version {
	newV := &Version{
		Major:      v.Major,
		Minor:      v.Minor,
		Patch:      v.Patch,
		HasVPrefix: v.HasVPrefix,
	}

	switch bumpType {
	case BumpMajor:
		newV.Major++
		newV.Minor = 0
		newV.Patch = 0
	case BumpMinor:
		newV.Minor++
		newV.Patch = 0
	case BumpPatch:
		newV.Patch++
	}

	return newV
}

// IsBumpType checks if a string is a valid bump type
func IsBumpType(s string) bool {
	switch BumpType(s) {
	case BumpPatch, BumpMinor, BumpMajor:
		return true
	}
	return false
}

// InitialVersion returns the initial version (0.1.0)
func InitialVersion() *Version {
	return &Version{Major: 0, Minor: 1, Patch: 0}
}
