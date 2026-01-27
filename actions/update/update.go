package update

import (
	"errors"
	"fmt"

	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// Update implements the model:update action
type Update struct {
	action.WithLogger
	action.WithTerm

	WorkingDir   string
	Package      string
	Type         string
	Ref          string
	URL          string
	Strategy     []string
	StrategyPath []string
}

// Execute runs the model:update action
func (u *Update) Execute() error {
	// Validate input
	if err := u.validate(); err != nil {
		return err
	}

	fa := &compose.FormsAction{}
	fa.SetLogger(u.Log())
	fa.SetTerm(u.Term())

	// If no package specified, run interactive update
	if u.Package == "" {
		return fa.UpdatePackages(u.WorkingDir)
	}

	// Clear ref for HTTP type
	ref := u.Ref
	if u.Type == compose.HTTPType {
		ref = ""
	}

	dependency := &compose.Dependency{
		Name: u.Package,
		Source: compose.Source{
			Type: u.Type,
			Ref:  ref,
			URL:  u.URL,
		},
	}

	rawStrategies := &compose.RawStrategies{
		Names: u.Strategy,
		Paths: u.StrategyPath,
	}

	return fa.UpdatePackage(dependency, rawStrategies, u.WorkingDir)
}

// validate validates input options
func (u *Update) validate() error {
	if len(u.Strategy) > 0 || len(u.StrategyPath) > 0 {
		if len(u.Strategy) != len(u.StrategyPath) {
			return errors.New("number of strategies and paths must be equal")
		}

		validStrategies := map[string]bool{
			compose.StrategyOverwriteLocal:     true,
			compose.StrategyRemoveExtraLocal:   true,
			compose.StrategyIgnoreExtraPackage: true,
			compose.StrategyFilterPackage:      true,
		}

		for _, strategy := range u.Strategy {
			if !validStrategies[strategy] {
				return fmt.Errorf("submitted strategy %s doesn't exist", strategy)
			}
		}
	}

	return nil
}
