// Command trim-openapi extracts a self-contained subset of a Kubernetes OpenAPI
// v2 (swagger) document: the named root definitions plus everything they
// reference through "#/definitions/..." refs. It exists to regenerate the
// built-in schema corpus fixture under extractor/testdata/corpus without
// committing a multi-megabyte swagger.json.
//
// Usage:
//
//	kubectl get --raw /openapi/v2 > swagger.json
//	go run ./hack/trim-openapi -in swagger.json \
//	  -out extractor/testdata/corpus/openapi.json \
//	  -keep io.k8s.api.core.v1.Pod,io.k8s.api.apps.v1.Deployment
//
// Descriptions are dropped by default to keep the fixture small; validation
// semantics do not depend on them.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

const refPrefix = "#/definitions/"

func main() {
	in := flag.String("in", "", "path to the full OpenAPI v2 swagger.json")
	out := flag.String("out", "", "path to write the trimmed document")
	keep := flag.String("keep", "", "comma-separated root definition names to keep")
	keepDescriptions := flag.Bool("keep-descriptions", false, "retain description fields")
	flag.Parse()

	if *in == "" || *out == "" || *keep == "" {
		fmt.Fprintln(os.Stderr, "trim-openapi: -in, -out, and -keep are required")
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*in, *out, strings.Split(*keep, ","), !*keepDescriptions); err != nil {
		fmt.Fprintln(os.Stderr, "trim-openapi:", err)
		os.Exit(1)
	}
}

func run(inPath, outPath string, roots []string, stripDescriptions bool) error {
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	var doc struct {
		Swagger     string                     `json:"swagger"`
		Definitions map[string]json.RawMessage `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", inPath, err)
	}

	defs := make(map[string]interface{}, len(doc.Definitions))
	for name, rawDef := range doc.Definitions {
		var def interface{}
		if err := json.Unmarshal(rawDef, &def); err != nil {
			return fmt.Errorf("parsing definition %s: %w", name, err)
		}
		defs[name] = def
	}

	keep := make(map[string]interface{})
	queue := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if _, ok := defs[root]; !ok {
			return fmt.Errorf("definition %q not found in %s", root, inPath)
		}
		queue = append(queue, root)
	}
	for len(queue) > 0 {
		name := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if _, done := keep[name]; done {
			continue
		}
		def := defs[name]
		if stripDescriptions {
			def = withoutDescriptions(def)
		}
		keep[name] = def
		queue = append(queue, collectRefs(def)...)
	}

	names := make([]string, 0, len(keep))
	for name := range keep {
		names = append(names, name)
	}
	sort.Strings(names)

	output := map[string]interface{}{
		"swagger":     doc.Swagger,
		"definitions": keep,
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %d definitions to %s\n", len(names), outPath)
	return nil
}

// collectRefs returns every "#/definitions/<name>" reference under node.
func collectRefs(node interface{}) []string {
	var refs []string
	switch n := node.(type) {
	case map[string]interface{}:
		for key, value := range n {
			if key == "$ref" {
				if ref, ok := value.(string); ok {
					if name, found := strings.CutPrefix(ref, refPrefix); found {
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

// withoutDescriptions returns a copy of node with every "description" key
// removed from schema maps. Property names are never touched: a property
// literally named "description" lives under a "properties" map, and only the
// map that *is* a schema loses its description keyword.
func withoutDescriptions(node interface{}) interface{} {
	switch n := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(n))
		for key, value := range n {
			if key == "description" {
				if _, isString := value.(string); isString {
					continue
				}
			}
			if key == "properties" {
				if props, ok := value.(map[string]interface{}); ok {
					trimmed := make(map[string]interface{}, len(props))
					for propName, propSchema := range props {
						trimmed[propName] = withoutDescriptions(propSchema)
					}
					out[key] = trimmed
					continue
				}
			}
			out[key] = withoutDescriptions(value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(n))
		for i, value := range n {
			out[i] = withoutDescriptions(value)
		}
		return out
	default:
		return node
	}
}
