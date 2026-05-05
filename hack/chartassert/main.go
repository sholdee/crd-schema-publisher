package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"
)

const (
	chartPath   = "charts/crd-schema-publisher"
	releaseName = "test"

	emptyDirWarning = "WARNING: CronJob extract-only mode is using emptyDir output"
)

type manifest map[string]any

type renderResult struct {
	docs []manifest
	raw  string
}

type renderCase struct {
	name       string
	args       []string
	install    bool
	assertions []assertion
}

type assertion struct {
	name  string
	check func(renderResult) error
}

func main() {
	cases := []renderCase{
		{
			name: "controller with existing secret",
			args: []string{"--set", "existingSecret.name=test"},
			assertions: []assertion{
				{name: "renders Deployment", check: assertKindCount("Deployment", 1)},
				{name: "does not render CronJob", check: assertKindCount("CronJob", 0)},
			},
		},
		{
			name: "cronjob mode",
			args: []string{"--set", "mode=cronjob"},
			assertions: []assertion{
				{name: "renders CronJob", check: assertKindCount("CronJob", 1)},
				{name: "does not render Deployment", check: assertKindCount("Deployment", 0)},
				{name: "does not render controller Role", check: assertKindCount("Role", 0)},
				{name: "omits Cloudflare env vars", check: assertOutputOmits("CLOUDFLARE_", "CF_PAGES_PROJECT")},
			},
		},
		{
			name: "default extract-only mode",
			assertions: []assertion{
				{name: "renders Deployment", check: assertKindCount("Deployment", 1)},
				{name: "omits Cloudflare env vars", check: assertOutputOmits("CLOUDFLARE_", "CF_PAGES_PROJECT")},
			},
		},
		{
			name: "cronjob with Cloudflare secret",
			args: []string{"--set", "mode=cronjob", "--set", "existingSecret.name=test"},
			assertions: []assertion{
				{name: "renders Cloudflare env vars", check: assertOutputContains("CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID", "CF_PAGES_PROJECT")},
			},
		},
		{
			name:    "cronjob extract-only emptyDir warning",
			install: true,
			args:    []string{"--set", "mode=cronjob"},
			assertions: []assertion{
				{name: "renders emptyDir warning", check: assertOutputContains(emptyDirWarning)},
			},
		},
		{
			name:    "cronjob Cloudflare mode omits emptyDir warning",
			install: true,
			args:    []string{"--set", "mode=cronjob", "--set", "existingSecret.name=test"},
			assertions: []assertion{
				{name: "omits emptyDir warning", check: assertOutputOmits(emptyDirWarning)},
			},
		},
		{
			name:    "cronjob persistent extract-only omits emptyDir warning",
			install: true,
			args:    []string{"--set", "mode=cronjob", "--set", "persistence.enabled=true"},
			assertions: []assertion{
				{name: "omits emptyDir warning", check: assertOutputOmits(emptyDirWarning)},
			},
		},
		{
			name:    "cronjob backend extract-only omits emptyDir warning",
			install: true,
			args: []string{
				"--set", "mode=cronjob",
				"--set", "extraContainers[0].name=backend",
				"--set", "extraContainers[0].image=busybox",
			},
			assertions: []assertion{
				{name: "omits emptyDir warning", check: assertOutputOmits(emptyDirWarning)},
			},
		},
		{
			name: "dashboard ConfigMap",
			args: []string{"--set", "existingSecret.name=test", "--set", "grafana.dashboard.enabled=true"},
			assertions: []assertion{
				{name: "dashboard JSON is embedded and valid", check: assertDashboardConfigMapJSON},
			},
		},
		{
			name: "built-in site serving",
			args: []string{"--set", "existingSecret.name=test", "--set", "serve.enabled=true"},
			assertions: []assertion{
				{name: "SERVE_SITE=true", check: assertDeploymentEnv("SERVE_SITE", "true")},
				{name: "SERVE_ACCESS_LOG=false", check: assertDeploymentEnv("SERVE_ACCESS_LOG", "false")},
				{name: "site container port", check: assertDeploymentPort("site", 8081)},
				{name: "Recreate strategy", check: assertPathEquals("Deployment", "deployment strategy", "Recreate", "spec", "strategy", "type")},
				{name: "site service port", check: assertServicePort("site", 8081)},
			},
		},
		{
			name: "built-in site access log",
			args: []string{"--set", "existingSecret.name=test", "--set", "serve.enabled=true", "--set", "serve.accessLog.enabled=true"},
			assertions: []assertion{
				{name: "SERVE_ACCESS_LOG=true", check: assertDeploymentEnv("SERVE_ACCESS_LOG", "true")},
			},
		},
		{
			name: "built-in site NetworkPolicy port",
			args: []string{"--set", "serve.enabled=true", "--set", "networkPolicy.enabled=true"},
			assertions: []assertion{
				{name: "NetworkPolicy allows site port", check: assertNestedValue("NetworkPolicy", 8081, "spec", "ingress", "ports", "port")},
			},
		},
		{
			name: "built-in site CiliumNetworkPolicy port",
			args: []string{"--set", "serve.enabled=true", "--set", "ciliumNetworkPolicy.enabled=true"},
			assertions: []assertion{
				{name: "CiliumNetworkPolicy allows site port", check: assertNestedValue("CiliumNetworkPolicy", "8081", "spec", "ingress", "toPorts", "ports", "port")},
			},
		},
		{
			name: "HTTPRoute",
			args: []string{
				"--set", "existingSecret.name=test",
				"--set", "serve.enabled=true",
				"--set", "serve.httpRoute.enabled=true",
				"--set", "serve.httpRoute.parentRefs[0].name=envoy-gateway",
				"--set", "serve.httpRoute.parentRefs[0].namespace=gateway",
				"--set", "serve.httpRoute.parentRefs[0].sectionName=https",
				"--set-string", "serve.httpRoute.hostnames[0]=kube-schemas-test.mgmt.sholdee.net",
				"--set", "serve.httpRoute.filters[0].type=RequestHeaderModifier",
				"--set", "serve.httpRoute.filters[0].requestHeaderModifier.add[0].name=X-Test",
				"--set-string", "serve.httpRoute.filters[0].requestHeaderModifier.add[0].value=e2e",
			},
			assertions: []assertion{
				{name: "renders HTTPRoute", check: assertKindCount("HTTPRoute", 1)},
				{name: "HTTPRoute name", check: assertPathEquals("HTTPRoute", "name", "test-crd-schema-publisher", "metadata", "name")},
				{name: "parentRef name", check: assertPathEquals("HTTPRoute", "parentRef name", "envoy-gateway", "spec", "parentRefs", 0, "name")},
				{name: "hostname", check: assertPathEquals("HTTPRoute", "hostname", "kube-schemas-test.mgmt.sholdee.net", "spec", "hostnames", 0)},
				{name: "default path", check: assertPathEquals("HTTPRoute", "match path", "/", "spec", "rules", 0, "matches", 0, "path", "value")},
				{name: "backend port", check: assertPathEquals("HTTPRoute", "backend port", 8081, "spec", "rules", 0, "backendRefs", 0, "port")},
				{name: "filter type", check: assertPathEquals("HTTPRoute", "filter type", "RequestHeaderModifier", "spec", "rules", 0, "filters", 0, "type")},
			},
		},
		{
			name: "Grafana Operator dashboard",
			args: []string{
				"--set", "existingSecret.name=test",
				"--set", "grafana.dashboard.operator.enabled=true",
				"--set-string", "grafana.dashboard.operator.datasources[0].inputName=DS_PROMETHEUS",
				"--set-string", "grafana.dashboard.operator.datasources[0].datasourceName=VictoriaMetrics",
			},
			assertions: []assertion{
				{name: "renders GrafanaDashboard", check: assertKindCount("GrafanaDashboard", 1)},
				{name: "ConfigMap ref name", check: assertPathEquals("GrafanaDashboard", "configMapRef name", "test-crd-schema-publisher-dashboard", "spec", "configMapRef", "name")},
				{name: "ConfigMap ref key", check: assertPathEquals("GrafanaDashboard", "configMapRef key", "crd-schema-publisher.json", "spec", "configMapRef", "key")},
				{name: "default selector", check: assertPathEquals("GrafanaDashboard", "default selector", "grafana", "spec", "instanceSelector", "matchLabels", "dashboards")},
				{name: "allow cross namespace", check: assertPathEquals("GrafanaDashboard", "allowCrossNamespaceImport", true, "spec", "allowCrossNamespaceImport")},
				{name: "datasource input", check: assertPathEquals("GrafanaDashboard", "datasource input", "DS_PROMETHEUS", "spec", "datasources", 0, "inputName")},
				{name: "datasource name", check: assertPathEquals("GrafanaDashboard", "datasource name", "VictoriaMetrics", "spec", "datasources", 0, "datasourceName")},
				{name: "operator ConfigMap label", check: assertPathEqualsFor("ConfigMap", "test-crd-schema-publisher-dashboard", "managed-by label", "grafana-operator", "metadata", "labels", "app.kubernetes.io/managed-by")},
			},
		},
		{
			name: "Grafana Operator custom matchLabels",
			args: []string{
				"--set", "existingSecret.name=test",
				"--set", "grafana.dashboard.operator.enabled=true",
				"--set-string", "grafana.dashboard.operator.instanceSelector.matchLabels.grafana\\.internal/instance=home",
			},
			assertions: []assertion{
				{name: "custom selector", check: assertPathEquals("GrafanaDashboard", "custom selector", "home", "spec", "instanceSelector", "matchLabels", "grafana.internal/instance")},
				{name: "omits default selector", check: assertPathMissing("GrafanaDashboard", "default selector", "spec", "instanceSelector", "matchLabels", "dashboards")},
			},
		},
		{
			name: "Grafana Operator custom matchExpressions",
			args: []string{
				"--set", "existingSecret.name=test",
				"--set", "grafana.dashboard.operator.enabled=true",
				"--set-string", "grafana.dashboard.operator.instanceSelector.matchExpressions[0].key=grafana.internal/instance",
				"--set-string", "grafana.dashboard.operator.instanceSelector.matchExpressions[0].operator=In",
				"--set-string", "grafana.dashboard.operator.instanceSelector.matchExpressions[0].values[0]=home",
			},
			assertions: []assertion{
				{name: "custom expression key", check: assertPathEquals("GrafanaDashboard", "custom expression key", "grafana.internal/instance", "spec", "instanceSelector", "matchExpressions", 0, "key")},
				{name: "omits default selector", check: assertPathMissing("GrafanaDashboard", "default selector", "spec", "instanceSelector", "matchLabels", "dashboards")},
			},
		},
		{
			name: "Grafana Operator empty selector from disabled default",
			args: []string{"--set", "existingSecret.name=test", "--set", "grafana.dashboard.operator.enabled=true", "--set", "grafana.dashboard.operator.defaultInstanceSelector.enabled=false"},
			assertions: []assertion{
				{name: "empty instance selector", check: assertMapLen("GrafanaDashboard", 0, "spec", "instanceSelector")},
			},
		},
		{
			name: "Grafana Operator empty selector from null default",
			args: []string{"--set", "existingSecret.name=test", "--set", "grafana.dashboard.operator.enabled=true", "--set-json", "grafana.dashboard.operator.defaultInstanceSelector=null"},
			assertions: []assertion{
				{name: "empty instance selector", check: assertMapLen("GrafanaDashboard", 0, "spec", "instanceSelector")},
			},
		},
		{
			name: "Grafana Operator folder",
			args: []string{"--set", "existingSecret.name=test", "--set", "grafana.dashboard.operator.enabled=true", "--set-string", "grafana.dashboard.operator.folder=Platform"},
			assertions: []assertion{
				{name: "folder", check: assertPathEquals("GrafanaDashboard", "folder", "Platform", "spec", "folder")},
			},
		},
		{
			name: "existingClaim suppresses PVC",
			args: []string{"--set", "existingSecret.name=test", "--set", "persistence.enabled=true", "--set", "persistence.existingClaim=my-pvc"},
			assertions: []assertion{
				{name: "no PVC", check: assertKindCount("PersistentVolumeClaim", 0)},
			},
		},
	}

	failures := runRenderCases(cases)
	failures += runFailureCases([]renderCase{
		{name: "rejects existingSecret with externalSecret", args: []string{"--set", "existingSecret.name=s", "--set", "externalSecret.enabled=true"}},
		{name: "rejects NetworkPolicy with CiliumNetworkPolicy", args: []string{"--set", "existingSecret.name=s", "--set", "networkPolicy.enabled=true", "--set", "ciliumNetworkPolicy.enabled=true"}},
		{name: "rejects serve in cronjob mode", args: []string{"--set", "mode=cronjob", "--set", "serve.enabled=true"}},
		{name: "rejects serve with multiple replicas", args: []string{"--set", "serve.enabled=true", "--set", "replicaCount=2"}},
		{name: "rejects site port equal to health port", args: []string{"--set", "serve.enabled=true", "--set", "serve.port=8080", "--set", "config.healthPort=8080"}},
		{name: "rejects privileged site port", args: []string{"--set", "serve.enabled=true", "--set", "serve.port=80"}},
		{name: "rejects HTTPRoute without serve enabled", args: []string{"--set", "serve.httpRoute.enabled=true", "--set", "serve.httpRoute.parentRefs[0].name=envoy-gateway"}},
		{name: "rejects HTTPRoute without parentRefs", args: []string{"--set", "serve.enabled=true", "--set", "serve.httpRoute.enabled=true"}},
		{name: "rejects sidecar plus operator dashboard", args: []string{"--set", "grafana.dashboard.enabled=true", "--set", "grafana.dashboard.operator.enabled=true"}},
		{name: "rejects folderRef plus folderUID", args: []string{"--set", "grafana.dashboard.operator.folderRef=folder", "--set", "grafana.dashboard.operator.folderUID=folder-uid"}},
		{name: "rejects folder plus folderRef", args: []string{"--set", "grafana.dashboard.operator.folder=Platform", "--set", "grafana.dashboard.operator.folderRef=folder-ref"}},
		{name: "rejects folder plus folderUID", args: []string{"--set", "grafana.dashboard.operator.folder=Platform", "--set", "grafana.dashboard.operator.folderUID=folder-uid"}},
		{name: "rejects invalid instanceSelector keys", args: []string{"--set", "grafana.dashboard.operator.enabled=true", "--set", "grafana.dashboard.operator.instanceSelector.mathLabels.dashboards=grafana"}},
	})

	if failures > 0 {
		os.Exit(1)
	}
	fmt.Println("chart assertions passed")
}

func runRenderCases(cases []renderCase) int {
	var failures int
	for _, tc := range cases {
		result, err := runRenderCase(tc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "case %q render failed: %v\n", tc.name, err)
			failures++
			continue
		}
		for _, a := range tc.assertions {
			if err := a.check(result); err != nil {
				fmt.Fprintf(os.Stderr, "case %q assertion %q failed: %v\n", tc.name, a.name, err)
				failures++
			}
		}
	}
	return failures
}

func runFailureCases(cases []renderCase) int {
	var failures int
	for _, tc := range cases {
		_, err := render(tc.args...)
		if err == nil {
			fmt.Fprintf(os.Stderr, "case %q unexpectedly rendered successfully\n", tc.name)
			failures++
		}
	}
	return failures
}

func runRenderCase(tc renderCase) (renderResult, error) {
	if tc.install {
		out, err := runHelm(append([]string{"install", "--dry-run", "--debug", releaseName, chartPath}, tc.args...)...)
		return renderResult{raw: string(out)}, err
	}
	return render(tc.args...)
}

func render(args ...string) (renderResult, error) {
	out, err := runHelm(append([]string{"template", releaseName, chartPath}, args...)...)
	if err != nil {
		return renderResult{}, err
	}
	docs, err := decodeManifests(out)
	if err != nil {
		return renderResult{}, err
	}
	return renderResult{docs: docs, raw: string(out)}, nil
}

func runHelm(args ...string) ([]byte, error) {
	cmd := exec.Command("helm", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("helm %s: %w\n%s%s", strings.Join(args, " "), err, stderr.String(), string(out))
	}
	return out, nil
}

func decodeManifests(data []byte) ([]manifest, error) {
	dec := yamlv3.NewDecoder(bytes.NewReader(data))
	var docs []manifest
	for {
		var doc manifest
		err := dec.Decode(&doc)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if len(doc) == 0 {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func kind(doc manifest) string {
	v, _ := doc["kind"].(string)
	return v
}

func name(doc manifest) string {
	v, _ := path(doc, "metadata", "name")
	s, _ := v.(string)
	return s
}

func assertKindCount(expectedKind string, expected int) func(renderResult) error {
	return func(result renderResult) error {
		var count int
		for _, doc := range result.docs {
			if kind(doc) == expectedKind {
				count++
			}
		}
		if count != expected {
			return fmt.Errorf("expected %d %s manifests, got %d", expected, expectedKind, count)
		}
		return nil
	}
}

func assertOutputContains(needles ...string) func(renderResult) error {
	return func(result renderResult) error {
		for _, needle := range needles {
			if !strings.Contains(result.raw, needle) {
				return fmt.Errorf("expected rendered output to contain %q", needle)
			}
		}
		return nil
	}
}

func assertOutputOmits(needles ...string) func(renderResult) error {
	return func(result renderResult) error {
		for _, needle := range needles {
			if strings.Contains(result.raw, needle) {
				return fmt.Errorf("expected rendered output to omit %q", needle)
			}
		}
		return nil
	}
}

func assertDashboardConfigMapJSON(result renderResult) error {
	doc, err := findKind("ConfigMap", result.docs)
	if err != nil {
		return err
	}
	v, ok := path(doc, "data", "crd-schema-publisher.json")
	if !ok {
		return fmt.Errorf("dashboard ConfigMap data is missing crd-schema-publisher.json")
	}
	dashboard, ok := v.(string)
	if !ok || dashboard == "" {
		return fmt.Errorf("dashboard ConfigMap data is empty or not a string: %#v", v)
	}
	if !json.Valid([]byte(dashboard)) {
		return fmt.Errorf("dashboard JSON is invalid")
	}
	return nil
}

func assertDeploymentEnv(envName, expected string) func(renderResult) error {
	return func(result renderResult) error {
		deployment, err := findKind("Deployment", result.docs)
		if err != nil {
			return err
		}
		container, err := findContainer(deployment, "crd-schema-publisher")
		if err != nil {
			return err
		}
		env, ok := path(container, "env")
		if !ok {
			return fmt.Errorf("deployment container env not found")
		}
		item, err := findNamedMap(env, envName)
		if err != nil {
			return err
		}
		got, ok := item["value"]
		if !ok {
			return fmt.Errorf("env var %s has no value", envName)
		}
		if fmt.Sprint(got) != expected {
			return fmt.Errorf("env var %s: expected %q, got %#v", envName, expected, got)
		}
		return nil
	}
}

func assertDeploymentPort(portName string, expected int) func(renderResult) error {
	return func(result renderResult) error {
		deployment, err := findKind("Deployment", result.docs)
		if err != nil {
			return err
		}
		container, err := findContainer(deployment, "crd-schema-publisher")
		if err != nil {
			return err
		}
		ports, ok := path(container, "ports")
		if !ok {
			return fmt.Errorf("deployment container ports not found")
		}
		port, err := findNamedMap(ports, portName)
		if err != nil {
			return err
		}
		return compareValue("containerPort", port["containerPort"], expected)
	}
}

func assertServicePort(portName string, expected int) func(renderResult) error {
	return func(result renderResult) error {
		service, err := findKind("Service", result.docs)
		if err != nil {
			return err
		}
		ports, ok := path(service, "spec", "ports")
		if !ok {
			return fmt.Errorf("service ports not found")
		}
		port, err := findNamedMap(ports, portName)
		if err != nil {
			return err
		}
		return compareValue("service port", port["port"], expected)
	}
}

func assertPathEquals(kindName, label string, expected any, keys ...any) func(renderResult) error {
	return func(result renderResult) error {
		doc, err := findKind(kindName, result.docs)
		if err != nil {
			return err
		}
		return assertPathValue(doc, label, expected, keys...)
	}
}

func assertPathEqualsFor(kindName, resourceName, label string, expected any, keys ...any) func(renderResult) error {
	return func(result renderResult) error {
		doc, err := findKindByName(kindName, resourceName, result.docs)
		if err != nil {
			return err
		}
		return assertPathValue(doc, label, expected, keys...)
	}
}

func assertPathValue(doc manifest, label string, expected any, keys ...any) error {
	got, ok := path(doc, keys...)
	if !ok {
		return fmt.Errorf("%s not found at %s", label, formatPath(keys...))
	}
	return compareValue(label, got, expected)
}

func assertPathMissing(kindName, label string, keys ...any) func(renderResult) error {
	return func(result renderResult) error {
		doc, err := findKind(kindName, result.docs)
		if err != nil {
			return err
		}
		if got, ok := path(doc, keys...); ok && got != nil {
			return fmt.Errorf("expected %s to be absent at %s, got %#v", label, formatPath(keys...), got)
		}
		return nil
	}
}

func assertMapLen(kindName string, expected int, keys ...any) func(renderResult) error {
	return func(result renderResult) error {
		doc, err := findKind(kindName, result.docs)
		if err != nil {
			return err
		}
		got, ok := path(doc, keys...)
		if !ok {
			return fmt.Errorf("map not found at %s", formatPath(keys...))
		}
		m, ok := stringMap(got)
		if !ok {
			return fmt.Errorf("expected map at %s, got %T", formatPath(keys...), got)
		}
		if len(m) != expected {
			return fmt.Errorf("expected map length %d at %s, got %d", expected, formatPath(keys...), len(m))
		}
		return nil
	}
}

func assertNestedValue(kindName string, expected any, keys ...string) func(renderResult) error {
	return func(result renderResult) error {
		doc, err := findKind(kindName, result.docs)
		if err != nil {
			return err
		}
		if !containsNestedValue(doc, expected, keys...) {
			return fmt.Errorf("%s did not contain %v at nested path %s", kindName, expected, strings.Join(keys, "."))
		}
		return nil
	}
}

func findKind(expectedKind string, docs []manifest) (manifest, error) {
	for _, doc := range docs {
		if kind(doc) == expectedKind {
			return doc, nil
		}
	}
	return nil, fmt.Errorf("manifest kind %s not found", expectedKind)
}

func findKindByName(expectedKind, expectedName string, docs []manifest) (manifest, error) {
	for _, doc := range docs {
		if kind(doc) == expectedKind && name(doc) == expectedName {
			return doc, nil
		}
	}
	return nil, fmt.Errorf("manifest kind %s named %s not found", expectedKind, expectedName)
}

func findContainer(doc manifest, containerName string) (map[string]any, error) {
	containers, ok := path(doc, "spec", "template", "spec", "containers")
	if !ok {
		return nil, fmt.Errorf("containers not found")
	}
	return findNamedMap(containers, containerName)
}

func findNamedMap(v any, expectedName string) (map[string]any, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected list for named lookup, got %T", v)
	}
	for _, item := range items {
		m, ok := stringMap(item)
		if !ok {
			continue
		}
		if m["name"] == expectedName {
			return m, nil
		}
	}
	return nil, fmt.Errorf("item named %q not found", expectedName)
}

func path(root any, keys ...any) (any, bool) {
	current := root
	for _, key := range keys {
		switch typedKey := key.(type) {
		case string:
			m, ok := stringMap(current)
			if !ok {
				return nil, false
			}
			current, ok = m[typedKey]
			if !ok {
				return nil, false
			}
		case int:
			items, ok := current.([]any)
			if !ok || typedKey < 0 || typedKey >= len(items) {
				return nil, false
			}
			current = items[typedKey]
		default:
			return nil, false
		}
	}
	return current, true
}

func containsNestedValue(v any, expected any, keys ...string) bool {
	if len(keys) == 0 {
		return reflect.DeepEqual(v, expected) || fmt.Sprint(v) == fmt.Sprint(expected)
	}
	key := keys[0]
	if typed, ok := stringMap(v); ok {
		next, ok := typed[key]
		if !ok {
			for _, item := range typed {
				if containsNestedValue(item, expected, keys...) {
					return true
				}
			}
			return false
		}
		return containsNestedValue(next, expected, keys[1:]...)
	}
	switch typed := v.(type) {
	case []any:
		for _, item := range typed {
			if containsNestedValue(item, expected, keys...) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func compareValue(label string, got any, expected any) error {
	if reflect.DeepEqual(got, expected) || fmt.Sprint(got) == fmt.Sprint(expected) {
		return nil
	}
	if expectedInt, ok := expected.(int); ok {
		switch typed := got.(type) {
		case int:
			if typed == expectedInt {
				return nil
			}
		case int64:
			if typed == int64(expectedInt) {
				return nil
			}
		case float64:
			if typed == float64(expectedInt) {
				return nil
			}
		case string:
			if parsed, err := strconv.Atoi(typed); err == nil && parsed == expectedInt {
				return nil
			}
		}
	}
	return fmt.Errorf("%s: expected %#v, got %#v", label, expected, got)
}

func formatPath(keys ...any) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprint(key))
	}
	return strings.Join(parts, ".")
}

func stringMap(v any) (map[string]any, bool) {
	if m, ok := v.(manifest); ok {
		return map[string]any(m), true
	}
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	raw, ok := v.(map[any]any)
	if !ok {
		return nil, false
	}
	converted := make(map[string]any, len(raw))
	for key, value := range raw {
		s, ok := key.(string)
		if !ok {
			return nil, false
		}
		converted[s] = value
	}
	return converted, true
}
