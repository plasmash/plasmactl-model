package release

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr/pkg/action"
	irelease "github.com/plasmash/plasmactl-model/internal/release"
)

const imageDir = "img"

// ReleaseResult is the structured result of model:release.
type ReleaseResult struct {
	Tag       string `json:"tag"`
	DryRun    bool   `json:"dry_run"`
	TagOnly   bool   `json:"tag_only"`
	ReleaseID string `json:"release_id,omitempty"`
	Asset     string `json:"asset,omitempty"`
}

// Release implements the model:release command
type Release struct {
	action.WithLogger
	action.WithTerm

	Keyring  keyring.Keyring
	Version  string
	DryRun   bool
	TagOnly  bool
	ForgeURL string
	Token    string

	result *ReleaseResult
}

// Result returns the structured result for JSON output.
func (r *Release) Result() any {
	return r.result
}

// Execute runs the release action
func (r *Release) Execute() error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Initialize git operations
	gitOps := irelease.NewGitOps(workDir)

	// Check branch
	branch, err := gitOps.GetCurrentBranch()
	if err != nil {
		return err
	}

	if branch != "master" && branch != "main" {
		return fmt.Errorf("current branch is %q, must be 'master' or 'main'", branch)
	}

	// Fetch latest tags
	if gitOps.HasRemote() {
		r.Term().Info().Println("Fetching latest tags from remote...")
		if err := gitOps.FetchTags(); err != nil {
			r.Term().Warning().Printfln("Failed to fetch tags: %v", err)
		}
	}

	// Get latest semver tag
	latestVersion, err := gitOps.GetLatestSemverTag()
	if err != nil {
		return err
	}

	var latestTag string
	if latestVersion == nil {
		r.Term().Info().Println("No valid SemVer tags found. Will create initial release.")
		latestTag = ""
	} else {
		latestTag = latestVersion.String()
		r.Term().Info().Printfln("Latest tag: %s", latestTag)
	}

	// Generate changelog
	changelogGen, err := irelease.NewChangelogGenerator(workDir)
	if err != nil {
		return err
	}

	changelog, err := changelogGen.Generate(latestTag)
	if err != nil {
		return fmt.Errorf("failed to generate changelog: %w", err)
	}

	if changelog == "" && latestTag != "" {
		r.Term().Info().Printfln("No changes since %s. Nothing to release.", latestTag)
		return nil
	}

	r.Term().Println()
	r.Term().Println(changelog)
	r.Term().Println()

	// Determine new version
	var newVersion *irelease.Version
	if r.Version == "" {
		// No version specified - bump patch by default
		if latestVersion == nil {
			newVersion = irelease.InitialVersion()
		} else {
			newVersion = latestVersion.Bump(irelease.BumpPatch)
		}
		r.Term().Info().Printfln("Auto-bumping to: %s", newVersion.String())
	} else if irelease.IsBumpType(r.Version) {
		// Bump type specified
		if latestVersion == nil {
			newVersion = irelease.InitialVersion()
		} else {
			newVersion = latestVersion.Bump(irelease.BumpType(r.Version))
		}
	} else {
		// Explicit version specified
		newVersion, err = irelease.ParseVersion(r.Version)
		if err != nil {
			return fmt.Errorf("invalid version %q: %w", r.Version, err)
		}
	}

	newTag := newVersion.String()
	r.Term().Info().Printfln("New version: %s", newTag)

	// Dry run - stop here
	if r.DryRun {
		r.result = &ReleaseResult{Tag: newTag, DryRun: true, TagOnly: r.TagOnly}
		r.Term().Println()
		r.Term().Warning().Println("Dry run - no changes made.")
		r.Term().Info().Printfln("Would create tag: %s", newTag)
		if r.TagOnly {
			r.Term().Info().Println("Would push tag only (no forge release)")
		} else {
			r.Term().Info().Println("Would create forge release and upload .pm")
		}
		return nil
	}

	// Create and push tag
	r.Term().Println()
	r.Term().Info().Printfln("Creating tag: %s", newTag)

	if err := gitOps.CreateTag(newTag, changelog); err != nil {
		return err
	}

	r.Term().Info().Println("Pushing tag to origin...")
	if err := gitOps.PushTag(newTag); err != nil {
		return err
	}

	// Tag only mode - stop here
	if r.TagOnly {
		r.result = &ReleaseResult{Tag: newTag, TagOnly: true}
		r.Term().Println()
		r.Term().Success().Printfln("Tag %s created and pushed.", newTag)
		return nil
	}

	// Get remote info
	remoteInfo, err := gitOps.GetRemoteInfo()
	if err != nil {
		return err
	}

	r.Term().Println()
	r.Term().Info().Printfln("Detecting forge type for %s...", remoteInfo.Host)

	// Create forge client
	forge := irelease.NewForge(remoteInfo.Host, remoteInfo.Repo, r.Token)

	forgeType, err := forge.DetectType()
	if err != nil {
		return err
	}

	r.Term().Info().Printfln("Detected forge: %s", forgeType)

	// Resolve token
	token := irelease.ResolveToken(r.Token, forgeType)
	if token == "" {
		r.Term().Println()
		r.Term().Error().Printfln("No API token available for %s", forgeType)
		r.Term().Println()
		r.Term().Println("Provide a token via one of:")
		r.Term().Println("  --token <token>")
		switch forgeType {
		case irelease.ForgeGitHub:
			r.Term().Println("  GITHUB_TOKEN environment variable")
		case irelease.ForgeGitLab:
			r.Term().Println("  GITLAB_TOKEN environment variable")
		case irelease.ForgeGitea, irelease.ForgeForgejo:
			r.Term().Println("  GITEA_TOKEN environment variable")
		}
		return fmt.Errorf("no API token available")
	}

	// Recreate forge with resolved token
	forge = irelease.NewForge(remoteInfo.Host, remoteInfo.Repo, token)
	forge.DetectType() // Re-detect with token

	// Create release
	r.Term().Println()
	releaseID, err := forge.CreateRelease(newTag, changelog)
	if err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}

	r.Term().Success().Printfln("Release created (ID: %s)", releaseID)

	// Find and upload Platform Model (.pm) file
	image := findImage(imageDir)
	if image == "" {
		r.result = &ReleaseResult{Tag: newTag, ReleaseID: releaseID}
		r.Term().Println()
		r.Term().Warning().Printfln("No Platform Model (.pm) found in %s - skipping artifact upload.", imageDir)
		r.Term().Println()
		r.Term().Success().Printfln("Release %s created successfully.", newTag)
		return nil
	}

	r.Term().Println()
	r.Term().Info().Printfln("Uploading Platform Model: %s", image)

	if err := forge.UploadAsset(releaseID, image); err != nil {
		return fmt.Errorf("failed to upload asset: %w", err)
	}

	r.result = &ReleaseResult{Tag: newTag, ReleaseID: releaseID, Asset: image}

	r.Term().Println()
	r.Term().Success().Printfln("Release %s created successfully with Platform Model!", newTag)

	return nil
}

// findImage finds the latest .pm file in the image directory
func findImage(dir string) string {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".pm" {
			return filepath.Join(dir, entry.Name())
		}
	}

	return ""
}
