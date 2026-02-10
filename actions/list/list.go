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
		l.Term().Error().Println("compose.yaml not found")
		return nil
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

	// Structured result is auto-formatted by launchr
	return nil
}

// printTreeWithRelations prints packages as a tree with components, chassis paths, and nodes
func (l *List) printTreeWithRelations(cfg *compose.Composition) error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	term := l.Term()

	// Build component→chassis map from graph
	componentToChassis := make(map[string]string)
	for _, n := range g.NodesByType("component") {
		for _, e := range g.EdgesTo(n.Name, "distributes") {
			componentToChassis[n.Name] = e.From().Name
		}
	}

	// Build chassis→nodes map from graph
	chassisToNodes := make(map[string][]string)
	for _, n := range g.NodesByType("node") {
		for _, e := range g.EdgesFrom(n.Name, "allocates") {
			chassisToNodes[e.To().Name] = append(chassisToNodes[e.To().Name], n.Name)
		}
	}
	for k := range chassisToNodes {
		sort.Strings(chassisToNodes[k])
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

			// Get chassis path for this component
			chassisPath := componentToChassis[compName]
			nodes := chassisToNodes[chassisPath]
			totalChildren := 0
			if chassisPath != "" {
				totalChildren++
			}
			totalChildren += len(nodes)

			childIdx := 0

			// Print chassis path
			if chassisPath != "" {
				childIdx++
				isLast := childIdx == totalChildren
				var childPrefix string
				if isLast {
					childPrefix = compIndent + "└── "
				} else {
					childPrefix = compIndent + "├── "
				}
				term.Printfln("%s📍 %s", childPrefix, chassisPath)
			}

			// Print nodes that serve this chassis path
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
