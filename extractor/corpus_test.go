package extractor

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	corpusDir           = "testdata/corpus"
	corpusExpectComment = "# expect:"
)

// TestCorpusValidation converts the CRDs and trimmed OpenAPI document under
// testdata/corpus with the real extractor and converter code, then validates
// each manifest against the generated schema using the same validator engine
// kubeconform uses. Valid manifests are ones the API server accepts and must
// pass; invalid ones must fail and mention the text in their leading
// "# expect:" comment. This catches schemas that drift stricter or looser than
// Kubernetes, which unit tests on individual transforms cannot see.
func TestCorpusValidation(t *testing.T) {
	outputDir := t.TempDir()
	buildCorpusSchemas(t, outputDir)

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft4)
	schemas := make(map[string]*jsonschema.Schema)
	loadSchema := func(t *testing.T, rel string) *jsonschema.Schema {
		t.Helper()
		if schema, ok := schemas[rel]; ok {
			return schema
		}
		path := filepath.Join(outputDir, rel)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("schema %s was not generated: %v", rel, err)
		}
		schema, err := compiler.Compile(path)
		if err != nil {
			t.Fatalf("compiling %s: %v", rel, err)
		}
		schemas[rel] = schema
		return schema
	}

	t.Run("valid", func(t *testing.T) {
		for _, file := range corpusFiles(t, "valid") {
			t.Run(filepath.Base(file), func(t *testing.T) {
				for i, doc := range corpusDocuments(t, file) {
					schema := loadSchema(t, doc.schemaPath)
					if err := schema.Validate(doc.instance); err != nil {
						t.Errorf("document %d (%s) should validate against %s:\n%v", i, doc.id, doc.schemaPath, err)
					}
				}
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		for _, file := range corpusFiles(t, "invalid") {
			t.Run(filepath.Base(file), func(t *testing.T) {
				expect := corpusExpectation(t, file)
				for i, doc := range corpusDocuments(t, file) {
					schema := loadSchema(t, doc.schemaPath)
					err := schema.Validate(doc.instance)
					if err == nil {
						t.Errorf("document %d (%s) should fail validation against %s", i, doc.id, doc.schemaPath)
						continue
					}
					if expect != "" && !strings.Contains(err.Error(), expect) {
						t.Errorf("document %d (%s) failed for a different reason\nwant substring: %q\ngot:\n%v", i, doc.id, expect, err)
					}
				}
			})
		}
	})
}

// buildCorpusSchemas writes CRD and built-in schemas into outputDir the same
// way convert --dir ... --openapi ... does.
func buildCorpusSchemas(t *testing.T, outputDir string) {
	t.Helper()

	crds, err := ParseCRDsFromDir(filepath.Join(corpusDir, "crds"))
	if err != nil {
		t.Fatalf("parsing corpus CRDs: %v", err)
	}
	if len(crds) == 0 {
		t.Fatal("corpus has no CRDs")
	}
	if _, err := WriteSchemas(crds, outputDir); err != nil {
		t.Fatalf("writing CRD schemas: %v", err)
	}

	openapi, err := os.ReadFile(filepath.Join(corpusDir, "openapi.json"))
	if err != nil {
		t.Fatalf("reading corpus openapi document: %v", err)
	}
	count, err := WriteOpenAPISchemasFromBytesExcludingCRDs(openapi, outputDir, SchemaFilter{}, crds)
	if err != nil {
		t.Fatalf("writing built-in schemas: %v", err)
	}
	if count == 0 {
		t.Fatal("corpus openapi document produced no schemas")
	}
}

type corpusDocument struct {
	id         string
	schemaPath string
	instance   interface{}
}

func corpusFiles(t *testing.T, subdir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(corpusDir, subdir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no corpus files under %s/%s", corpusDir, subdir)
	}
	sort.Strings(files)
	return files
}

// corpusDocuments splits a multi-document YAML file and decodes each document
// into a JSON instance plus the relative path of the schema it targets, using
// the same group normalization as the generated output layout.
func corpusDocuments(t *testing.T, path string) []corpusDocument {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var docs []corpusDocument
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(raw)))
	for {
		chunk, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if len(bytes.TrimSpace(chunk)) == 0 {
			continue
		}
		jsonBytes, err := sigsyaml.YAMLToJSON(chunk)
		if err != nil {
			t.Fatalf("converting %s to JSON: %v", path, err)
		}
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(jsonBytes))
		if err != nil {
			t.Fatalf("decoding %s: %v", path, err)
		}
		obj, ok := instance.(map[string]interface{})
		if !ok {
			t.Fatalf("%s: document is not an object", path)
		}
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		if apiVersion == "" || kind == "" {
			t.Fatalf("%s: document is missing apiVersion or kind", path)
		}
		group, version := "core", apiVersion
		if g, v, found := strings.Cut(apiVersion, "/"); found {
			group, version = g, v
		}
		docs = append(docs, corpusDocument{
			id:         fmt.Sprintf("%s/%s", apiVersion, kind),
			schemaPath: filepath.Join(group, fmt.Sprintf("%s_%s.json", strings.ToLower(kind), version)),
			instance:   instance,
		})
	}
	if len(docs) == 0 {
		t.Fatalf("%s: no YAML documents found", path)
	}
	return docs
}

// corpusExpectation returns the text after a leading "# expect:" comment, or
// an empty string when the file does not pin the failure reason.
func corpusExpectation(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if rest, found := strings.CutPrefix(line, corpusExpectComment); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}
