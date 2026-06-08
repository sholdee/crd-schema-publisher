package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sholdee/crd-schema-publisher/hack/releaselabels"
)

type eventPayload struct {
	PullRequest *struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"pull_request"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	repository := os.Getenv("GITHUB_REPOSITORY")
	if eventPath == "" {
		return errors.New("GITHUB_EVENT_PATH is required")
	}
	if repository == "" {
		return errors.New("GITHUB_REPOSITORY is required")
	}

	event, err := readEvent(eventPath)
	if err != nil {
		return err
	}
	if event.PullRequest == nil {
		return errors.New("release-labeler requires a pull_request event payload")
	}

	result := releaselabels.ForPullRequest(releaselabels.PullRequest{
		Title:  event.PullRequest.Title,
		Body:   event.PullRequest.Body,
		Author: event.PullRequest.User.Login,
	})

	client := ghClient{}
	if err := syncPullRequestLabels(client, repository, event.PullRequest.Number, result.Labels); err != nil {
		return err
	}
	if !result.Recognized {
		return fmt.Errorf(
			"PR title must start with a release-note type: %s\nExamples: feat(index): add grouping, fix(renderer): improve layout, docs: update install guide",
			strings.Join(releaselabels.PrimaryLabelNames(), ", "),
		)
	}

	fmt.Printf("Applied release labels: %s\n", strings.Join(result.Labels, ", "))
	return nil
}

func readEvent(path string) (eventPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return eventPayload{}, fmt.Errorf("reading event payload: %w", err)
	}

	var event eventPayload
	if err := json.Unmarshal(data, &event); err != nil {
		return eventPayload{}, fmt.Errorf("parsing event payload: %w", err)
	}
	return event, nil
}

type ghClient struct{}

func syncPullRequestLabels(client ghClient, repository string, number int, desiredLabels []string) error {
	if err := ensureRepositoryLabels(client, repository); err != nil {
		return err
	}

	existing, err := currentIssueLabels(client, repository, number)
	if err != nil {
		return err
	}

	desired := stringSet(desiredLabels)
	managed := stringSet(releaselabels.ManagedLabelNames())
	for _, label := range existing {
		if _, isManaged := managed[label]; !isManaged {
			continue
		}
		if _, shouldKeep := desired[label]; shouldKeep {
			continue
		}
		if err := deleteIssueLabel(client, repository, number, label); err != nil {
			return err
		}
	}

	if len(desiredLabels) == 0 {
		return nil
	}

	if _, err := client.run("issue", "edit", strconv.Itoa(number), "--repo", repository, "--add-label", strings.Join(desiredLabels, ",")); err != nil {
		return fmt.Errorf("applying labels: %w", err)
	}
	return nil
}

func ensureRepositoryLabels(client ghClient, repository string) error {
	for _, label := range releaselabels.LabelSpecs() {
		if err := ensureRepositoryLabel(client, repository, label); err != nil {
			return err
		}
	}
	return nil
}

func ensureRepositoryLabel(client ghClient, repository string, label releaselabels.LabelSpec) error {
	if _, err := client.run(
		"label",
		"create",
		label.Name,
		"--repo",
		repository,
		"--color",
		label.Color,
		"--description",
		label.Description,
		"--force",
	); err != nil {
		return fmt.Errorf("ensuring label %q: %w", label.Name, err)
	}
	return nil
}

func currentIssueLabels(client ghClient, repository string, number int) ([]string, error) {
	output, err := client.run("api", fmt.Sprintf("repos/%s/issues/%d/labels", repository, number), "--jq", ".[].name")
	if err != nil {
		return nil, fmt.Errorf("listing current labels: %w", err)
	}

	var labels []string
	for _, line := range strings.Split(output, "\n") {
		label := strings.TrimSpace(line)
		if label != "" {
			labels = append(labels, label)
		}
	}
	return labels, nil
}

func deleteIssueLabel(client ghClient, repository string, number int, label string) error {
	if _, err := client.run("issue", "edit", strconv.Itoa(number), "--repo", repository, "--remove-label", label); err != nil {
		return fmt.Errorf("deleting stale label %q: %w", label, err)
	}
	return nil
}

func (ghClient) run(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Env = os.Environ()
	if token := firstNonEmpty(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")); token != "" {
		cmd.Env = append(cmd.Env, "GH_TOKEN="+token)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
