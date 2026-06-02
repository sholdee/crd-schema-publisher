package extractor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const sampleOpenAPI = `{
  "definitions": {
    "io.k8s.api.apps.v1.Deployment": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "apps", "version": "v1", "kind": "Deployment"}],
      "properties": {
        "apiVersion": {"type": "string"},
        "kind": {"type": "string"},
        "metadata": {"$ref": "#/definitions/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"}
      }
    },
    "io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta": {
      "type": "object",
      "properties": {"name": {"type": "string"}}
    },
    "io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent": {
      "type": "object",
      "x-kubernetes-group-version-kind": [{"group": "", "version": "v1", "kind": "WatchEvent"}],
      "properties": {"type": {"type": "string"}}
    }
  }
}`

func TestWriteOpenAPISchemas(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "swagger.json")
	if err := os.WriteFile(specPath, []byte(sampleOpenAPI), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")

	count, err := WriteOpenAPISchemas(specPath, out, SchemaFilter{})
	if err != nil {
		t.Fatalf("WriteOpenAPISchemas: %v", err)
	}
	// Deployment emitted; WatchEvent skipped (GVK but no apiVersion/kind props).
	if count != 1 {
		t.Fatalf("expected 1 schema, got %d", count)
	}

	data, err := os.ReadFile(filepath.Join(out, "apps", "deployment_v1.json"))
	if err != nil {
		t.Fatalf("reading deployment schema: %v", err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}

	// Self-contained: the referenced ObjectMeta is bundled under definitions,
	// and the $ref stays internal to the file.
	defs, ok := schema["definitions"].(map[string]interface{})
	if !ok || defs["io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"] == nil {
		t.Fatalf("expected ObjectMeta bundled under definitions, got %v", schema["definitions"])
	}
	meta := schema["properties"].(map[string]interface{})["metadata"].(map[string]interface{})
	if ref := meta["$ref"]; ref != "#/definitions/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta" {
		t.Fatalf("expected internal $ref, got %v", ref)
	}

	// WatchEvent must not be written.
	if _, err := os.Stat(filepath.Join(out, "core", "watchevent_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected WatchEvent to be skipped")
	}

	// kinds manifest maps the file to its kind so the index/search picks it up.
	manifest, err := os.ReadFile(filepath.Join(out, "_meta", "kinds.json"))
	if err != nil {
		t.Fatalf("reading kinds manifest: %v", err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(manifest, &kinds); err != nil {
		t.Fatal(err)
	}
	if kinds["apps/deployment_v1.json"] != "Deployment" {
		t.Fatalf("expected kinds manifest entry for Deployment, got %v", kinds)
	}
}

func TestWriteKindsManifestMerges(t *testing.T) {
	dir := t.TempDir()
	if err := writeKindsManifest(dir, map[string]string{"apps/deployment_v1.json": "Deployment"}); err != nil {
		t.Fatal(err)
	}
	if err := writeKindsManifest(dir, map[string]string{"cert-manager.io/certificate_v1.json": "Certificate"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "_meta", "kinds.json"))
	if err != nil {
		t.Fatal(err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(data, &kinds); err != nil {
		t.Fatal(err)
	}
	if kinds["apps/deployment_v1.json"] != "Deployment" || kinds["cert-manager.io/certificate_v1.json"] != "Certificate" {
		t.Fatalf("expected both passes merged, got %v", kinds)
	}
}
