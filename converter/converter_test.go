package converter

import (
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
