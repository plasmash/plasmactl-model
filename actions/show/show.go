package show

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-model/internal/compose"
	"github.com/plasmash/plasmactl-model/pkg/model"
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

	packagesDir := filepath.Join(s.WorkingDir, model.PackagesDir)
	mergedDir := filepath.Join(s.WorkingDir, model.MergedSrcDir)
	srcDir := filepath.Join(s.WorkingDir, "src")

	// Initialize result
	s.result = &ShowResult{}

	// Handle --merged flag: show merged composition
	if s.Merged {
		return s.showMerged(mergedDir)
	}

	// Handle --src flag: show only local src/ components
	if s.Src {
		return s.showSrc(srcDir)
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
		for _, dep := range cfg.Dependencies {
			if dep.Name == pkgName {
				pkg := s.buildPackageInfo(dep, packagesDir)
				s.result.Packages = append(s.result.Packages, pkg)
				// Output is handled by launchr based on result schema
				return nil
			}
		}
		s.Term().Error().Printfln("Package %q not found", pkgName)
		return nil
	}

	// Default: show model overview (packages + src + stats)
	return s.showOverview(cfg, packagesDir, srcDir, mergedDir)
}

// buildPackageInfo creates a PackageInfo from a compose.Dependency
func (s *Show) buildPackageInfo(dep compose.Dependency, packagesDir string) PackageInfo {
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

	// Discover components using shared logic from plasmactl-component
	pkgPath := filepath.Join(packagesDir, dep.Name, ref)

	// Check if src/ subdirectory exists (plasma-core style)
	srcPath := filepath.Join(pkgPath, "src")
	if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
		pkgPath = srcPath
	}

	components, _ := component.LoadFromPath(pkgPath)
	for _, comp := range components {
		pkg.Components = append(pkg.Components, comp.Name)
	}

	return pkg
}

// printPackage outputs human-readable package details
func (s *Show) printPackage(pkg PackageInfo) {
	fmt.Printf("package\t%s\n", pkg.Name)
	fmt.Printf("ref\t%s\n", pkg.Ref)
	if pkg.URL != "" {
		fmt.Printf("url\t%s\n", pkg.URL)
	}
	fmt.Printf("type\t%s\n", pkg.Type)
	for _, strat := range pkg.Strategies {
		fmt.Printf("strategy\t%s\n", strat)
	}

	if len(pkg.Components) > 0 {
		s.Term().Info().Printfln("Components (%d)", len(pkg.Components))
		for _, comp := range pkg.Components {
			fmt.Println(comp)
		}
	}
}

// showMerged displays the merged composition result
func (s *Show) showMerged(mergedDir string) error {
	if _, err := os.Stat(mergedDir); os.IsNotExist(err) {
		s.Term().Warning().Println("Merged directory not found. Run model:compose first.")
		return nil
	}

	components, _ := component.LoadFromPath(mergedDir)
	if len(components) == 0 {
		s.Term().Info().Println("No components in merged composition")
		return nil
	}

	s.Term().Info().Printfln("Merged Components (%d)", len(components))
	fmt.Printf("Location: %s\n", mergedDir)

	for _, comp := range components {
		fmt.Println(comp.Name)
	}

	return nil
}

// showSrc displays only local src/ components
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

	s.Term().Info().Printfln("Source Components (%d)", len(components))
	fmt.Printf("Location: %s\n\n", srcDir)

	for _, comp := range components {
		fmt.Println(comp.Name)
	}

	return nil
}

// showPackagesOnly displays packages without component details
func (s *Show) showPackagesOnly(cfg *compose.Composition) error {
	if len(cfg.Dependencies) == 0 {
		s.Term().Info().Println("No package dependencies")
		return nil
	}

	s.Term().Info().Printfln("Packages (%d)", len(cfg.Dependencies))
	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}
		fmt.Printf("%s@%s\n", dep.Name, ref)
	}

	return nil
}

// showOverview displays the model overview with packages, src, and stats
func (s *Show) showOverview(cfg *compose.Composition, packagesDir, srcDir, mergedDir string) error {
	// Count totals
	var totalPkgComponents int
	var srcComponents []component.Component

	// Show packages summary
	if len(cfg.Dependencies) > 0 {
		s.Term().Info().Printfln("Packages (%d)", len(cfg.Dependencies))
		for _, dep := range cfg.Dependencies {
			ref := dep.Source.Ref
			if ref == "" {
				ref = "latest"
			}

			// Count components in this package
			pkgPath := filepath.Join(packagesDir, dep.Name, ref)
			srcPath := filepath.Join(pkgPath, "src")
			if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
				pkgPath = srcPath
			}
			components, _ := component.LoadFromPath(pkgPath)
			componentCount := len(components)
			totalPkgComponents += componentCount

			fmt.Printf("  %s@%s\t(%d components)\n", dep.Name, ref, componentCount)
		}
	}

	// Show src/ summary
	if _, err := os.Stat(srcDir); err == nil {
		srcComponents, _ = component.LoadFromPath(srcDir)
		if len(srcComponents) > 0 {
			s.Term().Info().Printfln("Source (%d)", len(srcComponents))
			fmt.Printf("  Location: %s\n", srcDir)
		}
	}

	// Show merged stats
	if _, err := os.Stat(mergedDir); err == nil {
		mergedComponents, _ := component.LoadFromPath(mergedDir)
		if len(mergedComponents) > 0 {
			s.Term().Info().Printfln("Merged: %d components total", len(mergedComponents))
			fmt.Printf("  Location: %s\n", mergedDir)
		}
	} else {
		// Estimate total
		total := totalPkgComponents + len(srcComponents)
		if total > 0 {
			s.Term().Info().Printfln("Total: ~%d components (run model:compose to merge)", total)
		}
	}

	return nil
}
