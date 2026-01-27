package compose

import (
	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr/pkg/action"

	icompose "github.com/plasmash/plasmactl-model/internal/compose"
)

// Compose implements the model:compose action
type Compose struct {
	action.WithLogger
	action.WithTerm

	Keyring            keyring.Keyring
	WorkingDir         string
	BaseDir            string
	Clean              bool
	SkipNotVersioned   bool
	ConflictsVerbosity bool
	Interactive        bool
}

// Execute runs the model:compose action
func (c *Compose) Execute() error {
	composer, err := icompose.CreateComposer(
		c.BaseDir,
		icompose.ComposerOptions{
			Clean:              c.Clean,
			WorkingDir:         c.WorkingDir,
			SkipNotVersioned:   c.SkipNotVersioned,
			ConflictsVerbosity: c.ConflictsVerbosity,
			Interactive:        c.Interactive,
		},
		c.Keyring,
	)
	if err != nil {
		return err
	}

	composer.SetLogger(c.Log())
	composer.SetTerm(c.Term())

	return composer.RunInstall()
}
