package compose

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/plasmash/plasmactl-model/pkg/model"
)

var composePermissions uint32 = 0644

// Re-export for internal use
var (
	Lookup       = model.Lookup
	TargetLatest = model.TargetLatest
)

// Type aliases for internal use
type (
	Composition = model.Composition
	Package     = model.Package
	Dependency  = model.Dependency
	Strategy    = model.Strategy
	Source      = model.Source
)

func writeComposeYaml(cfg *Composition) error {
	yamlContent, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(model.ComposeFile, yamlContent, os.FileMode(composePermissions))
}
