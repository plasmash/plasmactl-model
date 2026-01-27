package delete

import (
	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/internal/compose"
)

// Delete implements the model:delete action
type Delete struct {
	action.WithLogger
	action.WithTerm

	WorkingDir string
	Packages   []string
}

// Execute runs the model:delete action
func (d *Delete) Execute() error {
	fa := &compose.FormsAction{}
	fa.SetLogger(d.Log())
	fa.SetTerm(d.Term())

	return fa.DeletePackages(d.Packages, d.WorkingDir)
}
