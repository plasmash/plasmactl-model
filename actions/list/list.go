package list

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-chassis/pkg/chassis"
	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-node/pkg/node"

	"github.com/plasmash/plasmactl-model/internal/compose"
	"github.com/plasmash/plasmactl-model/pkg/model"
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

	if len(cfg.Dependencies) == 0 {
		l.Term().Info().Println("No package dependencies")
		return nil
	}

	// Build result
	l.result = &ListResult{}
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

// printTreeWithRelations prints packages as a tree with components (🧩), chassis paths (📍), and nodes (🖥)
func (l *List) printTreeWithRelations(cfg *compose.Composition) error {
	packagesDir := filepath.Join(l.WorkingDir, model.PackagesDir)

	// Check if packages directory exists
	if _, err := os.Stat(packagesDir); os.IsNotExist(err) {
		l.Term().Error().Printfln("packages directory not found: %s (run model:compose first)", packagesDir)
		return nil
	}

	// Load chassis for path validation
	c, _ := chassis.Load(l.WorkingDir)

	// Load components from playbooks
	components, _ := component.LoadFromPlaybooks(l.WorkingDir)
	componentToChassis := make(map[string]string)
	for _, comp := range components {
		componentToChassis[comp.Name] = comp.Chassis
	}

	// Load nodes and compute allocations
	nodesByPlatform, _ := node.LoadByPlatform(l.WorkingDir)
	chassisToNodes := make(map[string][]string)
	if c != nil {
		for _, nodes := range nodesByPlatform {
			allocations := nodes.Allocations(c)
			for _, n := range nodes {
				for _, chassisPath := range allocations[n.Hostname] {
					chassisToNodes[chassisPath] = append(chassisToNodes[chassisPath], n.DisplayName())
				}
			}
		}
	}

	// Sort nodes per chassis path
	for chassisPath := range chassisToNodes {
		sort.Strings(chassisToNodes[chassisPath])
	}

	for pi, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}

		// Print package header
		fmt.Printf("📦 %s@%s\n", dep.Name, ref)

		// Discover components in this package using shared component discovery
		pkgPath := filepath.Join(packagesDir, dep.Name, ref)
		srcPath := filepath.Join(pkgPath, "src")
		if stat, err := os.Stat(srcPath); err == nil && stat.IsDir() {
			pkgPath = srcPath
		}
		pkgComponents, _ := component.LoadFromPath(pkgPath)

		for ci, comp := range pkgComponents {
			isLastComp := ci == len(pkgComponents)-1

			var compPrefix, compIndent string
			if isLastComp {
				compPrefix = "└── "
				compIndent = "    "
			} else {
				compPrefix = "├── "
				compIndent = "│   "
			}

			fmt.Printf("%s🧩 %s\n", compPrefix, comp.DisplayName())

			// Get chassis path for this component
			chassisPath := componentToChassis[comp.Name]
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
				fmt.Printf("%s📍 %s\n", childPrefix, chassisPath)
			}

			// Print nodes that serve this chassis path
			for _, n := range nodes {
				childIdx++
				isLast := childIdx == totalChildren
				var childPrefix string
				if isLast {
					childPrefix = compIndent + "└── "
				} else {
					childPrefix = compIndent + "├── "
				}
				fmt.Printf("%s🖥  %s\n", childPrefix, n)
			}
		}

		if pi < len(cfg.Dependencies)-1 {
			fmt.Println()
		}
	}

	return nil
}

