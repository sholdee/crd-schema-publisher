package releaselabels

import (
	"regexp"
	"strings"
)

// PullRequest contains the PR fields used for release-note labeling.
type PullRequest struct {
	Title  string
	Body   string
	Author string
}

// Result describes the release labels derived from a PR.
type Result struct {
	Labels     []string
	Recognized bool
}

// LabelSpec is the repository label definition managed by the labeler.
type LabelSpec struct {
	Name        string
	Color       string
	Description string
}

var labelSpecs = []LabelSpec{
	{Name: "feat", Color: "0e8a16", Description: "Release notes: user-facing feature"},
	{Name: "fix", Color: "d73a4a", Description: "Release notes: bug fix"},
	{Name: "docs", Color: "0075ca", Description: "Release notes: documentation"},
	{Name: "deps", Color: "0366d6", Description: "Release notes: dependency update"},
	{Name: "refactor", Color: "fbca04", Description: "Release notes: code refactoring"},
	{Name: "perf", Color: "1d76db", Description: "Release notes: performance improvement"},
	{Name: "revert", Color: "b60205", Description: "Release notes: reverted change"},
	{Name: "ci", Color: "5319e7", Description: "Release notes: CI workflow maintenance"},
	{Name: "build", Color: "5319e7", Description: "Release notes: build or packaging maintenance"},
	{Name: "test", Color: "5319e7", Description: "Release notes: test maintenance"},
	{Name: "chore", Color: "cfd3d7", Description: "Release notes: maintenance"},
	{Name: "breaking", Color: "b60205", Description: "Release notes: breaking change"},
}

var primaryLabelNames = []string{"feat", "fix", "docs", "deps", "refactor", "perf", "revert", "ci", "build", "test", "chore"}

var (
	conventionalTitleRE = regexp.MustCompile(`(?i)^([a-z]+)(?:\(([^)]+)\))?(!)?:\s+.+`)
	breakingTitleRE     = regexp.MustCompile(`(?i)^[a-z]+(?:\([^)]+\))?!:`)
	breakingBodyRE      = regexp.MustCompile(`(?im)^BREAKING[ -]CHANGE:`)
	titleLabels         = map[string]string{
		"feat":     "feat",
		"fix":      "fix",
		"docs":     "docs",
		"deps":     "deps",
		"ci":       "ci",
		"build":    "build",
		"test":     "test",
		"chore":    "chore",
		"refactor": "refactor",
		"perf":     "perf",
		"revert":   "revert",
		"style":    "chore",
	}
	primaryLabels = map[string]struct{}{
		"feat":     {},
		"fix":      {},
		"docs":     {},
		"deps":     {},
		"refactor": {},
		"perf":     {},
		"revert":   {},
		"ci":       {},
		"build":    {},
		"test":     {},
		"chore":    {},
	}
)

// LabelSpecs returns a defensive copy of the managed label definitions.
func LabelSpecs() []LabelSpec {
	specs := make([]LabelSpec, len(labelSpecs))
	copy(specs, labelSpecs)
	return specs
}

// ManagedLabelNames returns the labels that the PR labeler may add or remove.
func ManagedLabelNames() []string {
	names := make([]string, 0, len(labelSpecs))
	for _, spec := range labelSpecs {
		names = append(names, spec.Name)
	}
	return names
}

// PrimaryLabelNames returns labels accepted as the main release-note type.
func PrimaryLabelNames() []string {
	names := make([]string, len(primaryLabelNames))
	copy(names, primaryLabelNames)
	return names
}

// ForPullRequest maps a PR title/body/author to release-note labels.
func ForPullRequest(pr PullRequest) Result {
	var labels []string
	primary := "deps"
	if !isBotAuthor(pr.Author) {
		primary = labelForTitle(pr.Title)
	}
	if primary != "" {
		labels = append(labels, primary)
	}
	if primary != "" && isBreakingChange(pr.Title, pr.Body) {
		labels = append(labels, "breaking")
	}

	return Result{
		Labels:     labels,
		Recognized: hasPrimaryLabel(labels),
	}
}

func labelForTitle(title string) string {
	match := conventionalTitleRE.FindStringSubmatch(strings.TrimSpace(title))
	if match == nil {
		return ""
	}

	typ := strings.ToLower(match[1])
	scope := strings.ToLower(match[2])
	if typ == "deps" || scope == "deps" || scope == "dependencies" {
		return "deps"
	}
	return titleLabels[typ]
}

func isBreakingChange(title, body string) bool {
	return breakingTitleRE.MatchString(strings.TrimSpace(title)) || breakingBodyRE.MatchString(body)
}

func hasPrimaryLabel(labels []string) bool {
	for _, label := range labels {
		if _, ok := primaryLabels[label]; ok {
			return true
		}
	}
	return false
}

func isBotAuthor(author string) bool {
	login := strings.ToLower(author)
	return strings.HasSuffix(login, "[bot]") ||
		strings.HasPrefix(login, "app/") ||
		strings.Contains(login, "dependabot") ||
		strings.Contains(login, "renovate") ||
		strings.Contains(login, "pull-bunyan")
}
