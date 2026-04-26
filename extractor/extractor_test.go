package extractor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeLister struct {
	crds []apiextensionsv1.CustomResourceDefinition
	err  error
}

func (f *fakeLister) List(_ context.Context, _ metav1.ListOptions) (*apiextensionsv1.CustomResourceDefinitionList, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &apiextensionsv1.CustomResourceDefinitionList{Items: f.crds}, nil
}

func fakeCRD() apiextensionsv1.CustomResourceDefinition {
	schema := &apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"spec": {
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"replicas": {Type: "integer"},
				},
			},
		},
	}
	return apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "tests.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Test"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: "v1",
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: schema,
					},
				},
			},
		},
	}
}

func TestWriteSchemas_CreatesGroupDirAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	crds := []apiextensionsv1.CustomResourceDefinition{fakeCRD()}
	count, err := WriteSchemas(crds, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 schema, got %d", count)
	}
	schemaPath := filepath.Join(tmpDir, "example.io", "test_v1.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("schema file not found: %v", err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// After conversion, type may be "object" or ["object", "null"]
	switch v := schema["type"].(type) {
	case string:
		if v != "object" {
			t.Fatalf("expected type=object, got %v", v)
		}
	case []interface{}:
		found := false
		for _, item := range v {
			if item == "object" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected type to contain 'object', got %v", v)
		}
	default:
		t.Fatalf("unexpected type field: %T %v", schema["type"], schema["type"])
	}

	kindManifest := filepath.Join(tmpDir, "_meta", "kinds.json")
	data, err = os.ReadFile(kindManifest)
	if err != nil {
		t.Fatalf("kind manifest not found: %v", err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(data, &kinds); err != nil {
		t.Fatalf("invalid kind manifest JSON: %v", err)
	}
	if got := kinds["example.io/test_v1.json"]; got != "Test" {
		t.Fatalf("expected kind manifest to contain original Kind, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "example.io", "test_v1.kind")); !os.IsNotExist(err) {
		t.Fatalf("expected per-schema .kind sidecar to be removed, got err=%v", err)
	}
}

func TestWriteSchemas_CreatesMasterStandalone(t *testing.T) {
	tmpDir := t.TempDir()
	crds := []apiextensionsv1.CustomResourceDefinition{fakeCRD()}
	_, _ = WriteSchemas(crds, tmpDir)
	standalonePath := filepath.Join(tmpDir, "master-standalone", "example.io-test-stable-v1.json")
	if _, err := os.Stat(standalonePath); os.IsNotExist(err) {
		t.Fatalf("master-standalone file not found: %s", standalonePath)
	}
}

// --- ListCRDs tests ---

func TestListCRDs_SingleCRD(t *testing.T) {
	lister := &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD()}}
	crds, err := ListCRDs(lister)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 1 {
		t.Fatalf("expected 1 CRD, got %d", len(crds))
	}
}

func TestListCRDs_MultipleCRDs(t *testing.T) {
	lister := &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{fakeCRD(), fakeCRD(), fakeCRD()}}
	crds, err := ListCRDs(lister)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 3 {
		t.Fatalf("expected 3 CRDs, got %d", len(crds))
	}
}

func TestListCRDs_EmptyList(t *testing.T) {
	lister := &fakeLister{crds: []apiextensionsv1.CustomResourceDefinition{}}
	crds, err := ListCRDs(lister)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(crds) != 0 {
		t.Fatalf("expected 0 CRDs, got %d", len(crds))
	}
}

func TestListCRDs_APIError(t *testing.T) {
	lister := &fakeLister{err: errors.New("api server unavailable")}
	_, err := ListCRDs(lister)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "listing CRDs") {
		t.Fatalf("expected wrapped error containing 'listing CRDs', got: %v", err)
	}
	if !errors.Is(err, lister.err) {
		t.Fatalf("expected wrapped original error, got: %v", err)
	}
}

// --- WriteSchemas additional tests ---

func TestWriteSchemas_MultipleVersions(t *testing.T) {
	schema := &apiextensionsv1.JSONSchemaProps{Type: "object"}
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Widget"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: schema}},
				{Name: "v2", Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: schema}},
			},
		},
	}
	tmpDir := t.TempDir()
	count, err := WriteSchemas([]apiextensionsv1.CustomResourceDefinition{crd}, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 schemas, got %d", count)
	}
	for _, v := range []string{"v1", "v2"} {
		groupPath := filepath.Join(tmpDir, "example.io", "widget_"+v+".json")
		if _, err := os.Stat(groupPath); os.IsNotExist(err) {
			t.Errorf("missing group file: %s", groupPath)
		}
		standalonePath := filepath.Join(tmpDir, "master-standalone", "example.io-widget-stable-"+v+".json")
		if _, err := os.Stat(standalonePath); os.IsNotExist(err) {
			t.Errorf("missing standalone file: %s", standalonePath)
		}
	}
}

func TestWriteSchemas_NoSchema(t *testing.T) {
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Legacy"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1beta1", Schema: nil},
			},
		},
	}
	tmpDir := t.TempDir()
	count, err := WriteSchemas([]apiextensionsv1.CustomResourceDefinition{crd}, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 schemas for CRD with nil schema, got %d", count)
	}
}

func TestWriteSchemas_EmptyList(t *testing.T) {
	tmpDir := t.TempDir()
	count, err := WriteSchemas([]apiextensionsv1.CustomResourceDefinition{}, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 schemas, got %d", count)
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("reading temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files created, got %d entries", len(entries))
	}
}

func TestWriteSchemas_IntOrString(t *testing.T) {
	schema := &apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"port": {
				Format: "int-or-string",
			},
		},
	}
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "services.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Service"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: schema}},
			},
		},
	}
	tmpDir := t.TempDir()
	_, err := WriteSchemas([]apiextensionsv1.CustomResourceDefinition{crd}, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "example.io", "service_v1.json"))
	if err != nil {
		t.Fatalf("reading schema file: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshaling schema: %v", err)
	}
	props, ok := result["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map in output")
	}
	port, ok := props["port"].(map[string]interface{})
	if !ok {
		t.Fatal("expected port property in output")
	}
	assertIntOrStringPortSchema(t, port)
}

func assertIntOrStringPortSchema(t *testing.T, port map[string]interface{}) {
	t.Helper()
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array in port, got: %v", port)
	}
	if len(oneOf) != 3 {
		t.Fatalf("expected 3 oneOf entries, got %d", len(oneOf))
	}
	// Verify the three types are string, integer, and null. The property is
	// optional, so nullable is represented as a oneOf branch instead of a
	// conflicting sibling type.
	types := make(map[string]bool)
	for _, item := range oneOf {
		m, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map in oneOf, got %T", item)
		}
		if tp, ok := m["type"].(string); ok {
			types[tp] = true
		}
	}
	if !types["string"] || !types["integer"] || !types["null"] {
		t.Fatalf("expected oneOf to contain string, integer, and null types, got: %v", oneOf)
	}
	// Verify format was removed
	if _, hasFormat := port["format"]; hasFormat {
		t.Fatal("expected format to be removed from port field")
	}
	if _, hasType := port["type"]; hasType {
		t.Fatalf("expected sibling type to be removed from int-or-string field, got %v", port["type"])
	}
}

// goldenCRD returns a realistic CRD that exercises all three converter transforms:
// - additionalProperties (nested objects get additionalProperties: false)
// - replaceIntOrString (format: "int-or-string" → oneOf string/integer)
// - allowNullOptionalFields (optional fields become nullable)
// The schema also includes required fields, arrays, nested objects, and enums
// to catch subtle interactions between transforms.
func goldenCRD() apiextensionsv1.CustomResourceDefinition {
	return apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "cert-manager.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Certificate"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1",
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type:     "object",
						Required: []string{"spec"},
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {
								Type:     "object",
								Required: []string{"secretName", "issuerRef"},
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"secretName": {Type: "string"},
									"duration":   {Type: "string"},
									"renewBefore": {
										Type:   "string",
										Format: "int-or-string",
									},
									"dnsNames": {
										Type: "array",
										Items: &apiextensionsv1.JSONSchemaPropsOrArray{
											Schema: &apiextensionsv1.JSONSchemaProps{Type: "string"},
										},
									},
									"issuerRef": {
										Type:     "object",
										Required: []string{"name", "kind"},
										Properties: map[string]apiextensionsv1.JSONSchemaProps{
											"name":  {Type: "string"},
											"kind":  {Type: "string"},
											"group": {Type: "string"},
										},
									},
									"usages": {
										Type: "array",
										Items: &apiextensionsv1.JSONSchemaPropsOrArray{
											Schema: &apiextensionsv1.JSONSchemaProps{
												Type: "string",
												Enum: []apiextensionsv1.JSON{
													{Raw: []byte(`"signing"`)},
													{Raw: []byte(`"digital signature"`)},
													{Raw: []byte(`"server auth"`)},
													{Raw: []byte(`"client auth"`)},
												},
											},
										},
									},
								},
							},
							"status": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"ready": {Type: "boolean"},
									"conditions": {
										Type: "array",
										Items: &apiextensionsv1.JSONSchemaPropsOrArray{
											Schema: &apiextensionsv1.JSONSchemaProps{
												Type: "object",
												Properties: map[string]apiextensionsv1.JSONSchemaProps{
													"type":   {Type: "string"},
													"status": {Type: "string"},
													"reason": {Type: "string"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}},
		},
	}
}

const goldenFixturePath = "testdata/golden_certificate_v1.json"

func TestWriteSchemas_GoldenE2E(t *testing.T) {
	tmpDir := t.TempDir()
	crds := []apiextensionsv1.CustomResourceDefinition{goldenCRD()}

	count, err := WriteSchemas(crds, tmpDir)
	if err != nil {
		t.Fatalf("WriteSchemas error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 schema, got %d", count)
	}

	// Read actual output
	actualPath := filepath.Join(tmpDir, "cert-manager.io", "certificate_v1.json")
	actual, err := os.ReadFile(actualPath)
	if err != nil {
		t.Fatalf("reading output schema: %v", err)
	}

	// Normalize: re-marshal to consistent formatting
	var actualObj interface{}
	if err := json.Unmarshal(actual, &actualObj); err != nil {
		t.Fatalf("unmarshaling actual: %v", err)
	}
	actualNorm, _ := json.MarshalIndent(actualObj, "", "  ")

	// Read or generate golden fixture
	golden, err := os.ReadFile(goldenFixturePath)
	if err != nil {
		// First run: generate the fixture
		t.Logf("Golden fixture not found, generating: %s", goldenFixturePath)
		if err := os.WriteFile(goldenFixturePath, append(actualNorm, '\n'), 0o644); err != nil {
			t.Fatalf("writing golden fixture: %v", err)
		}
		t.Fatalf("Golden fixture generated at %s — verify it manually, then re-run", goldenFixturePath)
	}

	// Normalize golden too
	var goldenObj interface{}
	if err := json.Unmarshal(golden, &goldenObj); err != nil {
		t.Fatalf("unmarshaling golden fixture: %v", err)
	}
	goldenNorm, _ := json.MarshalIndent(goldenObj, "", "  ")

	if string(actualNorm) != string(goldenNorm) {
		t.Errorf("schema output does not match golden fixture %s\n\nTo update after an intentional change, delete the fixture and re-run.\n\ngot:\n%s\n\nwant:\n%s",
			goldenFixturePath, string(actualNorm), string(goldenNorm))
	}

	// Also verify master-standalone output exists and matches
	standalonePath := filepath.Join(tmpDir, "master-standalone", "cert-manager.io-certificate-stable-v1.json")
	standalone, err := os.ReadFile(standalonePath)
	if err != nil {
		t.Fatalf("master-standalone file not found: %v", err)
	}
	if string(actual) != string(standalone) {
		t.Error("master-standalone output differs from primary output — both should be identical")
	}
}

func boolPtr(b bool) *bool { return &b }

func edgeCaseCRD() apiextensionsv1.CustomResourceDefinition {
	return apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "virtualservices.testing.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "testing.example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "VirtualService"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1",
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type:     "object",
						Required: []string{"spec"},
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec": {
								Type:     "object",
								Required: []string{"output"},
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"output": {
										Type:     "object",
										Required: []string{"properties"},
										Properties: map[string]apiextensionsv1.JSONSchemaProps{
											"properties": {
												Type:        "object",
												Description: "Output properties map",
												AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
													Schema: &apiextensionsv1.JSONSchemaProps{
														Type:     "object",
														Required: []string{"type"},
														Properties: map[string]apiextensionsv1.JSONSchemaProps{
															"type":        {Type: "string"},
															"value":       {Type: "string"},
															"description": {Type: "string"},
															"default": {
																// Only XPreserveUnknownFields — minimal schema passes through untouched
																XPreserveUnknownFields: boolPtr(true),
															},
															"properties": {
																Type:                   "object",
																Description:            "Nested properties for object types",
																XPreserveUnknownFields: boolPtr(true),
															},
														},
													},
												},
											},
											"required": {
												Type: "array",
												Items: &apiextensionsv1.JSONSchemaPropsOrArray{
													Schema: &apiextensionsv1.JSONSchemaProps{Type: "string"},
												},
											},
										},
									},
									"match": {
										Type: "object",
										Properties: map[string]apiextensionsv1.JSONSchemaProps{
											"type":     {Type: "string", Description: "Match type"},
											"items":    {Type: "array", Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &apiextensionsv1.JSONSchemaProps{Type: "string"}}},
											"required": {Type: "boolean", Description: "Whether match is required"},
										},
									},
									"options": {
										Type: "object",
										Properties: map[string]apiextensionsv1.JSONSchemaProps{
											"timeout": {Type: "string"},
										},
										AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{Allows: true},
									},
								},
							},
						},
					},
				},
			}},
		},
	}
}

const goldenEdgeCaseFixturePath = "testdata/golden_edgecase_v1.json"

func TestWriteSchemas_GoldenEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	crds := []apiextensionsv1.CustomResourceDefinition{edgeCaseCRD()}

	count, err := WriteSchemas(crds, tmpDir)
	if err != nil {
		t.Fatalf("WriteSchemas error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 schema, got %d", count)
	}

	// Read actual output
	actualPath := filepath.Join(tmpDir, "testing.example.io", "virtualservice_v1.json")
	actual, err := os.ReadFile(actualPath)
	if err != nil {
		t.Fatalf("reading output schema: %v", err)
	}

	// Normalize: re-marshal to consistent formatting
	var actualObj interface{}
	if err := json.Unmarshal(actual, &actualObj); err != nil {
		t.Fatalf("unmarshaling actual: %v", err)
	}
	actualNorm, _ := json.MarshalIndent(actualObj, "", "  ")

	// Read or generate golden fixture
	golden, err := os.ReadFile(goldenEdgeCaseFixturePath)
	if err != nil {
		// First run: generate the fixture
		t.Logf("Golden fixture not found, generating: %s", goldenEdgeCaseFixturePath)
		if err := os.WriteFile(goldenEdgeCaseFixturePath, append(actualNorm, '\n'), 0o644); err != nil {
			t.Fatalf("writing golden fixture: %v", err)
		}
		t.Fatalf("Golden fixture generated at %s — verify it manually, then re-run", goldenEdgeCaseFixturePath)
	}

	// Normalize golden too
	var goldenObj interface{}
	if err := json.Unmarshal(golden, &goldenObj); err != nil {
		t.Fatalf("unmarshaling golden fixture: %v", err)
	}
	goldenNorm, _ := json.MarshalIndent(goldenObj, "", "  ")

	if string(actualNorm) != string(goldenNorm) {
		t.Errorf("schema output does not match golden fixture %s\n\nTo update after an intentional change, delete the fixture and re-run.\n\ngot:\n%s\n\nwant:\n%s",
			goldenEdgeCaseFixturePath, string(actualNorm), string(goldenNorm))
	}

	// Also verify master-standalone output exists and matches
	standalonePath := filepath.Join(tmpDir, "master-standalone", "testing.example.io-virtualservice-stable-v1.json")
	standalone, err := os.ReadFile(standalonePath)
	if err != nil {
		t.Fatalf("master-standalone file not found: %v", err)
	}
	if string(actual) != string(standalone) {
		t.Error("master-standalone output differs from primary output — both should be identical")
	}
}

func TestWriteSchemas_NullableOptionalFields(t *testing.T) {
	schema := &apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"name": {Type: "string"},
		},
	}
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "items.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Item"},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: schema}},
			},
		},
	}
	tmpDir := t.TempDir()
	_, err := WriteSchemas([]apiextensionsv1.CustomResourceDefinition{crd}, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "example.io", "item_v1.json"))
	if err != nil {
		t.Fatalf("reading schema file: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshaling schema: %v", err)
	}
	props, ok := result["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map in output")
	}
	name, ok := props["name"].(map[string]interface{})
	if !ok {
		t.Fatal("expected name property in output")
	}
	typeVal, ok := name["type"].([]interface{})
	if !ok {
		t.Fatalf("expected type to be an array for nullable field, got: %T %v", name["type"], name["type"])
	}
	if len(typeVal) != 2 {
		t.Fatalf("expected 2 type entries, got %d", len(typeVal))
	}
	types := make(map[string]bool)
	for _, v := range typeVal {
		if s, ok := v.(string); ok {
			types[s] = true
		}
	}
	if !types["string"] || !types["null"] {
		t.Fatalf("expected type array to contain 'string' and 'null', got: %v", typeVal)
	}
}
