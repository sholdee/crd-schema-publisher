package jsonschema

var SchemaMapKeywords = map[string]struct{}{
	"$defs":             {},
	"definitions":       {},
	"dependencies":      {},
	"patternProperties": {},
	"properties":        {},
}

var SchemaValueKeywords = map[string]struct{}{
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

var SchemaArrayKeywords = map[string]struct{}{
	"allOf": {},
	"anyOf": {},
	"oneOf": {},
}

func VisitChildSchemasForKeyword(k string, v any, visit func(map[string]any)) {
	if _, ok := SchemaMapKeywords[k]; ok {
		VisitSchemaMapValues(v, visit)
		return
	}
	if _, ok := SchemaValueKeywords[k]; ok {
		VisitSchemaValue(v, visit)
		return
	}
	if _, ok := SchemaArrayKeywords[k]; ok {
		VisitSchemaArray(v, visit)
	}
}

func VisitSchemaMapValues(v any, visit func(map[string]any)) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for _, item := range m {
		VisitSchemaValue(item, visit)
	}
}

func VisitSchemaValue(v any, visit func(map[string]any)) {
	if nested, ok := v.(map[string]any); ok {
		visit(nested)
		return
	}
	VisitSchemaArray(v, visit)
}

func VisitSchemaArray(v any, visit func(map[string]any)) {
	arr, ok := v.([]any)
	if !ok {
		return
	}
	for _, item := range arr {
		if nested, ok := item.(map[string]any); ok {
			visit(nested)
		}
	}
}
