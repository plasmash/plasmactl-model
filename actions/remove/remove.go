package remove

import (
	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// Remove implements the model:remove action
type Remove struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Packages   []string
}

// Execute runs the model:remove action
func (r *Remove) Execute() error {
	fa := &compose.FormsAction{}
	fa.SetLogger(r.Log())
	fa.SetTerm(r.Term())

	return fa.DeletePackages(r.Packages, r.WorkingDir)
}
