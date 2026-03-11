package list

import (
	"fmt"
	"os"
	"sort"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-model/internal/compose"
	"github.com/plasmash/plasmactl-platform/pkg/graph"
)

// PackageListItem represents a package in the list output
type PackageListItem struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
}

// ListResult is the structured output for model:list
type ListResult struct {
	Packages []PackageListItem `json:"packages"`
}

// List implements the model:list action
type List struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Tree       bool

	result *ListResult
}

// Result returns the structured result for JSON output
func (l *List) Result() any {
	return l.result
}

// Execute runs the model:list action
func (l *List) Execute() error {
	cfg, err := compose.Lookup(os.DirFS(l.WorkingDir))
	if err != nil {
		return fmt.Errorf("compose.yaml not found: %w", err)
	}

	// Build result
	l.result = &ListResult{}

	if len(cfg.Dependencies) == 0 {
		l.Term().Info().Println("No package dependencies")
		return nil
	}
	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}
		l.result.Packages = append(l.result.Packages, PackageListItem{
			Name: dep.Name,
			Ref:  ref,
		})
	}

	// Tree output is special - still needs custom printing
	if l.Tree {
		return l.printTreeWithRelations(cfg)
	}

	term := l.Term()
	for _, pkg := range l.result.Packages {
		term.Printfln("%s@%s", pkg.Name, pkg.Ref)
	}
	return nil
}

// printTreeWithRelations prints packages as a tree with components, zones, and nodes
func (l *List) printTreeWithRelations(cfg *compose.Composition) error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	term := l.Term()

	// Build component→zone map from graph
	componentToZone := make(map[string]string)
	for _, n := range g.NodesByType("component") {
		for _, e := range g.EdgesTo(n.Name, "distributes") {
			componentToZone[n.Name] = e.From().Name
		}
	}

	// Build zone→nodes map from graph
	zoneToNodes := make(map[string][]string)
	for _, n := range g.NodesByType("node") {
		for _, e := range g.EdgesFrom(n.Name, "allocates") {
			zoneToNodes[e.To().Name] = append(zoneToNodes[e.To().Name], n.Name)
		}
	}
	for k := range zoneToNodes {
		sort.Strings(zoneToNodes[k])
	}

	for pi, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}

		// Print package header
		term.Printfln("📦 %s@%s", dep.Name, ref)

		// Get components in this package from graph
		var pkgComponents []string
		for _, e := range g.EdgesFrom(dep.Name, "contains") {
			if e.To().Type == "component" {
				pkgComponents = append(pkgComponents, e.To().Name)
			}
		}
		sort.Strings(pkgComponents)

		for ci, compName := range pkgComponents {
			isLastComp := ci == len(pkgComponents)-1

			var compPrefix, compIndent string
			if isLastComp {
				compPrefix = "└── "
				compIndent = "    "
			} else {
				compPrefix = "├── "
				compIndent = "│   "
			}

			n := g.Node(compName)
			version := ""
			if n != nil {
				version = n.Version
			}
			term.Printfln("%s🧩 %s", compPrefix, component.FormatDisplayName(compName, version))

			// Get zone for this component
			zonePath := componentToZone[compName]
			nodes := zoneToNodes[zonePath]
			totalChildren := 0
			if zonePath != "" {
				totalChildren++
			}
			totalChildren += len(nodes)

			childIdx := 0

			// Print zone
			if zonePath != "" {
				childIdx++
				isLast := childIdx == totalChildren
				var childPrefix string
				if isLast {
					childPrefix = compIndent + "└── "
				} else {
					childPrefix = compIndent + "├── "
				}
				term.Printfln("%s📍 %s", childPrefix, zonePath)
			}

			// Print nodes that serve this zone
			for _, nd := range nodes {
				childIdx++
				isLast := childIdx == totalChildren
				var childPrefix string
				if isLast {
					childPrefix = compIndent + "└── "
				} else {
					childPrefix = compIndent + "├── "
				}
				term.Printfln("%s🖥  %s", childPrefix, nd)
			}
		}

		if pi < len(cfg.Dependencies)-1 {
			term.Printfln("")
		}
	}

	return nil
}
