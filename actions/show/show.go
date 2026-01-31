package show

import (
	"fmt"
	"os"
	"strings"

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

	// If specific package requested, find and show it
	if s.Package != "" {
		// Strip @ref if present (e.g., "plasma-core@prepare" -> "plasma-core")
		pkgName := s.Package
		if idx := strings.Index(pkgName, "@"); idx != -1 {
			pkgName = pkgName[:idx]
		}
		for _, dep := range cfg.Dependencies {
			if dep.Name == pkgName {
				printPackage(dep)
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
		printPackage(dep)
	}

	return nil
}

func printPackage(dep compose.Dependency) {
	fmt.Printf("package\t%s\n", dep.Name)
	if dep.Source.Ref != "" {
		fmt.Printf("ref\t%s\n", dep.Source.Ref)
	} else {
		fmt.Printf("ref\tlatest\n")
	}
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
}
