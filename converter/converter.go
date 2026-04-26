package converter

import (
	"os"
	"reflect"
)

var stringValidationKeywords = map[string]struct{}{
	"maxLength": {},
	"minLength": {},
	"pattern":   {},
}

var integerValidationKeywords = map[string]struct{}{
	"exclusiveMaximum": {},
	"exclusiveMinimum": {},
	"maximum":          {},
	"minimum":          {},
	"multipleOf":       {},
}

var schemaMapKeywords = map[string]struct{}{
	"$defs":             {},
	"definitions":       {},
	"dependencies":      {},
	"patternProperties": {},
	"properties":        {},
}

var schemaValueKeywords = map[string]struct{}{
	"additionalItems":      {},
	"additionalProperties": {},
	"contains":             {},
	"else":                 {},
	"if":                   {},
	"items":                {},
	"not":                  {},
	"propertyNames":        {},
	"then":                 {},
}

var schemaArrayKeywords = map[string]struct{}{
	"allOf": {},
	"anyOf": {},
	"oneOf": {},
}

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
		visitAdditionalPropertiesChildSchemasForKeyword(k, v, func(nested map[string]interface{}) {
			AdditionalProperties(nested, false)
		})
	}
	return data
}

// ReplaceIntOrString finds Kubernetes int-or-string schemas and rewrites them
// as an explicit JSON Schema oneOf union.
func ReplaceIntOrString(data map[string]interface{}) map[string]interface{} {
	if isIntOrStringSchema(data) {
		replaceIntOrStringSchema(data)
		return data
	}
	for k, v := range data {
		visitChildSchemasForKeyword(k, v, func(nested map[string]interface{}) {
			ReplaceIntOrString(nested)
		})
	}
	return data
}

func visitChildSchemasForKeyword(k string, v interface{}, visit func(map[string]interface{})) {
	if _, ok := schemaMapKeywords[k]; ok {
		visitSchemaMapValues(v, visit)
		return
	}
	if _, ok := schemaValueKeywords[k]; ok {
		visitSchemaValue(v, visit)
		return
	}
	if _, ok := schemaArrayKeywords[k]; ok {
		visitSchemaArray(v, visit)
	}
}

func visitAdditionalPropertiesChildSchemasForKeyword(k string, v interface{}, visit func(map[string]interface{})) {
	switch k {
	case "$defs", "definitions", "patternProperties", "properties":
		visitSchemaMapValues(v, visit)
		return
	case "additionalItems", "additionalProperties", "items":
		visitSchemaValue(v, visit)
		return
	}
}

func visitSchemaMapValues(v interface{}, visit func(map[string]interface{})) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return
	}
	for _, item := range m {
		visitSchemaValue(item, visit)
	}
}

func visitSchemaValue(v interface{}, visit func(map[string]interface{})) {
	if nested, ok := v.(map[string]interface{}); ok {
		visit(nested)
		return
	}
	visitSchemaArray(v, visit)
}

func visitSchemaArray(v interface{}, visit func(map[string]interface{})) {
	arr, ok := v.([]interface{})
	if !ok {
		return
	}
	for _, item := range arr {
		if nested, ok := item.(map[string]interface{}); ok {
			visit(nested)
		}
	}
}

func isIntOrStringSchema(data map[string]interface{}) bool {
	if fmt, ok := data["format"]; ok && fmt == "int-or-string" {
		return true
	}
	xIntOrString, ok := data["x-kubernetes-int-or-string"].(bool)
	return ok && xIntOrString
}

func replaceIntOrStringSchema(data map[string]interface{}) {
	stringBranch := map[string]interface{}{"type": "string"}
	integerBranch := map[string]interface{}{"type": "integer"}
	nullable := mergeExistingIntOrStringCompositions(data, stringBranch, integerBranch)

	for k, v := range data {
		switch {
		case k == "format" && v == "int-or-string":
			delete(data, k)
		case k == "oneOf":
			delete(data, k)
		case k == "type":
			if typeIncludesNull(v) {
				nullable = true
			}
			delete(data, k)
		case k == "nullable":
			if b, ok := v.(bool); ok && b {
				nullable = true
			}
			delete(data, k)
		case isStringValidationKeyword(k):
			addBranchKeyword(stringBranch, k, v)
			delete(data, k)
		case isIntegerValidationKeyword(k):
			addBranchKeyword(integerBranch, k, v)
			delete(data, k)
		}
	}

	oneOf := []interface{}{stringBranch, integerBranch}
	if nullable {
		oneOf = append(oneOf, map[string]interface{}{"type": "null"})
		appendNullToAllOfOneOfConstraints(data)
	}
	data["oneOf"] = oneOf
}

func mergeExistingIntOrStringCompositions(data, stringBranch, integerBranch map[string]interface{}) bool {
	nullable := false

	if oneOf, ok := data["oneOf"].([]interface{}); ok {
		if branchNullable, matched := mergeTypedUnionBranches(oneOf, stringBranch, integerBranch); matched {
			if branchNullable {
				nullable = true
			}
		} else {
			appendParentAllOfSchema(data, map[string]interface{}{"oneOf": oneOf})
		}
		delete(data, "oneOf")
	}

	if anyOf, ok := data["anyOf"].([]interface{}); ok {
		if branchNullable, matched := mergeTypedUnionBranches(anyOf, stringBranch, integerBranch); matched {
			if branchNullable {
				nullable = true
			}
			delete(data, "anyOf")
		}
	}

	if allOf, ok := data["allOf"].([]interface{}); ok {
		keptAllOf := make([]interface{}, 0, len(allOf))
		for _, item := range allOf {
			schema, ok := item.(map[string]interface{})
			if !ok {
				keptAllOf = append(keptAllOf, item)
				continue
			}
			anyOf, ok := schema["anyOf"].([]interface{})
			if !ok {
				keptAllOf = append(keptAllOf, item)
				continue
			}
			branchNullable, matched := mergeTypedUnionBranches(anyOf, stringBranch, integerBranch)
			if !matched {
				keptAllOf = append(keptAllOf, item)
				continue
			}
			if branchNullable {
				nullable = true
			}
			remainingSchema := copySchemaExcept(schema, "anyOf")
			if len(remainingSchema) > 0 {
				keptAllOf = append(keptAllOf, remainingSchema)
			}
		}
		if len(keptAllOf) == 0 {
			delete(data, "allOf")
		} else {
			data["allOf"] = keptAllOf
		}
	}

	return nullable
}

func mergeTypedUnionBranches(branches []interface{}, stringBranch, integerBranch map[string]interface{}) (bool, bool) {
	nullable := false
	stringCount := 0
	integerCount := 0
	nullCount := 0
	hasString := false
	hasInteger := false
	stringCopy := copySchema(stringBranch)
	integerCopy := copySchema(integerBranch)

	for _, item := range branches {
		schema, ok := item.(map[string]interface{})
		if !ok {
			return false, false
		}
		schemaType := schema["type"]
		recognized := false
		if typeIncludes(schemaType, "string") {
			mergeBranchKeywords(stringCopy, schema)
			stringCount++
			hasString = true
			recognized = true
		}
		if typeIncludes(schemaType, "integer") {
			mergeBranchKeywords(integerCopy, schema)
			integerCount++
			hasInteger = true
			recognized = true
		}
		if typeIncludesNull(schemaType) {
			nullable = true
			nullCount++
			recognized = true
		}
		if b, ok := schema["nullable"].(bool); ok && b {
			nullable = true
			nullCount++
		}
		if !recognized {
			return false, false
		}
	}
	if !hasString || !hasInteger || stringCount != 1 || integerCount != 1 || nullCount > 1 {
		return false, false
	}
	replaceSchema(stringBranch, stringCopy)
	replaceSchema(integerBranch, integerCopy)
	return nullable, true
}

func mergeBranchKeywords(dst, src map[string]interface{}) {
	for k, v := range src {
		if k == "type" || k == "nullable" {
			continue
		}
		addBranchKeyword(dst, k, v)
	}
}

func addBranchKeyword(dst map[string]interface{}, k string, v interface{}) {
	if k == "allOf" {
		appendAllOfValue(dst, v)
		return
	}
	existing, found := dst[k]
	if !found {
		dst[k] = v
		return
	}
	if reflect.DeepEqual(existing, v) {
		return
	}
	appendAllOfSchema(dst, map[string]interface{}{k: v})
}

func appendAllOfValue(dst map[string]interface{}, v interface{}) {
	allOf, ok := v.([]interface{})
	if !ok {
		if _, found := dst["allOf"]; !found {
			dst["allOf"] = v
		}
		return
	}
	for _, schema := range allOf {
		appendAllOfSchema(dst, schema)
	}
}

func appendAllOfSchema(dst map[string]interface{}, schema interface{}) {
	allOf, _ := dst["allOf"].([]interface{})
	dst["allOf"] = append(allOf, schema)
}

func copySchemaExcept(schema map[string]interface{}, omit string) map[string]interface{} {
	copy := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		if k == omit {
			continue
		}
		copy[k] = v
	}
	return copy
}

func copySchema(schema map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		copy[k] = v
	}
	return copy
}

func replaceSchema(dst, src map[string]interface{}) {
	for k := range dst {
		delete(dst, k)
	}
	for k, v := range src {
		dst[k] = v
	}
}

func appendParentAllOfSchema(data map[string]interface{}, schema map[string]interface{}) {
	allOf, ok := data["allOf"].([]interface{})
	if !ok {
		if existing, found := data["allOf"]; found {
			allOf = []interface{}{existing}
		}
	}
	data["allOf"] = append(allOf, schema)
}

// AllowNullOptionalFields makes optional fields nullable in the JSON schema.
// Simple typed schemas become "type": ["X", "null"]; explicit oneOf type
// unions receive a null branch instead of a conflicting sibling type.
// propertyName is the field's key in its parent "properties" map.
// requiredSet contains the names of required fields from the parent object.
func AllowNullOptionalFields(data map[string]interface{}, propertyName string, requiredSet map[string]bool) map[string]interface{} {
	if isOptionalProperty(propertyName, requiredSet) {
		allowNullForOptionalField(data)
	}

	childRequired := childRequiredSet(data)
	allowNullProperties(data, childRequired)
	allowNullNonPropertyChildren(data)
	return data
}

func isOptionalProperty(propertyName string, requiredSet map[string]bool) bool {
	return propertyName != "" && !requiredSet[propertyName]
}

func allowNullForOptionalField(data map[string]interface{}) {
	_, hasOneOf := data["oneOf"]
	if hasOneOf && appendNullOneOf(data) {
		addNullToType(data)
		appendNullToAllOfOneOfConstraints(data)
	} else if !hasOneOf {
		addNullToType(data)
	}
}

func childRequiredSet(data map[string]interface{}) map[string]bool {
	reqList, ok := data["required"].([]interface{})
	if !ok {
		return nil
	}
	required := make(map[string]bool, len(reqList))
	for _, r := range reqList {
		if s, ok := r.(string); ok {
			required[s] = true
		}
	}
	return required
}

func allowNullProperties(data map[string]interface{}, childRequired map[string]bool) {
	if props, ok := data["properties"].(map[string]interface{}); ok {
		for pk, pv := range props {
			if nested, ok := pv.(map[string]interface{}); ok {
				props[pk] = AllowNullOptionalFields(nested, pk, childRequired)
			}
		}
	}
}

func allowNullNonPropertyChildren(data map[string]interface{}) {
	for k, v := range data {
		if k == "properties" {
			continue
		}
		visitChildSchemasForKeyword(k, v, func(nested map[string]interface{}) {
			AllowNullOptionalFields(nested, "", nil)
		})
	}
}

func appendNullOneOf(data map[string]interface{}) bool {
	oneOf, ok := data["oneOf"].([]interface{})
	if !ok || !hasOnlyTypedOneOfBranches(oneOf) {
		return false
	}
	for _, item := range oneOf {
		if schema, ok := item.(map[string]interface{}); ok && schema["type"] == "null" {
			return true
		}
	}
	data["oneOf"] = append(oneOf, map[string]interface{}{"type": "null"})
	return true
}

func appendNullToAllOfOneOfConstraints(data map[string]interface{}) {
	allOf, ok := data["allOf"].([]interface{})
	if !ok {
		return
	}
	for i, item := range allOf {
		schema, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if _, ok := schema["oneOf"].([]interface{}); ok {
			allOf[i] = map[string]interface{}{
				"anyOf": []interface{}{
					copySchema(schema),
					map[string]interface{}{"type": "null"},
				},
			}
		}
	}
}

func hasOnlyTypedOneOfBranches(oneOf []interface{}) bool {
	if len(oneOf) == 0 {
		return false
	}
	for _, item := range oneOf {
		schema, ok := item.(map[string]interface{})
		if !ok {
			return false
		}
		if _, ok := schema["type"].(string); !ok {
			return false
		}
	}
	return true
}

func typeIncludesNull(t interface{}) bool {
	return typeIncludes(t, "null")
}

func typeIncludes(t interface{}, target string) bool {
	if t == "null" {
		return target == "null"
	}
	if t == target {
		return true
	}
	types, ok := t.([]interface{})
	if !ok {
		return false
	}
	for _, typ := range types {
		if typ == target {
			return true
		}
	}
	return false
}

func addNullToType(data map[string]interface{}) {
	t, ok := data["type"]
	if !ok || typeIncludesNull(t) {
		return
	}
	switch typ := t.(type) {
	case string:
		data["type"] = []interface{}{typ, "null"}
	case []interface{}:
		data["type"] = append(append([]interface{}{}, typ...), "null")
	}
}

func isStringValidationKeyword(k string) bool {
	_, ok := stringValidationKeywords[k]
	return ok
}

func isIntegerValidationKeyword(k string) bool {
	_, ok := integerValidationKeywords[k]
	return ok
}

// Convert applies all three transforms to a CRD OpenAPI schema in the correct order.
func Convert(schema map[string]interface{}) map[string]interface{} {
	skipRoot := os.Getenv("DENY_ROOT_ADDITIONAL_PROPERTIES") == ""
	schema = AdditionalProperties(schema, skipRoot)
	schema = ReplaceIntOrString(schema)
	schema = AllowNullOptionalFields(schema, "", nil)
	return schema
}
