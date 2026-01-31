package query

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// Query implements the model:query action
type Query struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Component  string
}

// Execute runs the model:query action
func (q *Query) Execute() error {
	cfg, err := compose.Lookup(os.DirFS(q.WorkingDir))
	if err != nil {
		q.Term().Error().Println("compose.yaml not found")
		return nil
	}

	// Convert component name to path pattern
	// e.g., "interaction.applications.connect" -> "interaction/applications/connect"
	componentPath := componentToPath(q.Component)

	packagesDir := filepath.Join(q.WorkingDir, ".plasma/package/compose/packages")

	var found []string
	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}
		// Check both direct path and src/ subdirectory
		pkgBasePath := filepath.Join(packagesDir, dep.Name, ref)
		pkgPath := filepath.Join(pkgBasePath, componentPath)
		srcPath := filepath.Join(pkgBasePath, "src", componentPath)

		if _, err := os.Stat(pkgPath); err == nil {
			found = append(found, fmt.Sprintf("%s@%s", dep.Name, ref))
		} else if _, err := os.Stat(srcPath); err == nil {
			found = append(found, fmt.Sprintf("%s@%s", dep.Name, ref))
		}
	}

	if len(found) == 0 {
		q.Term().Error().Printfln("Component %q not found in any package", q.Component)
		return nil
	}

	for _, pkg := range found {
		fmt.Println(pkg)
	}

	return nil
}

// componentToPath converts a component name to its directory path
// e.g., "interaction.applications.connect" -> "interaction/applications/connect"
// e.g., "platform.entities.person" -> "platform/entities/person"
func componentToPath(component string) string {
	// Component format: <layer>.<kind>.<name>
	// Simply replace dots with path separators
	return strings.ReplaceAll(component, ".", string(filepath.Separator))
}
