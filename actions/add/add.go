package add

import (
	"errors"
	"fmt"

	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// Add implements the model:add action
type Add struct {
	action.WithLogger
	action.WithTerm

	WorkingDir   string
	AllowCreate  bool
	Package      string
	Type         string
	Ref          string
	URL          string
	Strategy     []string
	StrategyPath []string
}

// Execute runs the model:add action
func (a *Add) Execute() error {
	// Validate input
	if err := a.validate(); err != nil {
		return err
	}

	// Clear ref for HTTP type
	ref := a.Ref
	if a.Type == compose.HTTPType {
		ref = ""
	}

	dependency := &compose.Dependency{
		Name: a.Package,
		Source: compose.Source{
			Type: a.Type,
			Ref:  ref,
			URL:  a.URL,
		},
	}

	rawStrategies := &compose.RawStrategies{
		Names: a.Strategy,
		Paths: a.StrategyPath,
	}

	fa := &compose.FormsAction{}
	fa.SetLogger(a.Log())
	fa.SetTerm(a.Term())

	return fa.AddPackage(a.AllowCreate, dependency, rawStrategies, a.WorkingDir)
}

// validate validates input options
func (a *Add) validate() error {
	if len(a.Strategy) > 0 || len(a.StrategyPath) > 0 {
		if len(a.Strategy) != len(a.StrategyPath) {
			return errors.New("number of strategies and paths must be equal")
		}

		validStrategies := map[string]bool{
			compose.StrategyOverwriteLocal:     true,
			compose.StrategyRemoveExtraLocal:   true,
			compose.StrategyIgnoreExtraPackage: true,
			compose.StrategyFilterPackage:      true,
		}

		for _, strategy := range a.Strategy {
			if !validStrategies[strategy] {
				return fmt.Errorf("submitted strategy %s doesn't exist", strategy)
			}
		}
	}

	return nil
}
