package extractor

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// AggregateSchemaName is the catalog-wide schema written at the output root.
// It lets an editor associate every manifest with one URL: each entry pins
// apiVersion and kind with single-value enums, then defers to the per-kind
// schema by relative $ref, so exactly one branch can match a document.
const AggregateSchemaName = "all.json"

// coreGroupDirName is the output directory for the empty (core) API group.
const coreGroupDirName = "core"

const aggregateSchemaDescription = "Every schema published in this catalog. Each oneOf branch matches one apiVersion and kind exactly, then validates the document against that kind's schema."

// WriteAggregateSchema writes all.json into outputDir from the kinds manifest
// that the CRD, built-in, and Kustomize writers accumulate. It returns the
// number of catalog entries. When the manifest is absent or empty, nothing is
// written: an empty oneOf is not a valid schema, and an empty catalog has
// nothing to associate.
func WriteAggregateSchema(outputDir string) (int, error) {
	kinds, err := readKindsManifest(outputDir)
	if err != nil {
		return 0, err
	}
	entries := aggregateEntries(kinds)
	if len(entries) == 0 {
		return 0, nil
	}

	schema := map[string]interface{}{
		"$schema":     draft04,
		"description": aggregateSchemaDescription,
		"oneOf":       entries,
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshaling aggregate schema: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, AggregateSchemaName), append(data, '\n'), 0o644); err != nil {
		return 0, fmt.Errorf("writing aggregate schema: %w", err)
	}
	return len(entries), nil
}

func readKindsManifest(outputDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(outputDir, metadataDirName, kindsManifestName))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading kinds manifest: %w", err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(data, &kinds); err != nil {
		return nil, fmt.Errorf("parsing kinds manifest: %w", err)
	}
	return kinds, nil
}

// aggregateEntries builds one discriminated branch per schema path, in path
// order so the output is stable across runs.
func aggregateEntries(kinds map[string]string) []interface{} {
	paths := make([]string, 0, len(kinds))
	for relPath := range kinds {
		paths = append(paths, relPath)
	}
	sort.Strings(paths)

	entries := make([]interface{}, 0, len(paths))
	for _, relPath := range paths {
		kind := kinds[relPath]
		apiVersion, ok := schemaPathAPIVersion(relPath)
		if !ok || kind == "" {
			continue
		}
		entries = append(entries, map[string]interface{}{
			"allOf": []interface{}{
				map[string]interface{}{
					"properties": map[string]interface{}{
						"apiVersion": map[string]interface{}{"enum": []interface{}{apiVersion}},
						"kind":       map[string]interface{}{"enum": []interface{}{kind}},
					},
					"required": []interface{}{"apiVersion", "kind"},
				},
				map[string]interface{}{"$ref": relPath},
			},
		})
	}
	return entries
}

// schemaPathAPIVersion derives the manifest apiVersion from a primary-layout
// schema path "<group>/<kind>_<version>.json". The core directory maps back to
// the empty API group, whose apiVersion is the bare version.
func schemaPathAPIVersion(relPath string) (string, bool) {
	dir, file := path.Split(relPath)
	group := strings.TrimSuffix(dir, "/")
	if group == "" || strings.Contains(group, "/") {
		return "", false
	}
	name, isJSON := strings.CutSuffix(file, ".json")
	if !isJSON {
		return "", false
	}
	sep := strings.LastIndex(name, "_")
	if sep <= 0 || sep == len(name)-1 {
		return "", false
	}
	version := name[sep+1:]
	if group == coreGroupDirName {
		return version, true
	}
	return group + "/" + version, true
}
