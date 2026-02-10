package show

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-model/internal/compose"
	"github.com/plasmash/plasmactl-platform/pkg/graph"
)

// PackageInfo represents a package dependency with its details
type PackageInfo struct {
	Name       string   `json:"name"`
	Ref        string   `json:"ref"`
	URL        string   `json:"url,omitempty"`
	Type       string   `json:"type"`
	Strategies []string `json:"strategies,omitempty"`
	Components []string `json:"components,omitempty"`
}

// ShowResult is the structured output for model:show
type ShowResult struct {
	Packages []PackageInfo `json:"packages"`
}

// Show implements the model:show action
type Show struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Package    string

	// Filter flags
	Packages bool // Show only external packages
	Src      bool // Show only local src/ components
	Merged   bool // Show merged composition result

	result *ShowResult
}

// Result returns the structured result for JSON output
func (s *Show) Result() any {
	return s.result
}

// Execute runs the model:show action
func (s *Show) Execute() error {
	cfg, err := compose.Lookup(os.DirFS(s.WorkingDir))
	if err != nil {
		s.Term().Error().Println("compose.yaml not found")
		return nil
	}

	// Initialize result
	s.result = &ShowResult{}

	// Handle --merged flag: show merged composition from graph
	if s.Merged {
		return s.showMerged()
	}

	// Handle --src flag: show only local src/ components (filesystem-based)
	if s.Src {
		return s.showSrc(filepath.Join(s.WorkingDir, "src"))
	}

	// Handle --packages flag: show only external packages (no components)
	if s.Packages {
		return s.showPackagesOnly(cfg)
	}

	// If specific package requested, find and show it
	if s.Package != "" {
		// Strip @ref if present (e.g., "plasma-core@prepare" -> "plasma-core")
		pkgName := s.Package
		if idx := strings.Index(pkgName, "@"); idx != -1 {
			pkgName = pkgName[:idx]
		}

		g, err := graph.Load()
		if err != nil {
			return fmt.Errorf("failed to load graph: %w", err)
		}

		for _, dep := range cfg.Dependencies {
			if dep.Name == pkgName {
				pkg := s.buildPackageInfo(dep, g)
				s.result.Packages = append(s.result.Packages, pkg)
				// Output is handled by launchr based on result schema
				return nil
			}
		}
		s.Term().Error().Printfln("Package %q not found", pkgName)
		return nil
	}

	// Default: show model overview (packages + src + stats)
	return s.showOverview(cfg)
}

// buildPackageInfo creates a PackageInfo from a compose.Dependency
func (s *Show) buildPackageInfo(dep compose.Dependency, g *graph.PlatformGraph) PackageInfo {
	ref := dep.Source.Ref
	if ref == "" {
		ref = "latest"
	}

	pkgType := dep.Source.Type
	if pkgType == "" {
		pkgType = "git"
	}

	pkg := PackageInfo{
		Name: dep.Name,
		Ref:  ref,
		URL:  dep.Source.URL,
		Type: pkgType,
	}

	// Add strategies
	for _, strat := range dep.Source.Strategies {
		pkg.Strategies = append(pkg.Strategies, strat.Name)
	}

	// Discover components from graph
	for _, e := range g.EdgesFrom(dep.Name, "contains") {
		if e.To().Type == "component" {
			pkg.Components = append(pkg.Components, e.To().Name)
		}
	}
	sort.Strings(pkg.Components)

	return pkg
}

// printPackage outputs human-readable package details
func (s *Show) printPackage(pkg PackageInfo) {
	term := s.Term()
	term.Printfln("package\t%s", pkg.Name)
	term.Printfln("ref\t%s", pkg.Ref)
	if pkg.URL != "" {
		term.Printfln("url\t%s", pkg.URL)
	}
	term.Printfln("type\t%s", pkg.Type)
	for _, strat := range pkg.Strategies {
		term.Printfln("strategy\t%s", strat)
	}

	if len(pkg.Components) > 0 {
		term.Info().Printfln("Components (%d)", len(pkg.Components))
		for _, comp := range pkg.Components {
			term.Printfln("%s", comp)
		}
	}
}

// showMerged displays the merged composition result from the graph
func (s *Show) showMerged() error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	components := g.NodesByType("component")
	if len(components) == 0 {
		s.Term().Info().Println("No components in composition")
		return nil
	}

	names := make([]string, len(components))
	for i, n := range components {
		names[i] = n.Name
	}
	sort.Strings(names)

	term := s.Term()
	term.Info().Printfln("Components (%d)", len(components))
	for _, name := range names {
		term.Printfln("%s", name)
	}

	return nil
}

// showSrc displays only local src/ components (filesystem-based, not in graph)
func (s *Show) showSrc(srcDir string) error {
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		s.Term().Info().Println("No src/ directory found")
		return nil
	}

	components, _ := component.LoadFromPath(srcDir)
	if len(components) == 0 {
		s.Term().Info().Println("No components in src/")
		return nil
	}

	term := s.Term()
	term.Info().Printfln("Source Components (%d)", len(components))
	term.Printfln("Location: %s\n", srcDir)

	for _, comp := range components {
		term.Printfln("%s", comp.Name)
	}

	return nil
}

// showPackagesOnly displays packages without component details
func (s *Show) showPackagesOnly(cfg *compose.Composition) error {
	if len(cfg.Dependencies) == 0 {
		s.Term().Info().Println("No package dependencies")
		return nil
	}

	term := s.Term()
	term.Info().Printfln("Packages (%d)", len(cfg.Dependencies))
	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}
		term.Printfln("%s@%s", dep.Name, ref)
	}

	return nil
}

// showOverview displays the model overview with packages, src, and stats
func (s *Show) showOverview(cfg *compose.Composition) error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	// Show packages summary with component counts from graph
	term := s.Term()
	if len(cfg.Dependencies) > 0 {
		term.Info().Printfln("Packages (%d)", len(cfg.Dependencies))
		for _, dep := range cfg.Dependencies {
			ref := dep.Source.Ref
			if ref == "" {
				ref = "latest"
			}

			var count int
			for _, e := range g.EdgesFrom(dep.Name, "contains") {
				if e.To().Type == "component" {
					count++
				}
			}

			term.Printfln("  %s@%s\t(%d components)", dep.Name, ref, count)
		}
	}

	// Show src/ summary (filesystem-based, local uncomposed code)
	srcDir := filepath.Join(s.WorkingDir, "src")
	if _, err := os.Stat(srcDir); err == nil {
		srcComponents, _ := component.LoadFromPath(srcDir)
		if len(srcComponents) > 0 {
			term.Info().Printfln("Source (%d)", len(srcComponents))
			term.Printfln("  Location: %s", srcDir)
		}
	}

	// Merged stats from graph
	allComponents := g.NodesByType("component")
	if len(allComponents) > 0 {
		term.Info().Printfln("Merged: %d components total", len(allComponents))
	}

	return nil
}
