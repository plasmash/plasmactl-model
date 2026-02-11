package remove

import (
	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// RemoveResult is the structured result of model:remove.
type RemoveResult struct {
	Packages []string `json:"packages"`
}

// Remove implements the model:remove action
type Remove struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Packages   []string

	result *RemoveResult
}

// Result returns the structured result for JSON output.
func (r *Remove) Result() any {
	return r.result
}

// Execute runs the model:remove action
func (r *Remove) Execute() error {
	fa := &compose.FormsAction{}
	fa.SetLogger(r.Log())
	fa.SetTerm(r.Term())

	if err := fa.DeletePackages(r.Packages, r.WorkingDir); err != nil {
		return err
	}

	r.result = &RemoveResult{Packages: r.Packages}
	return nil
}
