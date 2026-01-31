package show

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// Show implements the model:show action
type Show struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Package    string
}

// Execute runs the model:show action
func (s *Show) Execute() error {
	cfg, err := compose.Lookup(os.DirFS(s.WorkingDir))
	if err != nil {
		s.Term().Error().Println("compose.yaml not found")
		return nil
	}

	if len(cfg.Dependencies) == 0 {
		s.Term().Info().Println("No package dependencies")
		return nil
	}

	packagesDir := filepath.Join(s.WorkingDir, ".plasma/package/compose/packages")

	// If specific package requested, find and show it
	if s.Package != "" {
		// Strip @ref if present (e.g., "plasma-core@prepare" -> "plasma-core")
		pkgName := s.Package
		if idx := strings.Index(pkgName, "@"); idx != -1 {
			pkgName = pkgName[:idx]
		}
		for _, dep := range cfg.Dependencies {
			if dep.Name == pkgName {
				printPackage(dep, packagesDir, s.Term())
				return nil
			}
		}
		s.Term().Error().Printfln("Package %q not found", pkgName)
		return nil
	}

	// Show all packages
	for i, dep := range cfg.Dependencies {
		if i > 0 {
			fmt.Println()
		}
		printPackage(dep, packagesDir, s.Term())
	}

	return nil
}

func printPackage(dep compose.Dependency, packagesDir string, term *launchr.Terminal) {
	fmt.Printf("package\t%s\n", dep.Name)
	ref := dep.Source.Ref
	if ref == "" {
		ref = "latest"
	}
	fmt.Printf("ref\t%s\n", ref)
	if dep.Source.URL != "" {
		fmt.Printf("url\t%s\n", dep.Source.URL)
	}
	if dep.Source.Type != "" {
		fmt.Printf("type\t%s\n", dep.Source.Type)
	} else {
		fmt.Printf("type\tgit\n")
	}
	if len(dep.Source.Strategies) > 0 {
		for _, strat := range dep.Source.Strategies {
			fmt.Printf("strategy\t%s\n", strat.Name)
		}
	}

	// Discover and print components
	components := discoverComponents(packagesDir, dep.Name, ref)
	if len(components) > 0 {
		fmt.Println()
		term.Info().Printfln("Components (%d)", len(components))
		for _, comp := range components {
			fmt.Println(comp)
		}
	}
}

// discoverComponents finds all components in a package
func discoverComponents(packagesDir, pkgName, ref string) []string {
	var components []string

	pkgPath := filepath.Join(packagesDir, pkgName, ref)

	// Check if src/ subdirectory exists (pla-plasma structure)
	srcPath := filepath.Join(pkgPath, "src")
	if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
		pkgPath = srcPath
	}

	// Scan for components: <layer>/<kind>/<name>/
	layers, err := os.ReadDir(pkgPath)
	if err != nil {
		return nil
	}

	for _, l := range layers {
		if !l.IsDir() {
			continue
		}
		layerPath := filepath.Join(pkgPath, l.Name())

		// Scan component kinds (applications, entities, services, etc.)
		kinds, err := os.ReadDir(layerPath)
		if err != nil {
			continue
		}

		for _, k := range kinds {
			if !k.IsDir() {
				continue
			}
			kindPath := filepath.Join(layerPath, k.Name())

			// Scan component names directly under kind
			names, err := os.ReadDir(kindPath)
			if err != nil {
				continue
			}

			for _, name := range names {
				if !name.IsDir() {
					continue
				}
				// Component name: layer.kind.name
				compName := fmt.Sprintf("%s.%s.%s", l.Name(), k.Name(), name.Name())
				components = append(components, compName)
			}
		}
	}

	sort.Strings(components)
	return components
}
