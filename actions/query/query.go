package query

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-model/pkg/model"
	"github.com/plasmash/plasmactl-platform/pkg/graph"
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

	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	// Build package name → ref map from config
	pkgRefs := make(map[string]string)
	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}
		pkgRefs[dep.Name] = ref
	}

	var found []string

	// Search based on kind or auto-detect
	switch q.Kind {
	case "component":
		found = q.queryByComponent(g, pkgRefs, q.Identifier)
	case "chassis":
		found = q.queryByChassis(g, pkgRefs, q.Identifier)
	case "node":
		found = q.queryByNode(g, pkgRefs, q.Identifier)
	default:
		// Auto-detect: try component, then chassis, then node
		found = q.queryByComponent(g, pkgRefs, q.Identifier)
		if len(found) == 0 {
			found = q.queryByChassis(g, pkgRefs, q.Identifier)
		}
		if len(found) == 0 {
			found = q.queryByNode(g, pkgRefs, q.Identifier)
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
func (q *Query) queryByComponent(g *graph.PlatformGraph, pkgRefs map[string]string, componentName string) []string {
	var found []string
	for _, e := range g.EdgesTo(componentName, "contains") {
		if e.From().Type == "package" {
			if ref, ok := pkgRefs[e.From().Name]; ok {
				found = append(found, fmt.Sprintf("%s@%s", e.From().Name, ref))
			}
		}
	}
	return found
}

// queryByChassis finds packages with components attached to a chassis path
func (q *Query) queryByChassis(g *graph.PlatformGraph, pkgRefs map[string]string, chassisPath string) []string {
	// Find components attached to this chassis or descendant chassis paths
	var componentNames []string
	for _, n := range g.NodesByType("component") {
		for _, e := range g.EdgesTo(n.Name, "attaches") {
			chassis := e.From().Name
			if chassis == chassisPath || strings.HasPrefix(chassis, chassisPath+".") {
				componentNames = append(componentNames, n.Name)
			}
		}
	}

	// Find packages that provide these components
	var found []string
	for _, compName := range componentNames {
		found = append(found, q.queryByComponent(g, pkgRefs, compName)...)
	}
	return found
}

// queryByNode finds packages with components running on a node
func (q *Query) queryByNode(g *graph.PlatformGraph, pkgRefs map[string]string, hostname string) []string {
	nodeNode := g.Node(hostname)
	if nodeNode == nil || nodeNode.Type != "node" {
		return nil
	}

	// Get chassis paths this node serves
	chassisSet := make(map[string]bool)
	for _, e := range g.EdgesFrom(nodeNode.Name, "memberof") {
		chassisSet[e.To().Name] = true
	}

	// Find components attached to the node's chassis paths
	var componentNames []string
	for _, n := range g.NodesByType("component") {
		for _, e := range g.EdgesTo(n.Name, "attaches") {
			if chassisSet[e.From().Name] {
				componentNames = append(componentNames, n.Name)
			}
		}
	}

	// Find packages that provide these components
	var found []string
	for _, compName := range componentNames {
		found = append(found, q.queryByComponent(g, pkgRefs, compName)...)
	}
	return found
}
