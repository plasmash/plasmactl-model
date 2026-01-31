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

	// Scan for components: <namespace>/<type>/roles/<name>/
	namespaces, err := os.ReadDir(pkgPath)
	if err != nil {
		return nil
	}

	for _, ns := range namespaces {
		if !ns.IsDir() {
			continue
		}
		nsPath := filepath.Join(pkgPath, ns.Name())

		// Scan component types (applications, entities, services, etc.)
		types, err := os.ReadDir(nsPath)
		if err != nil {
			continue
		}

		for _, t := range types {
			if !t.IsDir() {
				continue
			}
			rolesPath := filepath.Join(nsPath, t.Name(), "roles")

			roles, err := os.ReadDir(rolesPath)
			if err != nil {
				continue
			}

			for _, role := range roles {
				if !role.IsDir() {
					continue
				}
				// Component name: namespace.type.name
				compName := fmt.Sprintf("%s.%s.%s", ns.Name(), t.Name(), role.Name())
				components = append(components, compName)
			}
		}
	}

	sort.Strings(components)
	return components
}
