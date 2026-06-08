package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

type fakeGhClient struct {
	outputs map[string]string
	calls   [][]string
}

func (f *fakeGhClient) run(args ...string) (string, error) {
	f.calls = append(f.calls, slices.Clone(args))
	for prefix, output := range f.outputs {
		if strings.HasPrefix(strings.Join(args, " "), prefix) {
			return output, nil
		}
	}
	return "", nil
}

func TestCurrentPullRequestFetchesCurrentMetadata(t *testing.T) {
	client := &fakeGhClient{
		outputs: map[string]string{
			"api repos/sholdee/crd-schema-publisher/pulls/156": `{"number":156,"title":"fix(renderer): relax schema description wrapping","body":"","user":{"login":"sholdee"}}`,
		},
	}

	got, err := currentPullRequest(client, "sholdee/crd-schema-publisher", 156)
	if err != nil {
		t.Fatalf("currentPullRequest() error: %v", err)
	}

	want := pullRequest{
		Number: 156,
		Title:  "fix(renderer): relax schema description wrapping",
		Body:   "",
		Author: "sholdee",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("currentPullRequest() = %#v, want %#v", got, want)
	}
}

func TestSyncPullRequestLabelsUsesRestIssueLabels(t *testing.T) {
	client := &fakeGhClient{
		outputs: map[string]string{
			"api --paginate repos/sholdee/crd-schema-publisher/issues/156/labels": "docs\ncustom\n",
		},
	}

	if err := syncPullRequestLabels(client, "sholdee/crd-schema-publisher", 156, []string{"fix"}); err != nil {
		t.Fatalf("syncPullRequestLabels() error: %v", err)
	}

	if !hasCall(client.calls, "api", "-X", "DELETE", "repos/sholdee/crd-schema-publisher/issues/156/labels/docs") {
		t.Fatal("expected stale managed docs label to be removed through REST issue-label endpoint")
	}
	if hasCall(client.calls, "api", "-X", "DELETE", "repos/sholdee/crd-schema-publisher/issues/156/labels/custom") {
		t.Fatal("non-managed custom label should not be removed")
	}
	if !hasCall(client.calls, "api", "-X", "POST", "repos/sholdee/crd-schema-publisher/issues/156/labels", "-f", "labels[]=fix") {
		t.Fatal("expected desired label to be applied through REST issue-label endpoint")
	}
	if hasCommand(client.calls, "issue", "edit") {
		t.Fatal("label sync must not use gh issue edit because pull_request_target tokens cannot use its GraphQL mutation")
	}
}

func hasCall(calls [][]string, want ...string) bool {
	for _, call := range calls {
		if slices.Equal(call, want) {
			return true
		}
	}
	return false
}

func hasCommand(calls [][]string, prefix ...string) bool {
	for _, call := range calls {
		if len(call) >= len(prefix) && slices.Equal(call[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}
