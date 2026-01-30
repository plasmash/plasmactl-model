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
	// e.g., "interaction.applications.connect" -> "interaction/applications/roles/connect"
	componentPath := componentToPath(q.Component)

	packagesDir := filepath.Join(q.WorkingDir, ".plasma/package/compose/packages")

	var found []string
	for _, dep := range cfg.Dependencies {
		pkgPath := filepath.Join(packagesDir, dep.Name, componentPath)
		if _, err := os.Stat(pkgPath); err == nil {
			ref := dep.Source.Ref
			if ref == "" {
				ref = "latest"
			}
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
// e.g., "interaction.applications.connect" -> "interaction/applications/roles/connect"
// e.g., "platform.entities.person" -> "platform/entities/roles/person"
func componentToPath(component string) string {
	parts := strings.Split(component, ".")
	if len(parts) < 3 {
		// Fallback: just replace dots with slashes
		return strings.ReplaceAll(component, ".", "/")
	}

	// Format: namespace.type.name or namespace.type.subtype.name
	// Components are in roles/ subdirectory
	namespace := parts[0]
	componentType := parts[1]
	name := parts[len(parts)-1]

	// Handle subtype (e.g., "platform.entities.roles.person" vs "platform.entities.person")
	if len(parts) == 4 && parts[2] == "roles" {
		// Already has roles in path
		return filepath.Join(namespace, componentType, "roles", name)
	}

	return filepath.Join(namespace, componentType, "roles", name)
}
