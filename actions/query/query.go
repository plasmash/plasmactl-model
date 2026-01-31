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

	packagesDir := filepath.Join(q.WorkingDir, ".plasma/model/compose/packages")

	var found []string
	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}
		pkgBasePath := filepath.Join(packagesDir, dep.Name, ref)

		// Check both package structures:
		// - src/<layer>/<kind>/<name>/ (plasma-core style)
		// - <layer>/<kind>/roles/<name>/ (plasma-work style)
		srcPath := filepath.Join(pkgBasePath, "src", componentPath)
		rolesPath := filepath.Join(pkgBasePath, componentPathWithRoles(q.Component))

		if _, err := os.Stat(srcPath); err == nil {
			found = append(found, fmt.Sprintf("%s@%s", dep.Name, ref))
		} else if _, err := os.Stat(rolesPath); err == nil {
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
func componentToPath(component string) string {
	// Component format: <layer>.<kind>.<name>
	return strings.ReplaceAll(component, ".", string(filepath.Separator))
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
