package releaselabels

import (
	"reflect"
	"testing"
)

func TestForPullRequestMapsConventionalTitles(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want Result
	}{
		{
			name: "feature with scope",
			pr:   PullRequest{Title: "feat(index): group schemas by source"},
			want: Result{Labels: []string{"feat"}, Recognized: true},
		},
		{
			name: "fix",
			pr:   PullRequest{Title: "fix: repair generated schema layout"},
			want: Result{Labels: []string{"fix"}, Recognized: true},
		},
		{
			name: "docs",
			pr:   PullRequest{Title: "docs(site): add installation guide"},
			want: Result{Labels: []string{"docs"}, Recognized: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForPullRequest(tt.pr); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ForPullRequest() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestForPullRequestMapsDependencyUpdates(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
	}{
		{name: "dependency scope", pr: PullRequest{Title: "chore(deps): update renovate"}},
		{name: "dependency bot", pr: PullRequest{Title: "Update dependency golang", Author: "pull-bunyan[bot]"}},
		{name: "dependency bot with recognized title", pr: PullRequest{Title: "fix: update Kubernetes dependency", Author: "pull-bunyan[bot]"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := Result{Labels: []string{"deps"}, Recognized: true}
			if got := ForPullRequest(tt.pr); !reflect.DeepEqual(got, want) {
				t.Fatalf("ForPullRequest() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestForPullRequestMapsCommonReleasePleaseCategories(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want Result
	}{
		{
			name: "refactor",
			pr:   PullRequest{Title: "refactor(renderer): simplify property rows"},
			want: Result{Labels: []string{"refactor"}, Recognized: true},
		},
		{
			name: "performance",
			pr:   PullRequest{Title: "perf(renderer): speed up schema search"},
			want: Result{Labels: []string{"perf"}, Recognized: true},
		},
		{
			name: "revert",
			pr:   PullRequest{Title: "revert: feat(index): group schemas by source"},
			want: Result{Labels: []string{"revert"}, Recognized: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForPullRequest(tt.pr); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ForPullRequest() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestForPullRequestGroupsStyleUnderMaintenance(t *testing.T) {
	want := Result{Labels: []string{"chore"}, Recognized: true}
	if got := ForPullRequest(PullRequest{Title: "style: format docs"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ForPullRequest() = %#v, want %#v", got, want)
	}
}

func TestForPullRequestAddsBreakingLabel(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want Result
	}{
		{
			name: "bang marker",
			pr:   PullRequest{Title: "feat!: remove legacy output path"},
			want: Result{Labels: []string{"feat", "breaking"}, Recognized: true},
		},
		{
			name: "body footer",
			pr: PullRequest{
				Title: "fix(renderer): remove legacy behavior",
				Body:  "BREAKING CHANGE: generated pages no longer support old markup.",
			},
			want: Result{Labels: []string{"fix", "breaking"}, Recognized: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForPullRequest(tt.pr); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ForPullRequest() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestForPullRequestRejectsHumanTitlesWithoutReleaseType(t *testing.T) {
	want := Result{Labels: nil, Recognized: false}
	if got := ForPullRequest(PullRequest{Title: "improve the docs site"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ForPullRequest() = %#v, want %#v", got, want)
	}
}

func TestManagedLabelNames(t *testing.T) {
	want := []string{"feat", "fix", "docs", "deps", "refactor", "perf", "revert", "ci", "build", "test", "chore", "breaking"}
	if got := ManagedLabelNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ManagedLabelNames() = %#v, want %#v", got, want)
	}
}

func TestPrimaryLabelNames(t *testing.T) {
	want := []string{"feat", "fix", "docs", "deps", "refactor", "perf", "revert", "ci", "build", "test", "chore"}
	if got := PrimaryLabelNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PrimaryLabelNames() = %#v, want %#v", got, want)
	}
}
