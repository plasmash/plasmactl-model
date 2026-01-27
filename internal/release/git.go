package release

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// GitOps provides git operations for releases
type GitOps struct {
	workDir string
}

// NewGitOps creates a new GitOps instance
func NewGitOps(workDir string) *GitOps {
	return &GitOps{workDir: workDir}
}

// GetCurrentBranch returns the current git branch name
func (g *GitOps) GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = g.workDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// FetchTags fetches tags from remote origin
func (g *GitOps) FetchTags() error {
	cmd := exec.Command("git", "fetch", "--tags", "origin")
	cmd.Dir = g.workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch tags: %w", err)
	}
	return nil
}

// GetTags returns all local tags
func (g *GitOps) GetTags() ([]string, error) {
	cmd := exec.Command("git", "tag", "-l")
	cmd.Dir = g.workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get tags: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var tags []string
	for _, line := range lines {
		if line != "" {
			tags = append(tags, line)
		}
	}
	return tags, nil
}

// GetLatestSemverTag returns the highest semver tag
func (g *GitOps) GetLatestSemverTag() (*Version, error) {
	tags, err := g.GetTags()
	if err != nil {
		return nil, err
	}

	var highest *Version
	for _, tag := range tags {
		v, err := ParseVersion(tag)
		if err != nil {
			continue // skip non-semver tags
		}
		if highest == nil || v.Compare(highest) > 0 {
			highest = v
		}
	}

	return highest, nil
}

// CreateTag creates an annotated tag with the given message
func (g *GitOps) CreateTag(tag, message string) error {
	cmd := exec.Command("git", "tag", "-f", "-a", tag, "-m", message)
	cmd.Dir = g.workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tag %s: %w", tag, err)
	}
	return nil
}

// PushTag pushes a tag to origin
func (g *GitOps) PushTag(tag string) error {
	cmd := exec.Command("git", "push", "origin", "tag", tag)
	cmd.Dir = g.workDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push tag %s: %w", tag, err)
	}
	return nil
}

// RemoteInfo contains information about the git remote
type RemoteInfo struct {
	Host string
	Repo string
}

var (
	sshRemoteRegex   = regexp.MustCompile(`^git@([^:]+):(.+?)(?:\.git)?$`)
	httpsRemoteRegex = regexp.MustCompile(`^https?://([^/]+)/(.+?)(?:\.git)?$`)
)

// GetRemoteInfo extracts host and repo from the origin remote URL
func (g *GitOps) GetRemoteInfo() (*RemoteInfo, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = g.workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote URL: %w", err)
	}

	url := strings.TrimSpace(string(output))

	// Try SSH format: git@host:owner/repo.git
	if matches := sshRemoteRegex.FindStringSubmatch(url); matches != nil {
		return &RemoteInfo{Host: matches[1], Repo: matches[2]}, nil
	}

	// Try HTTPS format: https://host/owner/repo.git
	if matches := httpsRemoteRegex.FindStringSubmatch(url); matches != nil {
		return &RemoteInfo{Host: matches[1], Repo: matches[2]}, nil
	}

	return nil, fmt.Errorf("could not parse remote URL: %s", url)
}

// HasRemote checks if a remote named "origin" exists
func (g *GitOps) HasRemote() bool {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = g.workDir
	return cmd.Run() == nil
}
