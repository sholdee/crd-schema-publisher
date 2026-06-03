package extractor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sholdee/crd-schema-publisher/schemametadata"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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
      "required": ["uid"],
      "properties": {
        "name": {"type": "string"},
        "creationTimestamp": {
          "$ref": "#/definitions/io.k8s.apimachinery.pkg.apis.meta.v1.Time",
          "description": "Populated by the system. Null for lists."
        },
        "uid": {"$ref": "#/definitions/io.k8s.apimachinery.pkg.types.UID"}
      }
    },
    "io.k8s.apimachinery.pkg.apis.meta.v1.Time": {
      "type": "string",
      "format": "date-time"
    },
    "io.k8s.apimachinery.pkg.types.UID": {
      "type": "string"
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

	assertOpenAPISchemaBundlesNullableRefs(t, schema)
	assertOpenAPIWatchEventSkipped(t, out)
	assertOpenAPIKindsManifest(t, out)
}

func TestWriteOpenAPISchemasFromBytesMatchesFilePath(t *testing.T) {
	dir := t.TempDir()
	fileOut := filepath.Join(dir, "file")
	rawOut := filepath.Join(dir, "raw")
	specPath := filepath.Join(dir, "swagger.json")
	if err := os.WriteFile(specPath, []byte(sampleOpenAPI), 0o644); err != nil {
		t.Fatal(err)
	}

	fileCount, err := WriteOpenAPISchemas(specPath, fileOut, SchemaFilter{})
	if err != nil {
		t.Fatalf("WriteOpenAPISchemas: %v", err)
	}
	rawCount, err := WriteOpenAPISchemasFromBytes([]byte(sampleOpenAPI), rawOut, SchemaFilter{})
	if err != nil {
		t.Fatalf("WriteOpenAPISchemasFromBytes: %v", err)
	}
	if rawCount != fileCount {
		t.Fatalf("expected raw count %d to match file count %d", rawCount, fileCount)
	}

	fileSchema, err := os.ReadFile(filepath.Join(fileOut, "apps", "deployment_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	rawSchema, err := os.ReadFile(filepath.Join(rawOut, "apps", "deployment_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rawSchema) != string(fileSchema) {
		t.Fatalf("expected raw schema to match file schema")
	}
}

func assertOpenAPISchemaBundlesNullableRefs(t *testing.T, schema map[string]interface{}) {
	t.Helper()

	// Self-contained: the referenced ObjectMeta is bundled under definitions,
	// and the $ref stays internal to the file.
	defs, ok := schema["definitions"].(map[string]interface{})
	if !ok || defs["io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"] == nil {
		t.Fatalf("expected ObjectMeta bundled under definitions, got %v", schema["definitions"])
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema properties, got %v", schema["properties"])
	}
	meta, ok := props["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata property, got %v", props["metadata"])
	}
	assertAnyOfHasRefAndNull(t, meta, "#/definitions/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta")

	objectMeta, ok := defs["io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ObjectMeta definition to be a map, got %T", defs["io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"])
	}
	objectMetaProps, ok := objectMeta["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ObjectMeta properties to be a map, got %T", objectMeta["properties"])
	}
	creationTimestamp, ok := objectMetaProps["creationTimestamp"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected creationTimestamp to be a map, got %T", objectMetaProps["creationTimestamp"])
	}
	assertAnyOfHasRefAndNull(t, creationTimestamp, "#/definitions/io.k8s.apimachinery.pkg.apis.meta.v1.Time")
	uid, ok := objectMetaProps["uid"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected uid to be a map, got %T", objectMetaProps["uid"])
	}
	if uid["$ref"] != "#/definitions/io.k8s.apimachinery.pkg.types.UID" {
		t.Fatalf("required ref field should stay direct, got %v", uid)
	}
	if defs["io.k8s.apimachinery.pkg.apis.meta.v1.Time"] == nil {
		t.Fatalf("expected Time definition to be bundled")
	}
	if defs["io.k8s.apimachinery.pkg.types.UID"] == nil {
		t.Fatalf("expected UID definition to be bundled")
	}
}

func assertOpenAPIWatchEventSkipped(t *testing.T, out string) {
	t.Helper()
	// WatchEvent must not be written.
	if _, err := os.Stat(filepath.Join(out, "core", "watchevent_v1.json")); !os.IsNotExist(err) {
		t.Fatalf("expected WatchEvent to be skipped")
	}
}

func assertOpenAPIKindsManifest(t *testing.T, out string) {
	t.Helper()
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
	metadata := readSchemaMetadata(t, out)
	entry := metadata["apps/deployment_v1.json"]
	if entry.Kind != "Deployment" || entry.Source != schemametadata.SchemaSourceBuiltin {
		t.Fatalf("expected built-in schema metadata for Deployment, got %#v", entry)
	}
}

func assertAnyOfHasRefAndNull(t *testing.T, schema map[string]interface{}, ref string) {
	t.Helper()
	if _, hasRef := schema["$ref"]; hasRef {
		t.Fatalf("nullable ref wrapper must not keep sibling $ref, got %v", schema)
	}
	anyOf, ok := schema["anyOf"].([]interface{})
	if !ok {
		t.Fatalf("expected anyOf array, got %T", schema["anyOf"])
	}
	hasRef := false
	hasNull := false
	for _, item := range anyOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected anyOf branch to be map, got %T", item)
		}
		if branch["$ref"] == ref {
			hasRef = true
		}
		if branch["type"] == "null" {
			hasNull = true
		}
	}
	if !hasRef || !hasNull {
		t.Fatalf("expected anyOf to contain %q and null, got %v", ref, anyOf)
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

func TestWriteSchemaMetadataManifestMerges(t *testing.T) {
	dir := t.TempDir()
	if err := writeSchemaMetadataManifest(dir, map[string]schemametadata.SchemaMetadataEntry{
		"apps/deployment_v1.json": {Kind: "Deployment", Source: schemametadata.SchemaSourceBuiltin},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeSchemaMetadataManifest(dir, map[string]schemametadata.SchemaMetadataEntry{
		"cert-manager.io/certificate_v1.json": {Kind: "Certificate", Source: schemametadata.SchemaSourceCRD},
	}); err != nil {
		t.Fatal(err)
	}

	metadata := readSchemaMetadata(t, dir)
	if metadata["apps/deployment_v1.json"].Source != schemametadata.SchemaSourceBuiltin {
		t.Fatalf("expected built-in metadata to survive merge, got %v", metadata)
	}
	if metadata["cert-manager.io/certificate_v1.json"].Source != schemametadata.SchemaSourceCRD {
		t.Fatalf("expected CRD metadata to be added, got %v", metadata)
	}
}

func TestSchemaMetadataManifestMergesAcrossWriters(t *testing.T) {
	dir := t.TempDir()

	if count, err := WriteSchemas([]apiextensionsv1.CustomResourceDefinition{fakeCRD()}, dir); err != nil {
		t.Fatalf("WriteSchemas: %v", err)
	} else if count != 1 {
		t.Fatalf("expected 1 CRD schema, got %d", count)
	}
	if count, err := WriteOpenAPISchemasFromBytes([]byte(sampleOpenAPI), dir, SchemaFilter{}); err != nil {
		t.Fatalf("WriteOpenAPISchemasFromBytes: %v", err)
	} else if count != 1 {
		t.Fatalf("expected 1 built-in schema, got %d", count)
	}
	if count, err := WriteKustomizeSchemas(dir); err != nil {
		t.Fatalf("WriteKustomizeSchemas: %v", err)
	} else if count != 2 {
		t.Fatalf("expected 2 Kustomize schemas, got %d", count)
	}

	metadata := readSchemaMetadata(t, dir)
	wantMetadata := map[string]schemametadata.SchemaMetadataEntry{
		"example.io/test_v1.json":                            {Kind: "Test", Source: schemametadata.SchemaSourceCRD},
		"apps/deployment_v1.json":                            {Kind: "Deployment", Source: schemametadata.SchemaSourceBuiltin},
		"kustomize.config.k8s.io/kustomization_v1beta1.json": {Kind: "Kustomization", Source: schemametadata.SchemaSourceKustomize},
		"kustomize.config.k8s.io/component_v1alpha1.json":    {Kind: "Component", Source: schemametadata.SchemaSourceKustomize},
	}
	if len(metadata) != len(wantMetadata) {
		t.Fatalf("expected metadata for primary schema files only, got %v", metadata)
	}
	for path, want := range wantMetadata {
		if got := metadata[path]; got != want {
			t.Fatalf("expected metadata[%q]=%#v, got %#v in %v", path, want, got, metadata)
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, "_meta", "kinds.json"))
	if err != nil {
		t.Fatalf("reading kinds manifest: %v", err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(data, &kinds); err != nil {
		t.Fatalf("expected kinds manifest to remain path-to-kind map: %v", err)
	}
	for path, want := range map[string]string{
		"example.io/test_v1.json":                            "Test",
		"apps/deployment_v1.json":                            "Deployment",
		"kustomize.config.k8s.io/kustomization_v1beta1.json": "Kustomization",
		"kustomize.config.k8s.io/component_v1alpha1.json":    "Component",
	} {
		if kinds[path] != want {
			t.Fatalf("expected kinds[%q]=%q, got %q in %v", path, want, kinds[path], kinds)
		}
	}
}
