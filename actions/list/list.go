package list

import (
	"fmt"
	"os"

	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// List implements the model:list action
type List struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
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

	for _, dep := range cfg.Dependencies {
		ref := dep.Source.Ref
		if ref == "" {
			ref = "latest"
		}
		fmt.Printf("%s@%s\n", dep.Name, ref)
	}

	return nil
}
