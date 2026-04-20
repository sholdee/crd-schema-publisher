package converter

import "os"

// AdditionalProperties walks the schema tree and adds "additionalProperties": false
// to any object that has "properties" but lacks "additionalProperties".
// When skip is true, the root object is not modified (only its children are).
func AdditionalProperties(data map[string]interface{}, skip bool) map[string]interface{} {
	if _, hasProps := data["properties"]; hasProps && !skip {
		if _, hasAP := data["additionalProperties"]; !hasAP {
			data["additionalProperties"] = false
		}
	}
	for k, v := range data {
		if k == "properties" {
			// The "properties" keyword maps property names to sub-schemas.
			// Recurse into each sub-schema individually — not the map itself,
			// which would corrupt schemas with a property named "properties".
			if props, ok := v.(map[string]interface{}); ok {
				for _, pv := range props {
					if propSchema, ok := pv.(map[string]interface{}); ok {
						AdditionalProperties(propSchema, false)
					}
				}
			}
			continue
		}
		if nested, ok := v.(map[string]interface{}); ok {
			AdditionalProperties(nested, false)
		}
	}
	return data
}

// ReplaceIntOrString finds {"format": "int-or-string"} and replaces with
// {"oneOf": [{"type": "string"}, {"type": "integer"}]}.
func ReplaceIntOrString(data map[string]interface{}) map[string]interface{} {
	if fmt, ok := data["format"]; ok && fmt == "int-or-string" {
		delete(data, "format")
		data["oneOf"] = []interface{}{
			map[string]interface{}{"type": "string"},
			map[string]interface{}{"type": "integer"},
		}
		return data
	}
	for k, v := range data {
		if nested, ok := v.(map[string]interface{}); ok {
			data[k] = ReplaceIntOrString(nested)
		} else if arr, ok := v.([]interface{}); ok {
			replaceIntOrStringSlice(arr)
		}
	}
	return data
}

func replaceIntOrStringSlice(arr []interface{}) {
	for i, v := range arr {
		if nested, ok := v.(map[string]interface{}); ok {
			arr[i] = ReplaceIntOrString(nested)
		}
	}
}

// AllowNullOptionalFields converts non-required fields with "type": "X" to
// "type": ["X", "null"]. This makes optional fields nullable in the JSON schema.
// propertyName is the field's key in its parent "properties" map.
// requiredSet contains the names of required fields from the parent object.
func AllowNullOptionalFields(data map[string]interface{}, propertyName string, requiredSet map[string]bool) map[string]interface{} {
	// If this property object has a "type" and is not required, make it nullable.
	if propertyName != "" && !requiredSet[propertyName] {
		if t, ok := data["type"].(string); ok && t != "null" {
			data["type"] = []interface{}{t, "null"}
		}
	}

	// Build the required set for this object's own properties.
	var childRequired map[string]bool
	if reqList, ok := data["required"].([]interface{}); ok {
		childRequired = make(map[string]bool, len(reqList))
		for _, r := range reqList {
			if s, ok := r.(string); ok {
				childRequired[s] = true
			}
		}
	}

	// Recurse into properties with the required set from this object.
	if props, ok := data["properties"].(map[string]interface{}); ok {
		for pk, pv := range props {
			if nested, ok := pv.(map[string]interface{}); ok {
				props[pk] = AllowNullOptionalFields(nested, pk, childRequired)
			}
		}
	}

	// Recurse into non-property children (e.g., items, additionalProperties schema).
	for k, v := range data {
		if k == "properties" {
			continue
		}
		if nested, ok := v.(map[string]interface{}); ok {
			data[k] = AllowNullOptionalFields(nested, "", nil)
		} else if arr, ok := v.([]interface{}); ok {
			allowNullSlice(arr)
		}
	}
	return data
}

func allowNullSlice(arr []interface{}) {
	for i, v := range arr {
		if nested, ok := v.(map[string]interface{}); ok {
			arr[i] = AllowNullOptionalFields(nested, "", nil)
		}
	}
}

// Convert applies all three transforms to a CRD OpenAPI schema in the correct order.
func Convert(schema map[string]interface{}) map[string]interface{} {
	skipRoot := os.Getenv("DENY_ROOT_ADDITIONAL_PROPERTIES") == ""
	schema = AdditionalProperties(schema, skipRoot)
	schema = ReplaceIntOrString(schema)
	schema = AllowNullOptionalFields(schema, "", nil)
	return schema
}
