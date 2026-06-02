package extractor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sholdee/crd-schema-publisher/schemametadata"
)

func TestWriteKustomizeSchemas(t *testing.T) {
	dir := t.TempDir()

	count, err := WriteKustomizeSchemas(dir)
	if err != nil {
		t.Fatalf("WriteKustomizeSchemas: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 schema, got %d", count)
	}

	data, err := os.ReadFile(filepath.Join(dir, "kustomize.config.k8s.io", "kustomization_v1beta1.json"))
	if err != nil {
		t.Fatalf("reading kustomization schema: %v", err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	props, _ := schema["properties"].(map[string]interface{})
	for _, want := range []string{"resources", "patches", "namespace"} {
		if _, ok := props[want]; !ok {
			t.Fatalf("reflected schema missing %q (got %d properties)", want, len(props))
		}
	}

	manifest, err := os.ReadFile(filepath.Join(dir, "_meta", "kinds.json"))
	if err != nil {
		t.Fatal(err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(manifest, &kinds); err != nil {
		t.Fatal(err)
	}
	if kinds["kustomize.config.k8s.io/kustomization_v1beta1.json"] != "Kustomization" {
		t.Fatalf("expected kinds manifest entry, got %v", kinds)
	}
	metadata := readSchemaMetadata(t, dir)
	entry := metadata["kustomize.config.k8s.io/kustomization_v1beta1.json"]
	if entry.Kind != "Kustomization" || entry.Source != schemametadata.SchemaSourceKustomize {
		t.Fatalf("expected Kustomize schema metadata, got %#v", entry)
	}
}
