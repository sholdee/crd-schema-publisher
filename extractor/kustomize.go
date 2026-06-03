package extractor

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/sholdee/crd-schema-publisher/schemametadata"

	"github.com/invopop/jsonschema"
	"sigs.k8s.io/kustomize/api/types"
)

// WriteKustomizeSchemas reflects kustomize's Kustomization Go type into
// self-contained JSON schemas for kustomize's top-level config kinds.
// Kustomization and Component are client-side types, not CRDs or API-server
// types, and kustomize's published OpenAPI is near-empty. The Go struct
// kustomize itself uses is the complete, version-pinned source.
func WriteKustomizeSchemas(outputDir string) (int, error) {
	specs := []struct {
		filenameKind string
		kind         string
		version      string
		apiVersion   string
	}{
		{
			filenameKind: "kustomization",
			kind:         types.KustomizationKind,
			version:      "v1beta1",
			apiVersion:   types.KustomizationVersion,
		},
		{
			filenameKind: "component",
			kind:         types.ComponentKind,
			version:      "v1alpha1",
			apiVersion:   types.ComponentVersion,
		},
	}

	const group = "kustomize.config.k8s.io"
	kinds := make(map[string]string, len(specs))
	metadata := make(map[string]schemametadata.SchemaMetadataEntry, len(specs))
	for _, spec := range specs {
		schema, err := reflectedKustomizeSchema(spec.kind, spec.apiVersion)
		if err != nil {
			return len(kinds), err
		}

		filename, err := writeSchemaMap(schema, spec.filenameKind, group, spec.version, outputDir)
		if err != nil {
			return len(kinds), err
		}

		relPath := filepath.ToSlash(filepath.Join(group, filename))
		kinds[relPath] = spec.kind
		metadata[relPath] = schemametadata.SchemaMetadataEntry{
			Kind:   spec.kind,
			Source: schemametadata.SchemaSourceKustomize,
		}
	}

	if err := writeKindsManifest(outputDir, kinds); err != nil {
		return len(kinds), err
	}
	if err := writeSchemaMetadataManifest(outputDir, metadata); err != nil {
		return len(kinds), err
	}
	return len(kinds), nil
}

func reflectedKustomizeSchema(kind, apiVersion string) (map[string]interface{}, error) {
	reflector := &jsonschema.Reflector{ExpandedStruct: true, DoNotReference: true}
	raw, err := json.Marshal(reflector.Reflect(&types.Kustomization{}))
	if err != nil {
		return nil, fmt.Errorf("reflecting kustomize schema: %w", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("decoding kustomize schema: %w", err)
	}
	delete(schema, "$id") // reflector emits a Go-package $id that is meaningless when served
	constrainKustomizeTypeMeta(schema, kind, apiVersion)
	return schema, nil
}

func constrainKustomizeTypeMeta(schema map[string]interface{}, kind, apiVersion string) {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		props = make(map[string]interface{}, 2)
		schema["properties"] = props
	}
	props["apiVersion"] = map[string]interface{}{"const": apiVersion, "type": "string"}
	props["kind"] = map[string]interface{}{"const": kind, "type": "string"}
}
