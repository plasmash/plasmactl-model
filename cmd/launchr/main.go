// Package executes Launchr application.
package main

import (
	"github.com/launchrctl/launchr"

	_ "github.com/plasmash/plasmactl-package"
)

func main() {
	launchr.RunAndExit()
}
