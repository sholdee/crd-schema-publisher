package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/sholdee/crd-schema-publisher/hack/releaselabels"
)

type eventPayload struct {
	PullRequest *struct {
		Number int `json:"number"`
	} `json:"pull_request"`
}

type pullRequest struct {
	Number int
	Title  string
	Body   string
	Author string
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

	client := ghClient{}
	pr, err := currentPullRequest(client, repository, event.PullRequest.Number)
	if err != nil {
		return err
	}

	result := releaselabels.ForPullRequest(releaselabels.PullRequest{
		Title:  pr.Title,
		Body:   pr.Body,
		Author: pr.Author,
	})

	if err := syncPullRequestLabels(client, repository, pr.Number, result.Labels); err != nil {
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

type ghRunner interface {
	run(args ...string) (string, error)
}

type ghClient struct{}

func currentPullRequest(client ghRunner, repository string, number int) (pullRequest, error) {
	output, err := client.run("api", fmt.Sprintf("repos/%s/pulls/%d", repository, number))
	if err != nil {
		return pullRequest{}, fmt.Errorf("fetching current pull request: %w", err)
	}

	var response struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		User   struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return pullRequest{}, fmt.Errorf("parsing current pull request: %w", err)
	}

	return pullRequest{
		Number: response.Number,
		Title:  response.Title,
		Body:   response.Body,
		Author: response.User.Login,
	}, nil
}

func syncPullRequestLabels(client ghRunner, repository string, number int, desiredLabels []string) error {
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

	args := []string{"api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/labels", repository, number)}
	for _, label := range desiredLabels {
		args = append(args, "-f", fmt.Sprintf("labels[]=%s", label))
	}
	if _, err := client.run(args...); err != nil {
		return fmt.Errorf("applying labels: %w", err)
	}
	return nil
}

func ensureRepositoryLabels(client ghRunner, repository string) error {
	for _, label := range releaselabels.LabelSpecs() {
		if err := ensureRepositoryLabel(client, repository, label); err != nil {
			return err
		}
	}
	return nil
}

func ensureRepositoryLabel(client ghRunner, repository string, label releaselabels.LabelSpec) error {
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

func currentIssueLabels(client ghRunner, repository string, number int) ([]string, error) {
	output, err := client.run("api", "--paginate", fmt.Sprintf("repos/%s/issues/%d/labels?per_page=100", repository, number), "--jq", ".[].name")
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

func deleteIssueLabel(client ghRunner, repository string, number int, label string) error {
	if _, err := client.run("api", "-X", "DELETE", fmt.Sprintf("repos/%s/issues/%d/labels/%s", repository, number, url.PathEscape(label))); err != nil {
		if strings.Contains(err.Error(), "Not Found") {
			return nil
		}
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
