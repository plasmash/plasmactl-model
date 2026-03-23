// Package test contains testscript-based integration tests for the compose plugin.
package test

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/launchrctl/launchr"
	launchrtest "github.com/launchrctl/launchr/test"
	_ "github.com/plasmash/plasmactl-model" // registers the plugin via init()
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"launchr": launchr.RunAndExit,
	})
}

func TestCompose(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/compose",
		Cmds:                launchrtest.CmdsTestScript(),
		RequireExplicitExec: true,
	})
}
