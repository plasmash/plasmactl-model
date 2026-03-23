package model

import (
	"errors"
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

const (
	// LockFile is the name of the lock file written to the merged directory.
	LockFile = "compose.lock"
)

var errLockNotExists = errors.New("compose.lock not found, run compose first")

// LockEntry represents a single resolved package in the lock file.
type LockEntry struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	URL        string   `yaml:"url"`
	Ref        string   `yaml:"ref"`
	Path       string   `yaml:"path"`
	RequiredBy []string `yaml:"required_by"`
}

// ComposeLock stores the full resolved dependency list produced by compose.
type ComposeLock struct {
	Packages []LockEntry `yaml:"packages"`
}

// LookupLock reads and parses the lock file from the given merged directory fs.
// Returns an error if the file is absent.
func LookupLock(fsys fs.FS) (*ComposeLock, error) {
	data, err := fs.ReadFile(fsys, LockFile)
	if err != nil {
		return nil, errLockNotExists
	}

	var lock ComposeLock
	if err = yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("compose.lock parsing failed: %w", err)
	}

	return &lock, nil
}
