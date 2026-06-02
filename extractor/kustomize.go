package extractor

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/sholdee/crd-schema-publisher/schemametadata"

	"github.com/invopop/jsonschema"
	"sigs.k8s.io/kustomize/api/types"
)

// WriteKustomizeSchemas reflects kustomize's Kustomization Go type into a
// self-contained JSON schema and writes it like any other kind. Kustomization
// is a client-side type — not a CRD or API-server type — and its published
// OpenAPI is near-empty, so the Go struct kustomize itself uses is the only
// complete, version-pinned source.
func WriteKustomizeSchemas(outputDir string) (int, error) {
	reflector := &jsonschema.Reflector{ExpandedStruct: true, DoNotReference: true}
	raw, err := json.Marshal(reflector.Reflect(&types.Kustomization{}))
	if err != nil {
		return 0, fmt.Errorf("reflecting kustomization schema: %w", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return 0, fmt.Errorf("decoding kustomization schema: %w", err)
	}
	delete(schema, "$id") // reflector emits a Go-package $id that is meaningless when served

	const group, version = "kustomize.config.k8s.io", "v1beta1"
	filename, err := writeSchemaMap(schema, "kustomization", group, version, outputDir)
	if err != nil {
		return 0, err
	}

	relPath := filepath.ToSlash(filepath.Join(group, filename))
	kinds := map[string]string{relPath: "Kustomization"}
	if err := writeKindsManifest(outputDir, kinds); err != nil {
		return 1, err
	}
	metadata := map[string]schemametadata.SchemaMetadataEntry{
		relPath: {Kind: "Kustomization", Source: schemametadata.SchemaSourceKustomize},
	}
	if err := writeSchemaMetadataManifest(outputDir, metadata); err != nil {
		return 1, err
	}
	return 1, nil
}
