package converter

import (
	"reflect"
	"testing"
)

// propField extracts result["properties"][field] with checked type assertions.
func propField(t *testing.T, result map[string]interface{}, field string) map[string]interface{} {
	t.Helper()
	props, ok := result["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected properties to be map, got %T", result["properties"])
	}
	val, ok := props[field].(map[string]interface{})
	if !ok {
		t.Fatalf("expected %s to be map, got %T", field, props[field])
	}
	return val
}

func assertOneOfHasType(t *testing.T, schema map[string]interface{}, expected string) {
	t.Helper()
	oneOf, ok := schema["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", schema["oneOf"])
	}
	for _, item := range oneOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected oneOf branch to be map, got %T", item)
		}
		if branch["type"] == expected {
			return
		}
	}
	t.Fatalf("expected oneOf to contain %q branch, got %v", expected, oneOf)
}

func assertAllOfOneOfAllowsNullBypass(t *testing.T, schema map[string]interface{}) {
	t.Helper()
	allOf, ok := schema["allOf"].([]interface{})
	if !ok || len(allOf) != 1 {
		t.Fatalf("expected preserved oneOf wrapper in allOf, got %T %v", schema["allOf"], schema["allOf"])
	}
	wrapper, ok := allOf[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected preserved wrapper to be map, got %T", allOf[0])
	}
	anyOf, ok := wrapper["anyOf"].([]interface{})
	if !ok || len(anyOf) != 2 {
		t.Fatalf("expected preserved oneOf to be wrapped in nullable anyOf, got %T %v", wrapper["anyOf"], wrapper["anyOf"])
	}

	hasOriginal := false
	hasNull := false
	for _, item := range anyOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected nullable anyOf branch to be map, got %T", item)
		}
		if _, ok := branch["oneOf"].([]interface{}); ok {
			hasOriginal = true
		}
		if branch["type"] == "null" {
			hasNull = true
		}
	}
	if !hasOriginal || !hasNull {
		t.Fatalf("expected nullable anyOf to contain original oneOf and null branch, got %v", anyOf)
	}
}

func TestAdditionalProperties_AddsToObjectWithProperties(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
		},
	}
	result := AdditionalProperties(schema, false)
	val, ok := result["additionalProperties"]
	if !ok {
		t.Fatal("expected additionalProperties to be set")
	}
	if val != false {
		t.Fatalf("expected additionalProperties=false, got %v", val)
	}
}

func TestAdditionalProperties_DoesNotOverwriteExisting(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
		},
		"additionalProperties": true,
	}
	result := AdditionalProperties(schema, false)
	if result["additionalProperties"] != true {
		t.Fatal("should not overwrite existing additionalProperties")
	}
}

func TestAdditionalProperties_SkipsObjectWithoutProperties(t *testing.T) {
	schema := map[string]interface{}{
		"type":        "object",
		"description": "no properties field",
	}
	result := AdditionalProperties(schema, false)
	if _, ok := result["additionalProperties"]; ok {
		t.Fatal("should not add additionalProperties when no properties field exists")
	}
}

func TestAdditionalProperties_RecursesIntoNestedObjects(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"spec": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"replicas": map[string]interface{}{"type": "integer"},
				},
			},
		},
	}
	result := AdditionalProperties(schema, false)
	spec := propField(t, result, "spec")
	if spec["additionalProperties"] != false {
		t.Fatal("nested object should also get additionalProperties=false")
	}
}

func TestAdditionalProperties_SkipsRootWhenFlagged(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
		},
	}
	result := AdditionalProperties(schema, true)
	if _, ok := result["additionalProperties"]; ok {
		t.Fatal("should skip root when skip=true")
	}
}

func TestAdditionalProperties_DoesNotCorruptPropertiesMap(t *testing.T) {
	// Regression: when a CRD has a field literally named "properties",
	// the converter would enter the properties map, see the "properties" key,
	// and inject "additionalProperties": false into the map — corrupting it
	// with a boolean entry that isn't a valid schema.
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"output": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"properties": map[string]interface{}{
						"type":        "object",
						"description": "Output properties map",
						"additionalProperties": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"value": map[string]interface{}{"type": "string"},
							},
						},
					},
					"required": map[string]interface{}{
						"type":  "array",
						"items": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}
	result := AdditionalProperties(schema, true)

	// Navigate to output's properties map
	outputSchema := propField(t, result, "output")
	outputProps, ok := outputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected output.properties to be a map")
	}

	// The properties map should only have "properties" and "required" — NOT "additionalProperties"
	if _, found := outputProps["additionalProperties"]; found {
		t.Fatal("additionalProperties was incorrectly injected into the properties map; " +
			"a property named 'properties' caused the converter to treat the map as a schema object")
	}

	// The "properties" field's own additionalProperties (the schema) should still be intact
	propsField, ok := outputProps["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'properties' property to be a map")
	}
	if _, ok := propsField["additionalProperties"].(map[string]interface{}); !ok {
		t.Fatal("the additionalProperties schema on the 'properties' field should be preserved")
	}
}

func TestAdditionalProperties_DoesNotCloseCompositionValidationBranches(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
			"mode": map[string]interface{}{"type": "string"},
		},
		"oneOf": []interface{}{
			map[string]interface{}{
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"pattern": "^prod-"},
				},
			},
		},
		"anyOf": []interface{}{
			map[string]interface{}{
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"pattern": "^stage-"},
				},
			},
		},
		"allOf": []interface{}{
			map[string]interface{}{
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"minLength": 3},
				},
			},
		},
		"not": map[string]interface{}{
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{"pattern": "^legacy$"},
			},
		},
		"dependencies": map[string]interface{}{
			"name": map[string]interface{}{
				"properties": map[string]interface{}{
					"mode": map[string]interface{}{"pattern": "^required-with-name$"},
				},
			},
		},
		"if": map[string]interface{}{
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{"pattern": "^prod$"},
			},
		},
		"then": map[string]interface{}{
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"pattern": "^prod-"},
			},
		},
		"else": map[string]interface{}{
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"pattern": "^nonprod-"},
			},
		},
	}

	result := AdditionalProperties(schema, true)

	for _, keyword := range []string{"oneOf", "anyOf", "allOf"} {
		branches, ok := result[keyword].([]interface{})
		if !ok || len(branches) != 1 {
			t.Fatalf("expected %s branch, got %#v", keyword, result[keyword])
		}
		branch, ok := branches[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected %s branch to be map, got %T", keyword, branches[0])
		}
		if _, found := branch["additionalProperties"]; found {
			t.Fatalf("%s validation branch should not be closed, got %#v", keyword, branch)
		}
	}

	notBranch, ok := result["not"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected not branch to be map, got %T", result["not"])
	}
	if _, found := notBranch["additionalProperties"]; found {
		t.Fatalf("not validation branch should not be closed, got %#v", notBranch)
	}
	dependencies, ok := result["dependencies"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dependencies map, got %T", result["dependencies"])
	}
	dependencyBranch, ok := dependencies["name"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dependency branch to be map, got %T", dependencies["name"])
	}
	if _, found := dependencyBranch["additionalProperties"]; found {
		t.Fatalf("dependency validation branch should not be closed, got %#v", dependencyBranch)
	}
	for _, keyword := range []string{"if", "then", "else"} {
		branch, ok := result[keyword].(map[string]interface{})
		if !ok {
			t.Fatalf("expected %s branch to be map, got %T", keyword, result[keyword])
		}
		if _, found := branch["additionalProperties"]; found {
			t.Fatalf("%s validation branch should not be closed, got %#v", keyword, branch)
		}
	}
}

func TestConvert_DoesNotMutateLiteralDefaultObjects(t *testing.T) {
	defaultValue := func() map[string]interface{} {
		return map[string]interface{}{
			"properties": map[string]interface{}{
				"format": "int-or-string",
				"type":   "string",
			},
			"x-kubernetes-int-or-string": true,
			"nullable":                   true,
		}
	}
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"template": map[string]interface{}{
				"type":    "object",
				"default": defaultValue(),
				"properties": map[string]interface{}{
					"port": map[string]interface{}{
						"format": "int-or-string",
					},
				},
			},
		},
	}

	result := Convert(schema)
	template := propField(t, result, "template")

	if !reflect.DeepEqual(template["default"], defaultValue()) {
		t.Fatalf("literal default value should remain unchanged, got %#v", template["default"])
	}
	port := propField(t, template, "port")
	if _, ok := port["oneOf"]; !ok {
		t.Fatal("real nested schemas should still be transformed")
	}
}

func TestConvert_DoesNotMutateLiteralEnumObjects(t *testing.T) {
	enumValue := func() map[string]interface{} {
		return map[string]interface{}{
			"type":   "object",
			"format": "int-or-string",
			"properties": map[string]interface{}{
				"name": "example",
			},
		}
	}
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mode": map[string]interface{}{
				"type": "object",
				"enum": []interface{}{enumValue()},
			},
		},
	}

	result := Convert(schema)
	mode := propField(t, result, "mode")
	enum, ok := mode["enum"].([]interface{})
	if !ok || len(enum) != 1 {
		t.Fatalf("expected enum value to be preserved, got %#v", mode["enum"])
	}
	if !reflect.DeepEqual(enum[0], enumValue()) {
		t.Fatalf("literal enum value should remain unchanged, got %#v", enum[0])
	}
}

func TestReplaceIntOrString_ReplacesFormat(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"format": "int-or-string",
			},
		},
	}
	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	oneOf, ok := port["oneOf"]
	if !ok {
		t.Fatal("expected oneOf to replace int-or-string format")
	}
	items, ok := oneOf.([]interface{})
	if !ok {
		t.Fatalf("expected oneOf to be slice, got %T", oneOf)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 oneOf items, got %d", len(items))
	}
	if _, hasFormat := port["format"]; hasFormat {
		t.Fatal("format field should be removed")
	}
}

func TestReplaceIntOrString_RemovesConflictingTypeAndPreservesMetadata(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":        "string",
				"format":      "int-or-string",
				"description": "service port",
				"default":     "http",
				"maxLength":   16,
				"minimum":     1,
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")

	if _, hasType := port["type"]; hasType {
		t.Fatalf("conflicting parent type should be removed, got %v", port["type"])
	}
	if port["description"] != "service port" {
		t.Fatalf("description metadata should be preserved, got %v", port["description"])
	}
	if port["default"] != "http" {
		t.Fatalf("default metadata should be preserved, got %v", port["default"])
	}

	oneOf, ok := port["oneOf"].([]interface{})
	if !ok || len(oneOf) != 2 {
		t.Fatalf("expected two oneOf branches, got %T %v", port["oneOf"], port["oneOf"])
	}
	stringBranch, ok := oneOf[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected string branch to be map, got %T", oneOf[0])
	}
	integerBranch, ok := oneOf[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected integer branch to be map, got %T", oneOf[1])
	}
	if stringBranch["type"] != "string" || stringBranch["maxLength"] != 16 {
		t.Fatalf("string branch should keep string constraints, got %v", stringBranch)
	}
	if _, found := stringBranch["minimum"]; found {
		t.Fatalf("string branch should not receive integer constraints, got %v", stringBranch)
	}
	if integerBranch["type"] != "integer" || integerBranch["minimum"] != 1 {
		t.Fatalf("integer branch should keep integer constraints, got %v", integerBranch)
	}
	if _, found := integerBranch["maxLength"]; found {
		t.Fatalf("integer branch should not receive string constraints, got %v", integerBranch)
	}
}

func TestConvert_OptionalIntOrStringAddsNullBranchWithoutParentType(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":   "string",
				"format": "int-or-string",
			},
		},
	}

	result := Convert(schema)
	port := propField(t, result, "port")

	if _, hasType := port["type"]; hasType {
		t.Fatalf("optional int-or-string should not keep sibling type, got %v", port["type"])
	}
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", port["oneOf"])
	}
	types := make(map[string]bool)
	for _, item := range oneOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected oneOf branch to be map, got %T", item)
		}
		if typ, ok := branch["type"].(string); ok {
			types[typ] = true
		}
	}
	if !types["string"] || !types["integer"] || !types["null"] {
		t.Fatalf("expected string, integer, and null branches, got %v", oneOf)
	}
}

func TestConvert_RequiredIntOrStringDoesNotAddNullBranch(t *testing.T) {
	schema := map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"port"},
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":   "string",
				"format": "int-or-string",
			},
		},
	}

	result := Convert(schema)
	port := propField(t, result, "port")
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", port["oneOf"])
	}
	for _, item := range oneOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected oneOf branch to be map, got %T", item)
		}
		if branch["type"] == "null" {
			t.Fatalf("required int-or-string should not include null branch, got %v", oneOf)
		}
	}
}

func TestConvert_NullableIntOrStringAddsNullBranch(t *testing.T) {
	schema := map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"port"},
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":     "string",
				"format":   "int-or-string",
				"nullable": true,
			},
		},
	}

	result := Convert(schema)
	port := propField(t, result, "port")
	if _, hasNullable := port["nullable"]; hasNullable {
		t.Fatalf("OpenAPI nullable should be rewritten, got %v", port["nullable"])
	}
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", port["oneOf"])
	}
	types := make(map[string]bool)
	for _, item := range oneOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected oneOf branch to be map, got %T", item)
		}
		if typ, ok := branch["type"].(string); ok {
			types[typ] = true
		}
	}
	if !types["string"] || !types["integer"] || !types["null"] {
		t.Fatalf("expected string, integer, and null branches, got %v", oneOf)
	}
}

func TestReplaceIntOrString_TypeArrayNullAddsNullBranch(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":   []interface{}{"string", "null"},
				"format": "int-or-string",
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	if _, hasType := port["type"]; hasType {
		t.Fatalf("conflicting parent type should be removed, got %v", port["type"])
	}
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", port["oneOf"])
	}
	types := make(map[string]bool)
	for _, item := range oneOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected oneOf branch to be map, got %T", item)
		}
		if typ, ok := branch["type"].(string); ok {
			types[typ] = true
		}
	}
	if !types["string"] || !types["integer"] || !types["null"] {
		t.Fatalf("expected string, integer, and null branches, got %v", oneOf)
	}
}

func TestReplaceIntOrString_UnfoldsKubernetesExtension(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"type":                       "string",
				"x-kubernetes-int-or-string": true,
				"description":                "target port",
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	if port["x-kubernetes-int-or-string"] != true {
		t.Fatalf("kubernetes extension should be preserved as metadata, got %v", port["x-kubernetes-int-or-string"])
	}
	if port["description"] != "target port" {
		t.Fatalf("description metadata should be preserved, got %v", port["description"])
	}
	if _, hasType := port["type"]; hasType {
		t.Fatalf("conflicting parent type should be removed, got %v", port["type"])
	}
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", port["oneOf"])
	}
	types := make(map[string]bool)
	for _, item := range oneOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected oneOf branch to be map, got %T", item)
		}
		if typ, ok := branch["type"].(string); ok {
			types[typ] = true
		}
	}
	if !types["string"] || !types["integer"] {
		t.Fatalf("expected string and integer branches, got %v", oneOf)
	}
}

func TestReplaceIntOrString_PreservesExistingOneOfBranchConstraints(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"oneOf": []interface{}{
					map[string]interface{}{
						"type":        "string",
						"pattern":     "^[a-z]+$",
						"description": "named port",
					},
					map[string]interface{}{
						"type":    "integer",
						"minimum": 1,
						"maximum": 65535,
					},
					map[string]interface{}{"type": "null"},
				},
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", port["oneOf"])
	}
	if len(oneOf) != 3 {
		t.Fatalf("expected string, integer, and null branches, got %v", oneOf)
	}
	stringBranch, ok := oneOf[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected string branch to be map, got %T", oneOf[0])
	}
	integerBranch, ok := oneOf[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected integer branch to be map, got %T", oneOf[1])
	}
	if stringBranch["pattern"] != "^[a-z]+$" || stringBranch["description"] != "named port" {
		t.Fatalf("string branch constraints and metadata should be preserved, got %v", stringBranch)
	}
	if integerBranch["minimum"] != 1 || integerBranch["maximum"] != 65535 {
		t.Fatalf("integer branch constraints should be preserved, got %v", integerBranch)
	}
}

func TestReplaceIntOrString_CanonicalizesKubernetesAnyOfForm(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"nullable":                   true,
				"anyOf": []interface{}{
					map[string]interface{}{
						"type":    "string",
						"pattern": "^[a-z]+$",
					},
					map[string]interface{}{
						"type":    "integer",
						"minimum": 1,
					},
				},
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	if _, hasAnyOf := port["anyOf"]; hasAnyOf {
		t.Fatalf("int-or-string anyOf marker should be removed after canonicalization, got %v", port["anyOf"])
	}
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok || len(oneOf) != 3 {
		t.Fatalf("expected string, integer, and null oneOf branches, got %T %v", port["oneOf"], port["oneOf"])
	}
	stringBranch, ok := oneOf[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected string branch to be map, got %T", oneOf[0])
	}
	integerBranch, ok := oneOf[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected integer branch to be map, got %T", oneOf[1])
	}
	if stringBranch["pattern"] != "^[a-z]+$" {
		t.Fatalf("string branch should preserve anyOf constraints, got %v", stringBranch)
	}
	if integerBranch["minimum"] != 1 {
		t.Fatalf("integer branch should preserve anyOf constraints, got %v", integerBranch)
	}
}

func TestReplaceIntOrString_CanonicalizesKubernetesAllOfAnyOfForm(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"nullable":                   true,
				"allOf": []interface{}{
					map[string]interface{}{
						"anyOf": []interface{}{
							map[string]interface{}{"type": "integer"},
							map[string]interface{}{
								"type":      "string",
								"maxLength": 15,
							},
						},
					},
					map[string]interface{}{
						"description": "additional allOf metadata",
					},
				},
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	allOf, ok := port["allOf"].([]interface{})
	if !ok || len(allOf) != 1 {
		t.Fatalf("allOf should keep only non-marker entries, got %T %v", port["allOf"], port["allOf"])
	}
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok || len(oneOf) != 3 {
		t.Fatalf("expected string, integer, and null oneOf branches, got %T %v", port["oneOf"], port["oneOf"])
	}
	stringBranch, ok := oneOf[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected string branch to be map, got %T", oneOf[0])
	}
	if stringBranch["maxLength"] != 15 {
		t.Fatalf("string branch should preserve nested anyOf constraints, got %v", stringBranch)
	}
}

func TestReplaceIntOrString_PreservesDuplicateParentAndBranchConstraints(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"pattern":                    "^svc-",
				"oneOf": []interface{}{
					map[string]interface{}{
						"type":    "string",
						"pattern": "^[a-z-]+$",
					},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", port["oneOf"])
	}
	stringBranch, ok := oneOf[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected string branch to be map, got %T", oneOf[0])
	}
	if stringBranch["pattern"] != "^[a-z-]+$" {
		t.Fatalf("branch-local pattern should remain on string branch, got %v", stringBranch)
	}
	allOf, ok := stringBranch["allOf"].([]interface{})
	if !ok || len(allOf) != 1 {
		t.Fatalf("duplicate parent pattern should be preserved through allOf, got %T %v", stringBranch["allOf"], stringBranch["allOf"])
	}
	parentPattern, ok := allOf[0].(map[string]interface{})
	if !ok || parentPattern["pattern"] != "^svc-" {
		t.Fatalf("expected parent pattern in string branch allOf, got %v", allOf[0])
	}
}

func TestReplaceIntOrString_PreservesIncompatibleExistingOneOf(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"oneOf": []interface{}{
					map[string]interface{}{
						"type":    "string",
						"pattern": "^http-",
					},
					map[string]interface{}{
						"type":    "string",
						"pattern": "^grpc-",
					},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	oneOf, ok := port["oneOf"].([]interface{})
	if !ok || len(oneOf) != 2 {
		t.Fatalf("expected canonical string/integer oneOf, got %T %v", port["oneOf"], port["oneOf"])
	}
	allOf, ok := port["allOf"].([]interface{})
	if !ok || len(allOf) != 1 {
		t.Fatalf("incompatible original oneOf should be preserved through allOf, got %T %v", port["allOf"], port["allOf"])
	}
	originalWrapper, ok := allOf[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected preserved oneOf wrapper to be map, got %T", allOf[0])
	}
	originalOneOf, ok := originalWrapper["oneOf"].([]interface{})
	if !ok || len(originalOneOf) != 3 {
		t.Fatalf("expected original oneOf branches to be preserved, got %T %v", originalWrapper["oneOf"], originalWrapper["oneOf"])
	}
}

func TestReplaceIntOrString_NullableIncompatibleOneOfAllowsNullThroughPreservedConstraint(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"nullable":                   true,
				"oneOf": []interface{}{
					map[string]interface{}{"type": "string", "pattern": "^http-"},
					map[string]interface{}{"type": "string", "pattern": "^grpc-"},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}

	result := ReplaceIntOrString(schema)
	port := propField(t, result, "port")
	assertAllOfOneOfAllowsNullBypass(t, port)
}

func TestConvert_OptionalIncompatibleOneOfAllowsNullThroughPreservedConstraint(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"oneOf": []interface{}{
					map[string]interface{}{"type": "string", "pattern": "^http-"},
					map[string]interface{}{"type": "string", "pattern": "^grpc-"},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}

	result := Convert(schema)
	port := propField(t, result, "port")
	assertOneOfHasType(t, port, "null")
	assertAllOfOneOfAllowsNullBypass(t, port)
}

func TestConvert_OptionalIncompatibleOneOfWithTypelessBranchAllowsNullThroughPreservedConstraint(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"x-kubernetes-int-or-string": true,
				"oneOf": []interface{}{
					map[string]interface{}{"pattern": "^http-"},
					map[string]interface{}{"type": "string", "pattern": "^grpc-"},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}

	result := Convert(schema)
	port := propField(t, result, "port")
	assertOneOfHasType(t, port, "null")
	assertAllOfOneOfAllowsNullBypass(t, port)
}

func TestAllowNullOptionalFields_ExpandsSiblingTypeWhenAppendingNullOneOf(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"choice": map[string]interface{}{
				"type": "string",
				"oneOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "integer"},
				},
			},
		},
	}

	result := AllowNullOptionalFields(schema, "", nil)
	choice := propField(t, result, "choice")
	typeVal, ok := choice["type"].([]interface{})
	if !ok {
		t.Fatalf("expected sibling type to include null, got %T %v", choice["type"], choice["type"])
	}
	if len(typeVal) != 2 || typeVal[0] != "string" || typeVal[1] != "null" {
		t.Fatalf("expected sibling type [string, null], got %v", typeVal)
	}
	oneOf, ok := choice["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", choice["oneOf"])
	}
	types := make(map[string]bool)
	for _, item := range oneOf {
		branch, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected oneOf branch to be map, got %T", item)
		}
		if typ, ok := branch["type"].(string); ok {
			types[typ] = true
		}
	}
	if !types["string"] || !types["integer"] || !types["null"] {
		t.Fatalf("expected string, integer, and null branches, got %v", oneOf)
	}
}

func TestAllowNullOptionalFields_DoesNotAddSiblingTypeToExistingOneOf(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"choice": map[string]interface{}{
				"type": "string",
				"oneOf": []interface{}{
					map[string]interface{}{"pattern": "^a"},
					map[string]interface{}{"pattern": "^b"},
				},
			},
		},
	}

	result := AllowNullOptionalFields(schema, "", nil)
	choice := propField(t, result, "choice")
	if choice["type"] != "string" {
		t.Fatalf("unsafe oneOf schema should keep original sibling type, got %v", choice["type"])
	}
	oneOf, ok := choice["oneOf"].([]interface{})
	if !ok {
		t.Fatalf("expected oneOf array, got %T", choice["oneOf"])
	}
	if len(oneOf) != 2 {
		t.Fatalf("unsafe oneOf schema should not receive a null branch, got %v", oneOf)
	}
}

func TestReplaceIntOrString_LeavesOtherFormats(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":   "string",
				"format": "date-time",
			},
		},
	}
	result := ReplaceIntOrString(schema)
	name := propField(t, result, "name")
	if name["format"] != "date-time" {
		t.Fatal("should preserve non-int-or-string format")
	}
}

func TestReplaceIntOrString_RecursesIntoArrayItems(t *testing.T) {
	schema := map[string]interface{}{
		"items": map[string]interface{}{
			"format": "int-or-string",
		},
	}
	result := ReplaceIntOrString(schema)
	items, ok := result["items"].(map[string]interface{})
	if !ok {
		t.Fatal("expected items to be map")
	}
	if _, ok := items["oneOf"]; !ok {
		t.Fatal("should recurse into nested objects")
	}
}

func TestAllowNullOptionalFields_ConvertsNonRequiredType(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
		},
	}
	result := AllowNullOptionalFields(schema, "", nil)
	name := propField(t, result, "name")
	typeVal := name["type"]
	arr, ok := typeVal.([]interface{})
	if !ok {
		t.Fatalf("expected type to be array, got %T", typeVal)
	}
	if len(arr) != 2 || arr[0] != "string" || arr[1] != "null" {
		t.Fatalf("expected [string, null], got %v", arr)
	}
}

func TestAllowNullOptionalFields_SkipsRequiredFields(t *testing.T) {
	schema := map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"name"},
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
		},
	}
	result := AllowNullOptionalFields(schema, "", nil)
	name := propField(t, result, "name")
	if name["type"] != "string" {
		t.Fatal("required field type should remain a string, not array")
	}
}

func TestAllowNullOptionalFields_MixedRequiredAndOptional(t *testing.T) {
	schema := map[string]interface{}{
		"type":     "object",
		"required": []interface{}{"name"},
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
			"description": map[string]interface{}{
				"type": "string",
			},
		},
	}
	result := AllowNullOptionalFields(schema, "", nil)
	props, ok := result["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be map")
	}

	// "name" is required — should stay as plain string
	nameField, ok := props["name"].(map[string]interface{})
	if !ok {
		t.Fatal("expected name to be map")
	}
	if nameField["type"] != "string" {
		t.Fatal("required field 'name' should remain type string")
	}

	// "description" is NOT required — should become ["string", "null"]
	descField, ok := props["description"].(map[string]interface{})
	if !ok {
		t.Fatal("expected description to be map")
	}
	descType := descField["type"]
	arr, ok := descType.([]interface{})
	if !ok {
		t.Fatalf("optional field 'description' should have array type, got %T: %v", descType, descType)
	}
	if len(arr) != 2 || arr[0] != "string" || arr[1] != "null" {
		t.Fatalf("expected [string, null], got %v", arr)
	}
}

func TestAllowNullOptionalFields_SkipsNullType(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"nothing": map[string]interface{}{
				"type": "null",
			},
		},
	}
	result := AllowNullOptionalFields(schema, "", nil)
	nothing := propField(t, result, "nothing")
	if nothing["type"] != "null" {
		t.Fatal("null type should not be modified")
	}
}

func TestConvert_AppliesAllTransforms(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"port": map[string]interface{}{
				"format": "int-or-string",
			},
			"name": map[string]interface{}{
				"type": "string",
			},
		},
	}
	result := Convert(schema)
	port := propField(t, result, "port")
	if _, ok := port["oneOf"]; !ok {
		t.Fatal("intOrString not applied")
	}
	name := propField(t, result, "name")
	typeVal := name["type"]
	if _, ok := typeVal.([]interface{}); !ok {
		t.Fatal("nullOptional not applied")
	}
}
