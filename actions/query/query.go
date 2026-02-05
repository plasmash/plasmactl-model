package query

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-chassis/pkg/chassis"
	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-node/pkg/node"

	"github.com/plasmash/plasmactl-model/pkg/model"
)

// QueryResult is the structured output for model:query
type QueryResult struct {
	Packages []string `json:"packages"`
}

// Query implements the model:query action
type Query struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Identifier string
	Kind       string // "component", "chassis", or "node" to skip auto-detection

	result QueryResult
}

// Execute runs the model:query action
func (q *Query) Execute() error {
	cfg, err := model.Lookup(os.DirFS(q.WorkingDir))
	if err != nil {
		q.Term().Error().Println("model.yaml not found")
		return nil
	}

	packagesDir := filepath.Join(q.WorkingDir, model.PackagesDir)

	// Check if packages directory exists
	if _, err := os.Stat(packagesDir); os.IsNotExist(err) {
		q.Term().Error().Printfln("packages directory not found: %s (run model:compose first)", packagesDir)
		return nil
	}

	var found []string

	// Search based on kind or auto-detect
	switch q.Kind {
	case "component":
		found = q.queryByComponent(cfg, packagesDir, q.Identifier)
	case "chassis":
		found = q.queryByChassis(cfg, packagesDir, q.Identifier)
	case "node":
		found = q.queryByNode(cfg, packagesDir, q.Identifier)
	default:
		// Auto-detect: try component, then chassis, then node
		found = q.queryByComponent(cfg, packagesDir, q.Identifier)
		if len(found) == 0 {
			found = q.queryByChassis(cfg, packagesDir, q.Identifier)
		}
		if len(found) == 0 {
			found = q.queryByNode(cfg, packagesDir, q.Identifier)
		}
	}

	if len(found) == 0 {
		q.Term().Warning().Printfln("No packages found for %q", q.Identifier)
		return nil
	}

	// Remove duplicates and sort
	seen := make(map[string]bool)
	var unique []string
	for _, pkg := range found {
		if !seen[pkg] {
			seen[pkg] = true
			unique = append(unique, pkg)
		}
	}
	sort.Strings(unique)

	q.result.Packages = unique

	for _, pkg := range unique {
		fmt.Println(pkg)
	}

	return nil
}

// Result returns the structured result for JSON output
func (q *Query) Result() any {
	return q.result
}

// queryByComponent finds packages that provide a specific component
func (q *Query) queryByComponent(cfg *model.Composition, packagesDir, componentName string) []string {
	var found []string

	componentPath := componentToPath(componentName)

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
		rolesPath := filepath.Join(pkgBasePath, componentPathWithRoles(componentName))

		if _, err := os.Stat(srcPath); err == nil {
			found = append(found, fmt.Sprintf("%s@%s", dep.Name, ref))
		} else if _, err := os.Stat(rolesPath); err == nil {
			found = append(found, fmt.Sprintf("%s@%s", dep.Name, ref))
		}
	}

	return found
}

// queryByChassis finds packages with components attached to a chassis path
func (q *Query) queryByChassis(cfg *model.Composition, packagesDir, chassisPath string) []string {
	// Load chassis to validate path
	c, err := chassis.Load(q.WorkingDir)
	if err != nil || !c.Exists(chassisPath) {
		return nil
	}

	// Load components to find attachments
	components, err := component.LoadFromPlaybooks(q.WorkingDir)
	if err != nil {
		return nil
	}

	// Find components attached to this chassis path or descendants
	var componentNames []string
	for _, comp := range components {
		if comp.Chassis == chassisPath || chassis.IsDescendantOf(comp.Chassis, chassisPath) {
			componentNames = append(componentNames, comp.Name)
		}
	}

	// Find packages that provide these components
	var found []string
	for _, compName := range componentNames {
		pkgs := q.queryByComponent(cfg, packagesDir, compName)
		found = append(found, pkgs...)
	}

	return found
}

// queryByNode finds packages with components running on a node
func (q *Query) queryByNode(cfg *model.Composition, packagesDir, hostname string) []string {
	// Load chassis for distribution
	c, err := chassis.Load(q.WorkingDir)
	if err != nil {
		return nil
	}

	// Load nodes
	nodesByPlatform, err := node.LoadByPlatform(q.WorkingDir)
	if err != nil {
		return nil
	}

	// Find the node and its chassis paths
	var nodeChassis []string
	for _, nodes := range nodesByPlatform {
		n := nodes.Find(hostname)
		if n != nil {
			allocations := nodes.Allocations(c)
			nodeChassis = allocations[hostname]
			break
		}
	}

	if len(nodeChassis) == 0 {
		return nil
	}

	// Load components
	components, err := component.LoadFromPlaybooks(q.WorkingDir)
	if err != nil {
		return nil
	}

	// Find components attached to the node's chassis paths
	chassisSet := make(map[string]bool)
	for _, chassisPath := range nodeChassis {
		chassisSet[chassisPath] = true
	}

	var componentNames []string
	for _, comp := range components {
		if chassisSet[comp.Chassis] {
			componentNames = append(componentNames, comp.Name)
		}
	}

	// Find packages that provide these components
	var found []string
	for _, compName := range componentNames {
		pkgs := q.queryByComponent(cfg, packagesDir, compName)
		found = append(found, pkgs...)
	}

	return found
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
