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
	if count != 2 {
		t.Fatalf("expected 2 schemas, got %d", count)
	}

	assertKustomizeSchema(t, dir, "kustomization", "v1beta1", "Kustomization", "kustomize.config.k8s.io/v1beta1")
	assertKustomizeSchema(t, dir, "component", "v1alpha1", "Component", "kustomize.config.k8s.io/v1alpha1")

	manifest, err := os.ReadFile(filepath.Join(dir, "_meta", "kinds.json"))
	if err != nil {
		t.Fatal(err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(manifest, &kinds); err != nil {
		t.Fatal(err)
	}
	if kinds["kustomize.config.k8s.io/kustomization_v1beta1.json"] != "Kustomization" {
		t.Fatalf("expected Kustomization kinds manifest entry, got %v", kinds)
	}
	if kinds["kustomize.config.k8s.io/component_v1alpha1.json"] != "Component" {
		t.Fatalf("expected Component kinds manifest entry, got %v", kinds)
	}
	metadata := readSchemaMetadata(t, dir)
	for path, kind := range map[string]string{
		"kustomize.config.k8s.io/kustomization_v1beta1.json": "Kustomization",
		"kustomize.config.k8s.io/component_v1alpha1.json":    "Component",
	} {
		entry := metadata[path]
		if entry.Kind != kind || entry.Source != schemametadata.SchemaSourceKustomize {
			t.Fatalf("expected %s schema metadata, got %#v", kind, entry)
		}
	}
}

func assertKustomizeSchema(t *testing.T, dir, filenameKind, version, kind, apiVersion string) {
	t.Helper()
	file := filenameKind + "_" + version + ".json"
	data, err := os.ReadFile(filepath.Join(dir, "kustomize.config.k8s.io", file))
	if err != nil {
		t.Fatalf("reading %s schema: %v", kind, err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	props, _ := schema["properties"].(map[string]interface{})
	for _, want := range []string{"resources", "patches", "namespace"} {
		if _, ok := props[want]; !ok {
			t.Fatalf("reflected %s schema missing %q (got %d properties)", kind, want, len(props))
		}
	}
	for field, want := range map[string]string{"apiVersion": apiVersion, "kind": kind} {
		prop, _ := props[field].(map[string]interface{})
		if prop["const"] != want {
			t.Fatalf("expected %s.%s const %q, got %#v", kind, field, want, prop)
		}
	}
	standalone := filepath.Join(dir, "master-standalone", "kustomize.config.k8s.io-"+filenameKind+"-stable-"+version+".json")
	if _, err := os.Stat(standalone); err != nil {
		t.Fatalf("expected standalone %s schema: %v", kind, err)
	}
}
