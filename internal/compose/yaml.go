package compose

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// TargetLatest is a fallback to the latest version.
	TargetLatest           = "latest"
	tplComposeBadStructure = "compose.yaml parsing failed - %w"
)

var (
	composePermissions uint32 = 0644
)

// YamlCompose stores compose definition
type YamlCompose struct {
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
		return GitType
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
func Lookup(fsys fs.FS) (*YamlCompose, error) {
	f, err := fs.ReadFile(fsys, composeFile)
	if err != nil {
		return &YamlCompose{}, errComposeNotExists
	}

	cfg, err := parseComposeYaml(f)
	if err != nil {
		return &YamlCompose{}, fmt.Errorf(tplComposeBadStructure, err)
	}

	return cfg, nil
}

func parseComposeYaml(input []byte) (*YamlCompose, error) {
	cfg := YamlCompose{}
	err := yaml.Unmarshal(input, &cfg)
	return &cfg, err
}

func writeComposeYaml(compose *YamlCompose) error {
	yamlContent, err := yaml.Marshal(compose)
	if err != nil {
		return fmt.Errorf("could not marshal struct into YAML: %v", err)
	}

	err = os.WriteFile(composeFile, yamlContent, os.FileMode(composePermissions))
	if err != nil {
		return err
	}

	return nil
}
