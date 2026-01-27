// Package plasmactlmodel implements a launchr plugin for plasma model composition
package plasmactlmodel

import (
	"context"
	"embed"
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"

	"github.com/plasmash/plasmactl-model/actions/add"
	"github.com/plasmash/plasmactl-model/actions/bundle"
	"github.com/plasmash/plasmactl-model/actions/compose"
	deleteaction "github.com/plasmash/plasmactl-model/actions/delete"
	"github.com/plasmash/plasmactl-model/actions/prepare"
	"github.com/plasmash/plasmactl-model/actions/release"
	"github.com/plasmash/plasmactl-model/actions/update"
	icompose "github.com/plasmash/plasmactl-model/internal/compose"
)

//go:embed actions/*/*.yaml
var actionYamlFS embed.FS

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is [launchr.Plugin] plugin providing model composition.
type Plugin struct {
	wd string
	k  keyring.Keyring
	m  action.Manager
}

// PluginInfo implements [launchr.Plugin] interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{Weight: 10}
}

// OnAppInit implements [launchr.OnAppInitPlugin] interface.
func (p *Plugin) OnAppInit(app launchr.App) error {
	app.GetService(&p.k)
	app.GetService(&p.m)
	p.wd = app.GetWD()

	// Register composed packages directory as a discovery root if it exists.
	// This is needed because launchr skips hidden directories (starting with .)
	// during discovery, so .plasma/ would be skipped otherwise.
	// This replaces the old launchr-compose plugin's registration of .compose/build.
	composePath := filepath.Join(p.wd, icompose.BuildDir)
	if stat, err := os.Stat(composePath); err == nil && stat.IsDir() {
		app.RegisterFS(action.NewDiscoveryFS(os.DirFS(composePath), p.wd))
	}

	return nil
}

// DiscoverActions implements [launchr.ActionDiscoveryPlugin] interface.
func (p *Plugin) DiscoverActions(_ context.Context) ([]*action.Action, error) {
	// Action model:compose.
	composeYaml, _ := actionYamlFS.ReadFile("actions/compose/compose.yaml")
	composeAction := action.NewFromYAML("model:compose", composeYaml)
	composeAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		log, term := getLogger(a)
		c := &compose.Compose{
			Keyring:            p.k,
			BaseDir:            p.wd,
			WorkingDir:         input.Opt("working-dir").(string),
			Clean:              input.Opt("clean").(bool),
			SkipNotVersioned:   input.Opt("skip-not-versioned").(bool),
			ConflictsVerbosity: input.Opt("conflicts-verbosity").(bool),
			Interactive:        input.Opt("interactive").(bool),
		}
		c.SetLogger(log)
		c.SetTerm(term)
		return c.Execute()
	}))

	// Action model:add.
	addYaml, _ := actionYamlFS.ReadFile("actions/add/add.yaml")
	addAction := action.NewFromYAML("model:add", addYaml)
	addAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		log, term := getLogger(a)
		add := &add.Add{
			WorkingDir:   p.wd,
			AllowCreate:  input.Opt("allow-create").(bool),
			Package:      input.Opt("package").(string),
			Type:         input.Opt("type").(string),
			Ref:          input.Opt("ref").(string),
			URL:          input.Opt("url").(string),
			Strategy:     action.InputOptSlice[string](input, "strategy"),
			StrategyPath: action.InputOptSlice[string](input, "strategy-path"),
		}
		add.SetLogger(log)
		add.SetTerm(term)
		return add.Execute()
	}))

	// Action model:update.
	updateYaml, _ := actionYamlFS.ReadFile("actions/update/update.yaml")
	updateAction := action.NewFromYAML("model:update", updateYaml)
	updateAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		log, term := getLogger(a)
		u := &update.Update{
			WorkingDir:   p.wd,
			Package:      input.Opt("package").(string),
			Type:         input.Opt("type").(string),
			Ref:          input.Opt("ref").(string),
			URL:          input.Opt("url").(string),
			Strategy:     action.InputOptSlice[string](input, "strategy"),
			StrategyPath: action.InputOptSlice[string](input, "strategy-path"),
		}
		u.SetLogger(log)
		u.SetTerm(term)
		return u.Execute()
	}))

	// Action model:delete.
	deleteYaml, _ := actionYamlFS.ReadFile("actions/delete/delete.yaml")
	deleteAction := action.NewFromYAML("model:delete", deleteYaml)
	deleteAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		log, term := getLogger(a)
		d := &deleteaction.Delete{
			WorkingDir: p.wd,
			Packages:   action.InputOptSlice[string](input, "packages"),
		}
		d.SetLogger(log)
		d.SetTerm(term)
		return d.Execute()
	}))

	// Action model:prepare - transforms composed model for Ansible deployment.
	prepareYaml, _ := actionYamlFS.ReadFile("actions/prepare/prepare.yaml")
	prepareActionDef := action.NewFromYAML("model:prepare", prepareYaml)
	prepareActionDef.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		log, term := getLogger(a)
		p := &prepare.Prepare{
			ComposeDir: input.Opt("compose-dir").(string),
			PrepareDir: input.Opt("prepare-dir").(string),
			Clean:      input.Opt("clean").(bool),
		}
		p.SetLogger(log)
		p.SetTerm(term)
		return p.Execute()
	}))

	// Action model:bundle - creates Platform Model (.pm) bundle.
	bundleYaml, _ := actionYamlFS.ReadFile("actions/bundle/bundle.yaml")
	bundleAction := action.NewFromYAML("model:bundle", bundleYaml)
	bundleAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, term := getLogger(a)
		b := &bundle.Bundle{
			HasPrepareAction: true,
		}
		b.SetLogger(log)
		b.SetTerm(term)
		return b.Execute()
	}))

	// Action model:release - creates git tags with changelog and uploads artifact to forge.
	releaseYaml, _ := actionYamlFS.ReadFile("actions/release/release.yaml")
	releaseAction := action.NewFromYAML("model:release", releaseYaml)
	releaseAction.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		log, term := getLogger(a)
		r := &release.Release{
			Keyring:  p.k,
			Version:  input.Arg("version").(string),
			DryRun:   input.Opt("dry-run").(bool),
			TagOnly:  input.Opt("tag-only").(bool),
			ForgeURL: input.Opt("forge-url").(string),
			Token:    input.Opt("token").(string),
		}
		r.SetLogger(log)
		r.SetTerm(term)
		return r.Execute()
	}))

	return []*action.Action{
		composeAction,
		addAction,
		updateAction,
		deleteAction,
		prepareActionDef,
		bundleAction,
		releaseAction,
	}, nil
}

func getLogger(a *action.Action) (*launchr.Logger, *launchr.Terminal) {
	log := launchr.Log()
	if rt, ok := a.Runtime().(action.RuntimeLoggerAware); ok {
		log = rt.LogWith()
	}

	term := launchr.Term()
	if rt, ok := a.Runtime().(action.RuntimeTermAware); ok {
		term = rt.Term()
	}

	return log, term
}
