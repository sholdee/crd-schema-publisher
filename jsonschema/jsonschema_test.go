package jsonschema

import (
	"reflect"
	"testing"
)

func TestTypeIncludesHandlesStringAndArrayTypes(t *testing.T) {
	if !TypeIncludes("string", "string") {
		t.Fatal("expected string type to include string")
	}
	if !TypeIncludes([]any{"string", "null"}, "null") {
		t.Fatal("expected type array to include null")
	}
	if TypeIncludes([]any{"integer"}, "string") {
		t.Fatal("did not expect integer to include string")
	}
}

func TestNonNullTypePrefersFirstNonNullType(t *testing.T) {
	if got := NonNullType([]any{"null", "boolean"}); got != "boolean" {
		t.Fatalf("expected boolean, got %q", got)
	}
}

func TestRequiredSetAndIsRequiredMatchCurrentBehavior(t *testing.T) {
	required := RequiredSet([]string{"metadata", "spec"})
	if !IsRequired(required, "spec") {
		t.Fatal("expected spec to be required")
	}
	if IsRequired(required, "status") {
		t.Fatal("did not expect status to be required")
	}
}

func TestUniqueNonNullBranchTypesPreservesOrderAndDropsDuplicates(t *testing.T) {
	branches := []string{"null", "string", "integer", "string"}
	got := UniqueNonNullBranchTypes(branches, func(v string) string { return v })
	want := []string{"string", "integer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestVisitChildSchemasForKeywordVisitsCompositionAndProperties(t *testing.T) {
	root := map[string]any{
		"properties": map[string]any{
			"properties": map[string]any{"type": "object"},
			"spec":       map[string]any{"type": "object"},
		},
		"oneOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"type": "integer"},
		},
	}
	var visited int
	for k, v := range root {
		VisitChildSchemasForKeyword(k, v, func(map[string]any) { visited++ })
	}
	if visited != 4 {
		t.Fatalf("expected 4 visited child schemas, got %d", visited)
	}
}
