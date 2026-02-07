// Package model provides types for managing platform model composition.
// The composition defines which packages are combined to form the platform model.
package model

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// TargetLatest is a fallback to the latest version.
	TargetLatest = "latest"
	// ComposeFile is the name of the compose configuration file.
	ComposeFile = "compose.yaml"
	// ModelDir is the base directory for model operations.
	ModelDir = ".plasma/model"
	// ComposeDir is the base directory for model composition.
	ComposeDir = ModelDir + "/compose"
	// MergedDir is the directory containing the merged composition result.
	MergedDir = ComposeDir + "/merged"
	// MergedSrcDir is the directory containing the merged source components.
	MergedSrcDir = MergedDir + "/src"
	// PackagesDir is the directory containing downloaded packages.
	PackagesDir = ComposeDir + "/packages"
	// PrepareDir is the directory containing prepared deployment artifacts.
	PrepareDir = ModelDir + "/prepare"
)

var (
	// ErrComposeNotExists is returned when compose.yaml doesn't exist.
	ErrComposeNotExists = errors.New("compose.yaml doesn't exist")
)

// Composition stores the model composition definition (packages and their dependencies).
type Composition struct {
	Name         string       `yaml:"name"`
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

// Package stores package definition
type Package struct {
	Name         string   `yaml:"name"`
	Source       Source   `yaml:"source,omitempty"`
	Dependencies []string `yaml:"dependencies,omitempty"`
}

// Dependency stores Dependency definition
type Dependency struct {
	Name   string `yaml:"name"`
	Source Source `yaml:"source,omitempty"`
}

// Strategy stores packages merge strategy name and Paths
type Strategy struct {
	Name  string   `yaml:"name"`
	Paths []string `yaml:"path"`
}

// Source stores package source definition
type Source struct {
	Type       string     `yaml:"type"`
	URL        string     `yaml:"url"`
	Ref        string     `yaml:"ref,omitempty"`
	Strategies []Strategy `yaml:"strategy,omitempty"`
}

// ToPackage converts dependency to package
func (d *Dependency) ToPackage(name string) *Package {
	return &Package{
		Name:   name,
		Source: d.Source,
	}
}

// AddDependency appends new package dependency
func (p *Package) AddDependency(dep string) {
	p.Dependencies = append(p.Dependencies, dep)
}

// GetStrategies from package
func (p *Package) GetStrategies() []Strategy {
	return p.Source.Strategies
}

// GetName from package
func (p *Package) GetName() string {
	return p.Name
}

// GetType from package source
func (p *Package) GetType() string {
	t := p.Source.Type
	if t == "" {
		return "git"
	}

	return strings.ToLower(t)
}

// GetURL from package source
func (p *Package) GetURL() string {
	return p.Source.URL
}

// GetRef from package source
func (p *Package) GetRef() string {
	return p.Source.Ref
}

// GetTarget returns a target version of package
func (p *Package) GetTarget() string {
	target := TargetLatest
	ref := p.GetRef()
	if ref != "" {
		target = ref
	}

	return target
}

// GetIdentifier returns a Go-style package identifier: domain/path/name@ref
// e.g., "projects.skilld.cloud/skilld/pla-plasma@prepare"
func (p *Package) GetIdentifier() string {
	rawURL := p.GetURL()
	ref := p.GetRef()

	// Parse URL to extract domain and path
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Fallback to name@ref if URL parsing fails
		if ref != "" {
			return p.Name + "@" + ref
		}
		return p.Name
	}

	// Build domain/path (strip .git suffix)
	path := strings.TrimSuffix(parsed.Path, ".git")
	path = strings.TrimPrefix(path, "/")
	identifier := parsed.Host + "/" + path

	// Append ref if present
	if ref != "" {
		identifier += "@" + ref
	}

	return identifier
}

// Lookup allows to search compose file, read and parse it.
func Lookup(fsys fs.FS) (*Composition, error) {
	f, err := fs.ReadFile(fsys, ComposeFile)
	if err != nil {
		return &Composition{}, ErrComposeNotExists
	}

	cfg, err := parseComposeYaml(f)
	if err != nil {
		return &Composition{}, fmt.Errorf("compose.yaml parsing failed - %w", err)
	}

	return cfg, nil
}

func parseComposeYaml(input []byte) (*Composition, error) {
	cfg := Composition{}
	err := yaml.Unmarshal(input, &cfg)
	return &cfg, err
}

// QueryPackage finds which package provides a given component.
// Returns package name with ref (e.g., "plasma-core@prepare") or empty string if not found.
func QueryPackage(dir, componentName string) string {
	cfg, err := Lookup(os.DirFS(dir))
	if err != nil {
		return ""
	}

	packagesDir := filepath.Join(dir, PackagesDir)
	componentPath := strings.ReplaceAll(componentName, ".", string(filepath.Separator))

	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = TargetLatest
		}
		pkgBasePath := filepath.Join(packagesDir, dep.Name, ref)

		// Check both package structures:
		// - src/<layer>/<kind>/<name>/ (plasma-core style)
		// - <layer>/<kind>/roles/<name>/ (plasma-work style)
		srcPath := filepath.Join(pkgBasePath, "src", componentPath)
		rolesPath := filepath.Join(pkgBasePath, componentPathWithRoles(componentName))

		if fileExists(srcPath) || fileExists(rolesPath) {
			return fmt.Sprintf("%s@%s", dep.Name, ref)
		}
	}

	return ""
}

// componentPathWithRoles converts a component name to path with roles/ subdirectory
// e.g., "interaction.applications.im" -> "interaction/applications/roles/im"
func componentPathWithRoles(component string) string {
	parts := strings.Split(component, ".")
	if len(parts) < 3 {
		return strings.ReplaceAll(component, ".", string(filepath.Separator))
	}
	// Format: <layer>/<kind>/roles/<name>
	return filepath.Join(parts[0], parts[1], "roles", parts[len(parts)-1])
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
