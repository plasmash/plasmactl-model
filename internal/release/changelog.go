package release

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	conventionalcommits "github.com/leodido/go-conventionalcommits"
	"github.com/leodido/go-conventionalcommits/parser"
)

// CommitTypeInfo contains display information for a commit type
type CommitTypeInfo struct {
	Title string
	Order int
}

var commitTypeInfo = map[string]CommitTypeInfo{
	"feat":     {Title: "Features", Order: 1},
	"fix":      {Title: "Bug Fixes", Order: 2},
	"perf":     {Title: "Performance", Order: 3},
	"refactor": {Title: "Refactoring", Order: 4},
	"docs":     {Title: "Documentation", Order: 5},
	"test":     {Title: "Tests", Order: 6},
	"build":    {Title: "Build", Order: 7},
	"ci":       {Title: "CI", Order: 8},
	"chore":    {Title: "Chores", Order: 9},
	"style":    {Title: "Style", Order: 10},
}

// ParsedCommit represents a parsed conventional commit
type ParsedCommit struct {
	Type        string
	Scope       string
	Description string
	Breaking    bool
	Hash        string
}

// ChangelogGenerator generates changelogs from git history
type ChangelogGenerator struct {
	repo   *git.Repository
	parser conventionalcommits.Machine
}

// NewChangelogGenerator creates a new ChangelogGenerator
func NewChangelogGenerator(workDir string) (*ChangelogGenerator, error) {
	repo, err := git.PlainOpenWithOptions(workDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	// Create parser with conventional commit types
	opts := []conventionalcommits.MachineOption{
		conventionalcommits.WithTypes(conventionalcommits.TypesConventional),
		conventionalcommits.WithBestEffort(),
	}
	p := parser.NewMachine(opts...)

	return &ChangelogGenerator{repo: repo, parser: p}, nil
}

// parseCommit parses a commit message using go-conventionalcommits
func (c *ChangelogGenerator) parseCommit(message, hash string) *ParsedCommit {
	// Parse first line only
	firstLine := strings.Split(message, "\n")[0]

	msg, err := c.parser.Parse([]byte(firstLine))
	if err != nil || !msg.Ok() {
		// Not a conventional commit - treat as "other"
		return &ParsedCommit{
			Type:        "other",
			Description: strings.TrimSpace(firstLine),
			Hash:        hash,
		}
	}

	cc := msg.(*conventionalcommits.ConventionalCommit)

	scope := ""
	if cc.Scope != nil {
		scope = *cc.Scope
	}

	return &ParsedCommit{
		Type:        cc.Type,
		Scope:       scope,
		Description: cc.Description,
		Breaking:    cc.Exclamation,
		Hash:        hash,
	}
}

// Generate generates a changelog from the given tag to HEAD
// If fromTag is empty, generates changelog for all commits
func (c *ChangelogGenerator) Generate(fromTag string) (string, error) {
	head, err := c.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	commitIter, err := c.repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return "", fmt.Errorf("failed to get commit log: %w", err)
	}

	// Find the stopping point (fromTag commit)
	var stopHash plumbing.Hash
	if fromTag != "" {
		stopHash, err = c.resolveTag(fromTag)
		if err != nil {
			return "", err
		}
	}

	// Collect commits by type
	commitsByType := make(map[string][]*ParsedCommit)
	var breakingChanges []*ParsedCommit

	err = commitIter.ForEach(func(commit *object.Commit) error {
		if stopHash != plumbing.ZeroHash && commit.Hash == stopHash {
			return errStop
		}

		parsed := c.parseCommit(commit.Message, commit.Hash.String()[:7])
		commitsByType[parsed.Type] = append(commitsByType[parsed.Type], parsed)

		if parsed.Breaking {
			breakingChanges = append(breakingChanges, parsed)
		}

		return nil
	})

	if err != nil && err != errStop {
		return "", err
	}

	return c.formatChangelog(commitsByType, breakingChanges), nil
}

var errStop = fmt.Errorf("stop")

// resolveTag resolves a tag name to its commit hash
func (c *ChangelogGenerator) resolveTag(tagName string) (plumbing.Hash, error) {
	// Try as lightweight tag reference
	ref, err := c.repo.Reference(plumbing.NewTagReferenceName(tagName), true)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("tag %s not found: %w", tagName, err)
	}

	// Check if it's an annotated tag
	tagObj, err := c.repo.TagObject(ref.Hash())
	if err != nil {
		// Lightweight tag - hash is the commit
		return ref.Hash(), nil
	}

	// Annotated tag - return target commit
	return tagObj.Target, nil
}

// formatChangelog formats the collected commits into a markdown changelog
func (c *ChangelogGenerator) formatChangelog(commitsByType map[string][]*ParsedCommit, breakingChanges []*ParsedCommit) string {
	var sb strings.Builder

	// Breaking changes first
	if len(breakingChanges) > 0 {
		sb.WriteString("### ⚠ Breaking Changes\n\n")
		for _, commit := range breakingChanges {
			c.formatCommit(&sb, commit)
		}
		sb.WriteString("\n")
	}

	// Sort types by order
	var types []string
	for t := range commitsByType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool {
		orderI := 99
		orderJ := 99
		if info, ok := commitTypeInfo[types[i]]; ok {
			orderI = info.Order
		}
		if info, ok := commitTypeInfo[types[j]]; ok {
			orderJ = info.Order
		}
		return orderI < orderJ
	})

	for _, t := range types {
		commits := commitsByType[t]
		if len(commits) == 0 {
			continue
		}

		title := t
		if info, ok := commitTypeInfo[t]; ok {
			title = info.Title
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", title))
		for _, commit := range commits {
			c.formatCommit(&sb, commit)
		}
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String())
}

func (c *ChangelogGenerator) formatCommit(sb *strings.Builder, commit *ParsedCommit) {
	if commit.Scope != "" {
		fmt.Fprintf(sb, "- **%s**: %s (%s)\n", commit.Scope, commit.Description, commit.Hash)
	} else {
		fmt.Fprintf(sb, "- %s (%s)\n", commit.Description, commit.Hash)
	}
}

// HasChanges checks if there are any changes since the given tag
func (c *ChangelogGenerator) HasChanges(fromTag string) (bool, error) {
	changelog, err := c.Generate(fromTag)
	if err != nil {
		return false, err
	}
	return changelog != "", nil
}
