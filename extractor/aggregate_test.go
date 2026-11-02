package extractor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaPathAPIVersion(t *testing.T) {
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{"core/pod_v1.json", "v1", true},
		{"apps/deployment_v1.json", "apps/v1", true},
		{"cert-manager.io/certificate_v1.json", "cert-manager.io/v1", true},
		{"kustomize.config.k8s.io/kustomization_v1beta1.json", "kustomize.config.k8s.io/v1beta1", true},
		{"autoscaling/horizontalpodautoscaler_v2.json", "autoscaling/v2", true},
		{"pod_v1.json", "", false},
		{"a/b/pod_v1.json", "", false},
		{"core/pod.json", "", false},
		{"core/pod_v1.html", "", false},
		{"core/_v1.json", "", false},
		{"core/pod_.json", "", false},
	}
	for _, tc := range cases {
		got, ok := schemaPathAPIVersion(tc.path)
		if ok != tc.ok || got != tc.want {
			t.Errorf("schemaPathAPIVersion(%q) = (%q, %v), want (%q, %v)", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestWriteAggregateSchema_NoManifestWritesNothing(t *testing.T) {
	dir := t.TempDir()
	count, err := WriteAggregateSchema(dir)
	if err != nil {
		t.Fatalf("WriteAggregateSchema: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 entries, got %d", count)
	}
	if _, err := os.Stat(filepath.Join(dir, AggregateSchemaName)); !os.IsNotExist(err) {
		t.Fatalf("expected no %s for an empty catalog, stat err=%v", AggregateSchemaName, err)
	}
}

func TestWriteAggregateSchema_CorruptManifestFails(t *testing.T) {
	dir := t.TempDir()
	if err := writeKindsManifest(dir, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metadataDirName, kindsManifestName), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteAggregateSchema(dir); err == nil {
		t.Fatal("expected an error for a corrupt kinds manifest")
	}
}

func TestWriteAggregateSchema_BuildsDiscriminatedEntries(t *testing.T) {
	dir := t.TempDir()
	kinds := map[string]string{
		"example.io/test_v1.json":                            "Test",
		"core/pod_v1.json":                                   "Pod",
		"core/event_v1.json":                                 "Event",
		"events.k8s.io/event_v1.json":                        "Event",
		"kustomize.config.k8s.io/kustomization_v1beta1.json": "Kustomization",
		"core/README.md":                                     "Ignored",
	}
	if err := writeKindsManifest(dir, kinds); err != nil {
		t.Fatal(err)
	}

	count, err := WriteAggregateSchema(dir)
	if err != nil {
		t.Fatalf("WriteAggregateSchema: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected 5 entries, got %d", count)
	}

	entries := readAggregateEntries(t, dir)
	if len(entries) != 5 {
		t.Fatalf("expected 5 oneOf branches, got %d", len(entries))
	}
	// Sorted by schema path so output is stable across runs.
	wantOrder := []string{
		"core/event_v1.json",
		"core/pod_v1.json",
		"events.k8s.io/event_v1.json",
		"example.io/test_v1.json",
		"kustomize.config.k8s.io/kustomization_v1beta1.json",
	}
	wantAPIVersion := map[string]string{
		"core/event_v1.json":                                 "v1",
		"core/pod_v1.json":                                   "v1",
		"events.k8s.io/event_v1.json":                        "events.k8s.io/v1",
		"example.io/test_v1.json":                            "example.io/v1",
		"kustomize.config.k8s.io/kustomization_v1beta1.json": "kustomize.config.k8s.io/v1beta1",
	}
	for i, entry := range entries {
		if entry.ref != wantOrder[i] {
			t.Fatalf("entry %d: expected ref %q, got %q", i, wantOrder[i], entry.ref)
		}
		if entry.apiVersion != wantAPIVersion[entry.ref] {
			t.Fatalf("entry %s: expected apiVersion %q, got %q", entry.ref, wantAPIVersion[entry.ref], entry.apiVersion)
		}
		if entry.kind != kinds[entry.ref] {
			t.Fatalf("entry %s: expected kind %q, got %q", entry.ref, kinds[entry.ref], entry.kind)
		}
	}
}

type aggregateEntry struct {
	apiVersion string
	kind       string
	ref        string
}

// readAggregateEntries decodes all.json and flattens each oneOf branch into
// its discriminator values and target ref, failing on any shape drift.
func readAggregateEntries(t *testing.T, dir string) []aggregateEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, AggregateSchemaName))
	if err != nil {
		t.Fatalf("reading %s: %v", AggregateSchemaName, err)
	}
	var schema struct {
		Schema string `json:"$schema"`
		OneOf  []struct {
			AllOf []json.RawMessage `json:"allOf"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parsing %s: %v", AggregateSchemaName, err)
	}
	if schema.Schema != draft04 {
		t.Fatalf("expected draft-04 $schema, got %q", schema.Schema)
	}

	entries := make([]aggregateEntry, 0, len(schema.OneOf))
	for i, branch := range schema.OneOf {
		if len(branch.AllOf) != 2 {
			t.Fatalf("branch %d: expected allOf with 2 schemas, got %d", i, len(branch.AllOf))
		}
		var discriminator struct {
			Properties struct {
				APIVersion struct {
					Enum []string `json:"enum"`
				} `json:"apiVersion"`
				Kind struct {
					Enum []string `json:"enum"`
				} `json:"kind"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(branch.AllOf[0], &discriminator); err != nil {
			t.Fatalf("branch %d: parsing discriminator: %v", i, err)
		}
		if len(discriminator.Properties.APIVersion.Enum) != 1 || len(discriminator.Properties.Kind.Enum) != 1 {
			t.Fatalf("branch %d: expected single-value apiVersion and kind enums, got %s", i, branch.AllOf[0])
		}
		if len(discriminator.Required) != 2 {
			t.Fatalf("branch %d: expected apiVersion and kind to be required, got %v", i, discriminator.Required)
		}
		var target struct {
			Ref string `json:"$ref"`
		}
		if err := json.Unmarshal(branch.AllOf[1], &target); err != nil {
			t.Fatalf("branch %d: parsing ref: %v", i, err)
		}
		if target.Ref == "" {
			t.Fatalf("branch %d: expected a $ref target, got %s", i, branch.AllOf[1])
		}
		entries = append(entries, aggregateEntry{
			apiVersion: discriminator.Properties.APIVersion.Enum[0],
			kind:       discriminator.Properties.Kind.Enum[0],
			ref:        target.Ref,
		})
	}
	return entries
}
