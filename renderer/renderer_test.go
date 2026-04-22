package renderer

import (
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testTemplate(t *testing.T) *template.Template {
	t.Helper()
	funcMap := template.FuncMap{
		"childNode": func(n *SchemaNode) *SchemaNode {
			if len(n.Properties) > 0 {
				return n
			}
			if n.Items != nil && len(n.Items.Properties) > 0 {
				return n.Items
			}
			return n
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(template.HTMLEscapeString(s))
		},
	}
	tmpl, err := template.New("schema").Funcs(funcMap).Parse(schemaTemplate)
	if err != nil {
		t.Fatalf("parsing template: %v", err)
	}
	return tmpl
}

func TestDisplayType_SimpleTypes(t *testing.T) {
	tests := []struct {
		name     string
		typVal   interface{}
		items    *SchemaNode
		oneOf    []*SchemaNode
		expected string
	}{
		{"string", "string", nil, nil, "string"},
		{"integer", "integer", nil, nil, "integer"},
		{"boolean", "boolean", nil, nil, "boolean"},
		{"number", "number", nil, nil, "number"},
		{"object", "object", nil, nil, "object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &SchemaNode{Type: tt.typVal, Items: tt.items, OneOf: tt.oneOf}
			if got := n.DisplayType(); got != tt.expected {
				t.Errorf("DisplayType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDisplayType_NullableTypes(t *testing.T) {
	tests := []struct {
		name     string
		typVal   interface{}
		expected string
	}{
		{"nullable string", []interface{}{"string", "null"}, "string"},
		{"nullable integer", []interface{}{"integer", "null"}, "integer"},
		{"nullable object", []interface{}{"object", "null"}, "object"},
		{"null first", []interface{}{"null", "boolean"}, "boolean"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &SchemaNode{Type: tt.typVal}
			if got := n.DisplayType(); got != tt.expected {
				t.Errorf("DisplayType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDisplayType_Arrays(t *testing.T) {
	tests := []struct {
		name     string
		items    *SchemaNode
		expected string
	}{
		{"array of strings", &SchemaNode{Type: "string"}, "[]string"},
		{"array of objects", &SchemaNode{Type: "object"}, "[]object"},
		{"array of integers", &SchemaNode{Type: "integer"}, "[]integer"},
		{"array no items", nil, "[]object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &SchemaNode{Type: "array", Items: tt.items}
			if got := n.DisplayType(); got != tt.expected {
				t.Errorf("DisplayType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDisplayType_IntOrString(t *testing.T) {
	n := &SchemaNode{
		OneOf: []*SchemaNode{
			{Type: "string"},
			{Type: "integer"},
		},
	}
	if got := n.DisplayType(); got != "string | integer" {
		t.Errorf("DisplayType() = %q, want %q", got, "string | integer")
	}
}

func TestDisplayType_NoType(t *testing.T) {
	n := &SchemaNode{}
	if got := n.DisplayType(); got != "object" {
		t.Errorf("DisplayType() = %q, want %q", got, "object")
	}
}

func TestIsRequired(t *testing.T) {
	parent := &SchemaNode{
		Required: []string{"name", "spec"},
	}
	if !parent.IsRequired("name") {
		t.Error("name should be required")
	}
	if !parent.IsRequired("spec") {
		t.Error("spec should be required")
	}
	if parent.IsRequired("status") {
		t.Error("status should not be required")
	}
}

func TestHasChildren(t *testing.T) {
	tests := []struct {
		name     string
		node     *SchemaNode
		expected bool
	}{
		{"object with properties", &SchemaNode{Type: "object", Properties: map[string]*SchemaNode{"a": {}}}, true},
		{"object no properties", &SchemaNode{Type: "object"}, false},
		{"array with object items", &SchemaNode{Type: "array", Items: &SchemaNode{Type: "object", Properties: map[string]*SchemaNode{"a": {}}}}, true},
		{"array with string items", &SchemaNode{Type: "array", Items: &SchemaNode{Type: "string"}}, false},
		{"string", &SchemaNode{Type: "string"}, false},
		{"nullable object with props", &SchemaNode{Type: []interface{}{"object", "null"}, Properties: map[string]*SchemaNode{"a": {}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.HasChildren(); got != tt.expected {
				t.Errorf("HasChildren() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSortedProperties(t *testing.T) {
	node := &SchemaNode{
		Properties: map[string]*SchemaNode{
			"zeta":  {Type: "string"},
			"alpha": {Type: "string"},
			"mu":    {Type: "string"},
		},
	}
	sorted := node.SortedProperties()
	if len(sorted) != 3 {
		t.Fatalf("expected 3 properties, got %d", len(sorted))
	}
	expected := []string{"alpha", "mu", "zeta"}
	for i, p := range sorted {
		if p.Name != expected[i] {
			t.Errorf("property %d: got %q, want %q", i, p.Name, expected[i])
		}
	}
}

func TestConstraints(t *testing.T) {
	minLen := int64(1)
	maxLen := int64(255)
	min := 0.0
	max := 100.0

	tests := []struct {
		name     string
		node     *SchemaNode
		expected []string
	}{
		{"enum", &SchemaNode{Enum: []interface{}{"TCP", "UDP"}}, []string{"enum: TCP, UDP"}},
		{"pattern", &SchemaNode{Pattern: "^[a-z]+$"}, []string{"pattern: ^[a-z]+$"}},
		{"format", &SchemaNode{Format: "date-time"}, []string{"format: date-time"}},
		{"min/max length", &SchemaNode{MinLength: &minLen, MaxLength: &maxLen}, []string{"minLength: 1", "maxLength: 255"}},
		{"min/max value", &SchemaNode{Minimum: &min, Maximum: &max}, []string{"minimum: 0", "maximum: 100"}},
		{"no constraints", &SchemaNode{Type: "string"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.Constraints()
			if len(got) != len(tt.expected) {
				t.Fatalf("Constraints() returned %d items, want %d: %v", len(got), len(tt.expected), got)
			}
			for i, c := range got {
				if c != tt.expected[i] {
					t.Errorf("constraint %d: got %q, want %q", i, c, tt.expected[i])
				}
			}
		})
	}
}

func TestRenderSchema_BasicOutput(t *testing.T) {
	schema := `{
		"type": "object",
		"required": ["spec"],
		"properties": {
			"spec": {
				"type": "object",
				"description": "Spec defines the desired state",
				"properties": {
					"replicas": {
						"type": "integer",
						"description": "Number of replicas",
						"minimum": 1,
						"maximum": 10
					},
					"name": {
						"type": "string",
						"description": "Resource name",
						"pattern": "^[a-z]+$"
					}
				},
				"required": ["replicas"]
			},
			"status": {
				"type": "object",
				"description": "Status of the resource"
			}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "myresource_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "example.io", "myresource_v1.json"), "example.io", "myresource_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "example.io", "myresource_v1.html"))
	if err != nil {
		t.Fatalf("HTML file not created: %v", err)
	}

	html := string(data)
	checks := []struct {
		substr string
		desc   string
	}{
		{"<!DOCTYPE html>", "valid HTML document"},
		{"Myresource v1", "page title with Kind and version"},
		{"example.io", "group name in metadata"},
		{"Myresource", "kind in metadata card"},
		{"v1", "version in metadata card"},
		{"apiVersion: example.io/v1", "YAML boilerplate"},
		{"kind: Myresource", "YAML boilerplate kind"},
		{"← Back to index", "back link"},
		{"Expand all", "expand all button"},
		{"Collapse all", "collapse all button"},
		{"View raw schema", "raw schema link"},
		{"myresource_v1.json", "link to JSON file"},
		{"spec", "spec property"},
		{"status", "status property"},
		{"replicas", "nested property"},
		{"Spec defines the desired state", "property description"},
		{"Number of replicas", "nested property description"},
		{"required", "required badge"},
		{"integer", "type badge"},
		{"object", "type badge for object"},
		{"minimum: 1", "constraint display"},
		{"maximum: 10", "constraint display"},
		{"pattern: ^[a-z]+$", "constraint display"},
		{"toggleTheme", "theme toggle function"},
		{"favicon.svg", "favicon link"},
		{"--accent", "CSS custom properties"},
		{"body::before", "starfield CSS"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("HTML should contain %s (looked for %q)", c.desc, c.substr)
		}
	}
}

func TestRenderSchema_LeafVsExpandable(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"nested": {
				"type": "object",
				"properties": {
					"inner": {"type": "string"}
				}
			}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "thing_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "thing_v1.json"), "test.io", "thing_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "test.io", "thing_v1.html"))
	html := string(data)

	if !strings.Contains(html, "<details") {
		t.Error("expandable property should use <details> element")
	}
	if !strings.Contains(html, "prop-leaf") {
		t.Error("leaf property should use prop-leaf class")
	}
}

func TestRenderSchema_ArrayTypes(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"tags": {
				"type": "array",
				"items": {"type": "string"},
				"description": "List of tags"
			},
			"servers": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"host": {"type": "string"}
					}
				}
			}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "thing_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "thing_v1.json"), "test.io", "thing_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "test.io", "thing_v1.html"))
	html := string(data)

	if !strings.Contains(html, "[]string") {
		t.Error("should show []string for string array")
	}
	if !strings.Contains(html, "[]object") {
		t.Error("should show []object for object array")
	}
}

func TestRenderSchema_IntOrString(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"port": {
				"oneOf": [{"type": "string"}, {"type": "integer"}],
				"description": "Port number or name"
			}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "thing_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "thing_v1.json"), "test.io", "thing_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "test.io", "thing_v1.html"))
	html := string(data)

	if !strings.Contains(html, "string | integer") {
		t.Error("should show 'string | integer' for oneOf int-or-string")
	}
}

func TestRenderSchema_EnumValues(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"protocol": {
				"type": "string",
				"enum": ["TCP", "UDP", "SCTP"],
				"description": "Network protocol"
			}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "thing_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "thing_v1.json"), "test.io", "thing_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "test.io", "thing_v1.html"))
	html := string(data)

	if !strings.Contains(html, "TCP") {
		t.Error("should show enum values")
	}
	if !strings.Contains(html, "enum:") {
		t.Error("should label enum constraint")
	}
}

func TestRenderSchema_MinimalSchema(t *testing.T) {
	schema := `{"type":"object"}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "empty_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "empty_v1.json"), "test.io", "empty_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "test.io", "empty_v1.html"))
	if err != nil {
		t.Fatalf("HTML file not created: %v", err)
	}

	html := string(data)
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("minimal schema should still produce valid HTML")
	}
	if !strings.Contains(html, "Empty") {
		t.Error("should derive Kind from filename")
	}
}

func TestRenderSchema_PreservesOriginalKindFromManifest(t *testing.T) {
	schema := `{"type":"object"}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "monitoring.coreos.com"), 0o755)
	_ = os.MkdirAll(filepath.Join(tmpDir, "_meta"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "monitoring.coreos.com", "servicemonitor_v1.json"), []byte(schema), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "_meta", "kinds.json"), []byte(`{"monitoring.coreos.com/servicemonitor_v1.json":"ServiceMonitor"}`), 0o644)

	err := renderSchemaFile(
		testTemplate(t),
		filepath.Join(tmpDir, "monitoring.coreos.com", "servicemonitor_v1.json"),
		"monitoring.coreos.com",
		"servicemonitor_v1.json",
		"",
	)
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "monitoring.coreos.com", "servicemonitor_v1.html"))
	if err != nil {
		t.Fatalf("HTML file not created: %v", err)
	}

	html := string(data)
	if !strings.Contains(html, "ServiceMonitor") {
		t.Fatal("expected exact Kind casing from manifest")
	}
	if strings.Contains(html, "Servicemonitor") {
		t.Fatal("expected title-cased fallback to be overridden")
	}
}

func TestRenderAll_CreatesHTMLFiles(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "cert-manager.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "certificate_v1.json"),
		[]byte(`{"type":"object","properties":{"spec":{"type":"object"}}}`), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "cert-manager.io", "issuer_v1.json"),
		[]byte(`{"type":"object"}`), 0o644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "monitoring.coreos.com"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "monitoring.coreos.com", "prometheus_v1.json"),
		[]byte(`{"type":"object"}`), 0o644)
	_ = os.MkdirAll(filepath.Join(tmpDir, "master-standalone"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "master-standalone", "test.json"),
		[]byte(`{"type":"object"}`), 0o644)

	err := RenderAll(tmpDir, "")
	if err != nil {
		t.Fatalf("RenderAll error: %v", err)
	}

	for _, path := range []string{
		"cert-manager.io/certificate_v1.html",
		"cert-manager.io/issuer_v1.html",
		"monitoring.coreos.com/prometheus_v1.html",
	} {
		if _, err := os.Stat(filepath.Join(tmpDir, path)); err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "master-standalone", "test.html")); err == nil {
		t.Error("master-standalone should not get HTML files")
	}
}

func TestRenderAll_SkipsNonJsonFiles(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(`{"type":"object"}`), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "README.md"), []byte(`# hello`), 0o644)

	err := RenderAll(tmpDir, "")
	if err != nil {
		t.Fatalf("RenderAll error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "example.io", "thing_v1.html")); err != nil {
		t.Error("JSON schema should get HTML")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "example.io", "README.html")); err == nil {
		t.Error("non-JSON file should not get HTML")
	}
}

func TestRenderAll_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	err := RenderAll(tmpDir, "")
	if err != nil {
		t.Fatalf("RenderAll should handle empty dir: %v", err)
	}
}

func TestRenderAll_SkipsMetadataDir(t *testing.T) {
	tmpDir := t.TempDir()
	groupDir := filepath.Join(tmpDir, "example.io")
	if err := os.MkdirAll(groupDir, 0o755); err != nil {
		t.Fatalf("mkdir group: %v", err)
	}
	if err := os.WriteFile(filepath.Join(groupDir, "test_v1.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	metaDir := filepath.Join(tmpDir, "_meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir metadata dir: %v", err)
	}
	manifest, err := json.Marshal(map[string]string{"example.io/test_v1.json": "Test"})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "kinds.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if err := RenderAll(tmpDir, ""); err != nil {
		t.Fatalf("RenderAll error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(groupDir, "test_v1.html")); err != nil {
		t.Fatalf("expected schema HTML file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(metaDir, "kinds.html")); !os.IsNotExist(err) {
		t.Fatalf("expected metadata dir to be skipped, got err=%v", err)
	}
}

func TestRenderSchema_BasePath(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(schema), 0o644)

	err := RenderAll(tmpDir, "/iac")
	if err != nil {
		t.Fatalf("RenderAll error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "example.io", "thing_v1.html"))
	if err != nil {
		t.Fatalf("HTML file not created: %v", err)
	}

	html := string(data)
	checks := []struct {
		substr string
		desc   string
	}{
		{`href="/iac/favicon.svg"`, "favicon with base path"},
		{`href="/iac/"`, "back link with base path"},
		{`href="/iac/example.io/thing_v1.json"`, "raw schema link with base path"},
		{`data-url="/iac/example.io/thing_v1.json"`, "copy URL data attr with base path"},
		{`data-base-path="/iac"`, "base path data attribute on body"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("HTML should contain %s (looked for %q)", c.desc, c.substr)
		}
	}
}

func TestRenderSchema_EmptyBasePath(t *testing.T) {
	schema := `{"type": "object", "properties": {"name": {"type": "string"}}}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "example.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "example.io", "thing_v1.json"), []byte(schema), 0o644)

	err := RenderAll(tmpDir, "")
	if err != nil {
		t.Fatalf("RenderAll error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "example.io", "thing_v1.html"))
	html := string(data)

	checks := []struct {
		substr string
		desc   string
	}{
		{`href="/favicon.svg"`, "favicon at root"},
		{`href="/"`, "back link at root"},
		{`href="/example.io/thing_v1.json"`, "raw schema link at root"},
		{`data-base-path=""`, "empty base path data attribute on body"},
	}
	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("HTML should contain %s (looked for %q)", c.desc, c.substr)
		}
	}
}

func TestRenderSchema_BooleanSchemaInProperties(t *testing.T) {
	// JSON Schema allows boolean schemas (true/false) as property values.
	// The renderer should handle these gracefully instead of crashing.
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"anything": true,
			"nothing": false
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "thing_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "thing_v1.json"), "test.io", "thing_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile should handle boolean schemas: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "test.io", "thing_v1.html"))
	html := string(data)

	if !strings.Contains(html, "name") {
		t.Error("should still render normal properties alongside boolean schemas")
	}
	if !strings.Contains(html, "anything") {
		t.Error("should render boolean true schema property")
	}
	if !strings.Contains(html, "nothing") {
		t.Error("should render boolean false schema property")
	}
}

func TestRenderSchema_CompositionWithBooleanSchemas(t *testing.T) {
	// JSON Schema allows boolean schemas (true/false) inside composition keywords
	// (oneOf, anyOf, allOf). The renderer should handle these gracefully.
	schema := `{
		"type": "object",
		"properties": {
			"flexible": {
				"oneOf": [{"type": "string"}, true]
			},
			"strict": {
				"anyOf": [{"type": "integer"}, false]
			},
			"composed": {
				"allOf": [{"type": "object", "properties": {"name": {"type": "string"}}}, true]
			}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "thing_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "thing_v1.json"), "test.io", "thing_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile should handle boolean schemas in composition keywords: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "test.io", "thing_v1.html"))
	html := string(data)

	if !strings.Contains(html, "flexible") {
		t.Error("should render the oneOf property with boolean schema")
	}
	if !strings.Contains(html, "strict") {
		t.Error("should render the anyOf property with boolean schema")
	}
	if !strings.Contains(html, "composed") {
		t.Error("should render the allOf property with boolean schema")
	}
	if !strings.Contains(html, "name") {
		t.Error("should render nested property inside allOf composition")
	}
}

func TestRenderSchema_DeepNesting(t *testing.T) {
	schema := `{
		"type": "object",
		"required": ["level1"],
		"properties": {
			"level1": {
				"type": "object",
				"required": ["level2"],
				"properties": {
					"level2": {
						"type": "object",
						"required": ["level3"],
						"properties": {
							"level3": {
								"type": "string",
								"description": "deeply nested"
							}
						}
					}
				}
			}
		}
	}`

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, "test.io"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "test.io", "deep_v1.json"), []byte(schema), 0o644)

	err := renderSchemaFile(testTemplate(t), filepath.Join(tmpDir, "test.io", "deep_v1.json"), "test.io", "deep_v1.json", "")
	if err != nil {
		t.Fatalf("renderSchemaFile error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "test.io", "deep_v1.html"))
	html := string(data)

	if !strings.Contains(html, "level1") {
		t.Error("should show level1")
	}
	if !strings.Contains(html, "level2") {
		t.Error("should show level2")
	}
	if !strings.Contains(html, "level3") {
		t.Error("should show level3")
	}
	if !strings.Contains(html, "deeply nested") {
		t.Error("should show deeply nested description")
	}
}
