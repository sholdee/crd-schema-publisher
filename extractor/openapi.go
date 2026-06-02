package extractor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// draft04 is the JSON Schema dialect Kubernetes OpenAPI v2 definitions use.
const draft04 = "http://json-schema.org/draft-04/schema#"

// openAPIDoc is the subset of a Kubernetes OpenAPI v2 (swagger) document we need:
// the flat map of named type definitions.
type openAPIDoc struct {
	Definitions map[string]json.RawMessage `json:"definitions"`
}

type groupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// WriteOpenAPISchemas reads a Kubernetes OpenAPI v2 (swagger) document and writes
// one JSON schema per built-in (group, version, kind), matching WriteSchemas'
// layout. OpenAPI definitions cross-reference via "#/definitions/...", so each
// schema embeds the transitive closure of its referenced definitions locally,
// keeping files self-contained like CRD schemas (and recursion representable).
func WriteOpenAPISchemas(openapiPath, outputDir string, filter SchemaFilter) (int, error) {
	raw, err := os.ReadFile(openapiPath)
	if err != nil {
		return 0, fmt.Errorf("reading openapi spec: %w", err)
	}

	var doc openAPIDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return 0, fmt.Errorf("parsing openapi spec: %w", err)
	}

	defs := make(map[string]map[string]interface{}, len(doc.Definitions))
	for name, rawDef := range doc.Definitions {
		var def map[string]interface{}
		if err := json.Unmarshal(rawDef, &def); err != nil {
			return 0, fmt.Errorf("parsing definition %s: %w", name, err)
		}
		defs[name] = def
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		sem      = make(chan struct{}, 10)
		count    int
		firstErr error
		kinds    = make(map[string]string)
	)

	for name, def := range defs {
		// Authorable kinds declare a GVK and apiVersion/kind properties; this
		// skips internal types like WatchEvent.
		gvks := groupVersionKinds(def)
		if len(gvks) == 0 || !hasManifestProperties(def) {
			continue
		}

		closure := definitionClosure(name, defs)
		for _, gvk := range gvks {
			group := gvk.Group
			if group == "" {
				group = "core"
			}
			if !filter.Matches(gvk.Kind, group, gvk.Version) {
				continue
			}
			schema := standaloneSchema(def, defs, closure)

			wg.Add(1)
			sem <- struct{}{}
			go func(schema map[string]interface{}, kind, group, version, originalKind string) {
				defer wg.Done()
				defer func() { <-sem }()

				filename, err := writeSchemaMap(schema, kind, group, version, outputDir)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}

				mu.Lock()
				kinds[filepath.ToSlash(filepath.Join(group, filename))] = originalKind
				count++
				mu.Unlock()
			}(schema, strings.ToLower(gvk.Kind), group, gvk.Version, gvk.Kind)
		}
	}

	wg.Wait()
	if firstErr != nil {
		return count, firstErr
	}
	if len(kinds) > 0 {
		if err := writeKindsManifest(outputDir, kinds); err != nil {
			return count, err
		}
	}
	return count, nil
}

// groupVersionKinds reads the x-kubernetes-group-version-kind extension. A type
// may register several (e.g. Event in core and events.k8s.io).
func groupVersionKinds(def map[string]interface{}) []groupVersionKind {
	raw, ok := def["x-kubernetes-group-version-kind"].([]interface{})
	if !ok {
		return nil
	}
	var gvks []groupVersionKind
	for _, entry := range raw {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		group, _ := m["group"].(string)
		version, _ := m["version"].(string)
		kind, _ := m["kind"].(string)
		if version != "" && kind != "" {
			gvks = append(gvks, groupVersionKind{Group: group, Version: version, Kind: kind})
		}
	}
	return gvks
}

func hasManifestProperties(def map[string]interface{}) bool {
	props, ok := def["properties"].(map[string]interface{})
	if !ok {
		return false
	}
	_, hasAPIVersion := props["apiVersion"]
	_, hasKind := props["kind"]
	return hasAPIVersion && hasKind
}

// definitionClosure returns every definition transitively reachable from root
// via "#/definitions/..." references.
func definitionClosure(root string, defs map[string]map[string]interface{}) map[string]bool {
	closure := make(map[string]bool)
	queue := collectRefs(defs[root])
	for len(queue) > 0 {
		name := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if closure[name] {
			continue
		}
		closure[name] = true
		queue = append(queue, collectRefs(defs[name])...)
	}
	return closure
}

// collectRefs walks a decoded schema and returns the names of every
// "#/definitions/<name>" reference it contains.
func collectRefs(node interface{}) []string {
	var refs []string
	switch n := node.(type) {
	case map[string]interface{}:
		for key, value := range n {
			if key == "$ref" {
				if ref, ok := value.(string); ok {
					if name, found := strings.CutPrefix(ref, "#/definitions/"); found {
						refs = append(refs, name)
					}
				}
				continue
			}
			refs = append(refs, collectRefs(value)...)
		}
	case []interface{}:
		for _, value := range n {
			refs = append(refs, collectRefs(value)...)
		}
	}
	return refs
}

// standaloneSchema returns a self-contained draft-04 copy of def with its
// referenced definitions embedded, so internal $refs resolve within the file.
func standaloneSchema(def map[string]interface{}, defs map[string]map[string]interface{}, closure map[string]bool) map[string]interface{} {
	schema := deepCopy(def)
	if len(closure) > 0 {
		bundle := make(map[string]interface{}, len(closure))
		for name := range closure {
			bundle[name] = deepCopy(defs[name])
		}
		schema["definitions"] = bundle
	}
	schema["$schema"] = draft04
	return schema
}

func deepCopy(m map[string]interface{}) map[string]interface{} {
	raw, _ := json.Marshal(m)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return out
}
