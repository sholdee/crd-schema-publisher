package renderer

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/sholdee/crd-schema-publisher/theme"
)

const (
	metadataDirName   = "_meta"
	kindsManifestName = "kinds.json"
)

// SchemaNode represents a JSON Schema node for rendering.
type SchemaNode struct {
	Type                 interface{}            `json:"type,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Properties           map[string]*SchemaNode `json:"properties,omitempty"`
	Items                *SchemaNode            `json:"items,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Enum                 []interface{}          `json:"enum,omitempty"`
	OneOf                []*SchemaNode          `json:"oneOf,omitempty"`
	AnyOf                []*SchemaNode          `json:"anyOf,omitempty"`
	AllOf                []*SchemaNode          `json:"allOf,omitempty"`
	Format               string                 `json:"format,omitempty"`
	Pattern              string                 `json:"pattern,omitempty"`
	Minimum              *float64               `json:"minimum,omitempty"`
	Maximum              *float64               `json:"maximum,omitempty"`
	MinLength            *int64                 `json:"minLength,omitempty"`
	MaxLength            *int64                 `json:"maxLength,omitempty"`
	MinItems             *int64                 `json:"minItems,omitempty"`
	MaxItems             *int64                 `json:"maxItems,omitempty"`
	Default              interface{}            `json:"default,omitempty"`
	AdditionalProperties interface{}            `json:"additionalProperties,omitempty"`
}

// UnmarshalJSON handles both regular JSON Schema objects and boolean schemas
// (true = accept any value, false = reject all values) which are valid in
// JSON Schema but cannot be decoded into a struct by the default unmarshaler.
func (n *SchemaNode) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && (data[0] == 't' || data[0] == 'f') {
		// Boolean schema — treat as empty node (no type, no properties).
		*n = SchemaNode{}
		return nil
	}

	// Decode as a regular object using an alias to avoid infinite recursion.
	type schemaAlias SchemaNode
	var alias schemaAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*n = SchemaNode(alias)
	return nil
}

// DisplayType returns a human-readable type string for the schema node.
func (n *SchemaNode) DisplayType() string {
	if len(n.OneOf) == 2 && n.Type == nil {
		types := make([]string, 0, 2)
		for _, o := range n.OneOf {
			types = append(types, o.DisplayType())
		}
		return strings.Join(types, " | ")
	}

	raw := n.resolveType()

	if raw == "array" {
		itemType := "object"
		if n.Items != nil {
			itemType = n.Items.resolveType()
		}
		return "[]" + itemType
	}

	if raw == "" {
		return "object"
	}

	return raw
}

// PropertyEntry is a name+node pair for sorted iteration.
type PropertyEntry struct {
	Name string
	Node *SchemaNode
}

// IsRequired returns true if the given property name is in this node's required list.
func (n *SchemaNode) IsRequired(name string) bool {
	for _, r := range n.Required {
		if r == name {
			return true
		}
	}
	return false
}

// HasChildren returns true if this node has nested properties to render.
func (n *SchemaNode) HasChildren() bool {
	if len(n.Properties) > 0 {
		return true
	}
	if n.Items != nil && len(n.Items.Properties) > 0 {
		return true
	}
	return false
}

// SortedProperties returns properties sorted alphabetically by name.
func (n *SchemaNode) SortedProperties() []PropertyEntry {
	entries := make([]PropertyEntry, 0, len(n.Properties))
	for name, node := range n.Properties {
		entries = append(entries, PropertyEntry{Name: name, Node: node})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// Constraints returns human-readable constraint strings for display.
func (n *SchemaNode) Constraints() []string {
	var cs []string
	if len(n.Enum) > 0 {
		vals := make([]string, len(n.Enum))
		for i, v := range n.Enum {
			vals[i] = fmt.Sprintf("%v", v)
		}
		cs = append(cs, "enum: "+strings.Join(vals, ", "))
	}
	if n.Pattern != "" {
		cs = append(cs, "pattern: "+n.Pattern)
	}
	if n.Format != "" {
		cs = append(cs, "format: "+n.Format)
	}
	if n.MinLength != nil {
		cs = append(cs, fmt.Sprintf("minLength: %d", *n.MinLength))
	}
	if n.MaxLength != nil {
		cs = append(cs, fmt.Sprintf("maxLength: %d", *n.MaxLength))
	}
	if n.MinItems != nil {
		cs = append(cs, fmt.Sprintf("minItems: %d", *n.MinItems))
	}
	if n.MaxItems != nil {
		cs = append(cs, fmt.Sprintf("maxItems: %d", *n.MaxItems))
	}
	if n.Minimum != nil {
		cs = append(cs, fmt.Sprintf("minimum: %g", *n.Minimum))
	}
	if n.Maximum != nil {
		cs = append(cs, fmt.Sprintf("maximum: %g", *n.Maximum))
	}
	return cs
}

// titleCase uppercases the first letter of s.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// schemaPageData holds template data for a single schema page.
type schemaPageData struct {
	Kind     string
	Group    string
	Version  string
	JSONPath string
	BasePath string
	Schema   *SchemaNode
}

// renderSchemaFile reads a JSON schema file and writes a sibling .html file.
func renderSchemaFile(tmpl *template.Template, jsonPath, group, filename, basePath string) error {
	kinds, err := loadKindManifest(filepath.Dir(filepath.Dir(jsonPath)))
	if err != nil {
		return err
	}
	return renderSchemaFileWithKinds(tmpl, jsonPath, group, filename, basePath, kinds)
}

func renderSchemaFileWithKinds(tmpl *template.Template, jsonPath, group, filename, basePath string, kinds map[string]string) error {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("reading schema %s: %w", jsonPath, err)
	}

	var schema SchemaNode
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("parsing schema %s: %w", jsonPath, err)
	}

	base := strings.TrimSuffix(filename, ".json")
	parts := strings.SplitN(base, "_", 2)
	kind := titleCase(parts[0])
	if manifestKind := lookupManifestKind(kinds, group, filename); manifestKind != "" {
		kind = manifestKind
	}
	version := ""
	if len(parts) == 2 {
		version = parts[1]
	}

	pageData := schemaPageData{
		Kind:     kind,
		Group:    group,
		Version:  version,
		JSONPath: basePath + "/" + group + "/" + filename,
		BasePath: basePath,
		Schema:   &schema,
	}

	htmlPath := strings.TrimSuffix(jsonPath, ".json") + ".html"
	f, err := os.Create(htmlPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", htmlPath, err)
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, pageData); err != nil {
		return err
	}
	return f.Close()
}

func loadKindManifest(outputDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(outputDir, metadataDirName, kindsManifestName))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("reading kind manifest: %w", err)
	}
	var kinds map[string]string
	if err := json.Unmarshal(data, &kinds); err != nil {
		return nil, fmt.Errorf("parsing kind manifest: %w", err)
	}
	return kinds, nil
}

func lookupManifestKind(kinds map[string]string, group, filename string) string {
	if len(kinds) == 0 {
		return ""
	}
	return strings.TrimSpace(kinds[filepath.ToSlash(filepath.Join(group, filename))])
}

type renderJob struct {
	jsonPath  string
	groupName string
	fileName  string
}

func collectRenderJobs(outputDir string) ([]renderJob, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("reading output dir: %w", err)
	}

	var jobs []renderJob
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "master-standalone" || entry.Name() == metadataDirName {
			continue
		}
		groupName := entry.Name()
		groupDir := filepath.Join(outputDir, groupName)
		files, err := os.ReadDir(groupDir)
		if err != nil {
			return nil, fmt.Errorf("reading group dir %s: %w", groupName, err)
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			jobs = append(jobs, renderJob{
				jsonPath:  filepath.Join(groupDir, f.Name()),
				groupName: groupName,
				fileName:  f.Name(),
			})
		}
	}
	return jobs, nil
}

// RenderAll walks the output directory and generates an HTML page for each JSON schema.
// Skips the master-standalone directory and non-JSON files.
func RenderAll(outputDir, basePath string) error {
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
		return fmt.Errorf("parsing template: %w", err)
	}

	jobs, err := collectRenderJobs(outputDir)
	if err != nil {
		return err
	}
	kinds, err := loadKindManifest(outputDir)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	errs := make(chan error, len(jobs))

	for _, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j renderJob) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := renderSchemaFileWithKinds(tmpl, j.jsonPath, j.groupName, j.fileName, basePath, kinds); err != nil {
				errs <- fmt.Errorf("rendering %s/%s: %w", j.groupName, j.fileName, err)
			}
		}(job)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		return err
	}
	return nil
}

var schemaTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Kind}} {{.Version}} — {{.Group}}</title>
<link rel="icon" type="image/svg+xml" href="{{.BasePath}}/favicon.svg">
<style>` + theme.CSSVars + theme.CSSBase + `
  a { color: var(--accent); text-decoration: none; }
  a:hover { text-decoration: underline; }
  .nav-row {
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 1.5rem;
  }
  .back-link { font-size: 0.85rem; display: flex; align-items: center; gap: 0.4rem; }
  .back-link kbd {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    font-size: 0.6rem; color: var(--fg-muted); background: var(--bg-surface);
    border: 1px solid var(--border); border-radius: 4px;
    padding: 0.1rem 0.35rem; line-height: 1;
  }
  .meta-cards {
    display: flex; gap: 0.75rem; margin-bottom: 1.25rem; flex-wrap: wrap;
  }
  .meta-card {
    background: var(--bg-surface); border: 1px solid var(--border);
    border-radius: 6px; padding: 0.5rem 1rem;
  }
  .meta-card .label { font-size: 0.7rem; color: var(--fg-muted); text-transform: uppercase; letter-spacing: 0.05em; }
  .meta-card .value { font-size: 1rem; font-weight: 600; }
  .yaml-block {
    background: var(--bg-surface); border: 1px solid var(--border);
    border-radius: 6px; padding: 0.75rem 1rem; margin-bottom: 1.5rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    font-size: 0.8rem; white-space: pre; overflow-x: auto; color: var(--fg);
  }
  .toolbar {
    display: flex; gap: 1rem; margin-bottom: 1rem; flex-wrap: wrap;
    align-items: center; justify-content: space-between;
  }
  .toolbar-left { display: flex; gap: 0.75rem; }
  .toolbar button, .toolbar a {
    background: none; border: none; color: var(--fg-muted); cursor: pointer;
    font-size: 0.8rem; padding: 0.2rem 0; transition: color 0.15s;
  }
  .toolbar button:hover, .toolbar a:hover { color: var(--accent); text-decoration: underline; }
  .prop {
    border: 1px solid var(--border); border-radius: 6px;
    margin-bottom: 0.35rem; transition: border-color 0.2s;
  }
  .prop[open] { border-color: var(--border-active); border-left-width: 2px; }
  .prop > summary {
    padding: 0.5rem 0.75rem; cursor: pointer;
    font-size: 0.85rem; background: var(--bg-surface); border-radius: 6px;
    list-style: none; display: flex; align-items: center; gap: 0.5rem;
    transition: background 0.15s;
  }
  .prop > summary::-webkit-details-marker { display: none; }
  .prop > summary::before { content: "\25B8"; color: var(--fg-muted); font-size: 0.7rem; }
  .prop[open] > summary::before { content: "\25BE"; color: var(--accent); }
  .prop > summary:hover { background: var(--bg-hover); }
  .prop-content { padding: 0.5rem 0.75rem 0.75rem; padding-left: 1.5rem; }
  .prop-leaf {
    padding: 0.5rem 0.75rem;
    font-size: 0.85rem; display: flex; align-items: flex-start; gap: 0.5rem;
    border: 1px solid var(--border); border-radius: 6px;
    margin-bottom: 0.35rem; background: var(--bg-surface);
  }
  .prop-leaf .prop-name { min-width: 0; }
  .prop-name {
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    color: var(--accent); font-weight: 600; white-space: nowrap;
  }
  .type-badge {
    background: var(--accent-dim); color: var(--accent);
    font-size: 0.65rem; font-weight: 700; padding: 0.1rem 0.4rem;
    border-radius: 8px; white-space: nowrap;
  }
  .required-badge {
    background: var(--required-bg); color: var(--required-fg);
    font-size: 0.65rem; font-weight: 700; padding: 0.1rem 0.4rem;
    border-radius: 8px; white-space: nowrap;
  }
  .prop-desc { color: var(--fg-muted); font-size: 0.82rem; margin-top: 0.25rem; }
  .prop-constraints {
    color: var(--fg-muted); font-size: 0.75rem; margin-top: 0.2rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
  }
  .prop-children { margin-top: 0.5rem; }
  .leaf-desc { color: var(--fg-muted); font-size: 0.82rem; flex: 1; min-width: 0; }
  .leaf-constraints {
    color: var(--fg-muted); font-size: 0.75rem; margin-top: 0.15rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
  }
</style>
` + theme.HeadScript + `
</head>
<body data-base-path="{{.BasePath}}">
` + theme.FlareDiv + `
<div class="nav-row">
  <a href="{{.BasePath}}/" class="back-link">← Back to index <kbd>Esc</kbd></a>
  ` + theme.ThemeToggleButton + `
</div>
<div class="meta-cards">
  <div class="meta-card"><div class="label">Kind</div><div class="value">{{.Kind}}</div></div>
  <div class="meta-card"><div class="label">Group</div><div class="value">{{.Group}}</div></div>
  <div class="meta-card"><div class="label">Version</div><div class="value">{{.Version}}</div></div>
</div>
<div class="yaml-block">apiVersion: {{.Group}}/{{.Version}}
kind: {{.Kind}}
metadata:
  name: example</div>
<div class="toolbar">
  <div class="toolbar-left">
    <button id="expand-all">Expand all</button>
    <button id="collapse-all">Collapse all</button>
  </div>
  <div class="toolbar-left">
    <a href="{{.JSONPath}}" target="_blank">View raw schema</a>
    <button id="copy-url" data-url="{{.JSONPath}}">Copy schema URL</button>
  </div>
</div>
{{- define "properties"}}
{{- range .SortedProperties}}
{{- if .Node.HasChildren}}
<details class="prop">
<summary>
  <span class="prop-name">{{.Name}}</span>
  <span class="type-badge">{{.Node.DisplayType}}</span>
  {{- if $.IsRequired .Name}} <span class="required-badge">required</span>{{end}}
</summary>
<div class="prop-content">
  {{- if .Node.Description}}<div class="prop-desc">{{.Node.Description}}</div>{{end}}
  {{- range .Node.Constraints}}<div class="prop-constraints">{{safeHTML .}}</div>{{end}}
  <div class="prop-children">
  {{- template "properties" (childNode .Node)}}
  </div>
</div>
</details>
{{- else}}
<div class="prop-leaf">
  <span class="prop-name">{{.Name}}</span>
  <span class="type-badge">{{.Node.DisplayType}}</span>
  {{- if $.IsRequired .Name}} <span class="required-badge">required</span>{{end}}
  <div class="leaf-desc">
    {{- if .Node.Description}}{{.Node.Description}}{{end}}
    {{- range .Node.Constraints}}<div class="leaf-constraints">{{safeHTML .}}</div>{{end}}
  </div>
</div>
{{- end}}
{{- end}}
{{- end}}
<div id="properties">
{{- template "properties" .Schema}}
</div>
` + theme.ToastDiv + `
` + theme.FooterHTML + `
<script>
(function(){
  var props = document.querySelectorAll('.prop');
  document.getElementById('expand-all').addEventListener('click', function(){
    props.forEach(function(p){ p.setAttribute('open',''); });
  });
  document.getElementById('collapse-all').addEventListener('click', function(){
    props.forEach(function(p){ p.removeAttribute('open'); });
  });
  var toast = document.getElementById('toast');
  var toastTimer;
  document.addEventListener('keydown', function(e){
    if (e.key === 'Escape') {
      e.preventDefault();
      if (document.activeElement && document.activeElement !== document.body) {
        document.activeElement.blur();
      }
      location.href = document.body.dataset.basePath + '/';
    }
  });
  document.getElementById('copy-url').addEventListener('click', function(){
    var url = location.origin + this.dataset.url;
    navigator.clipboard.writeText(url).then(function(){
      clearTimeout(toastTimer);
      toast.classList.add('show');
      toastTimer = setTimeout(function(){ toast.classList.remove('show'); }, 1500);
    });
  });
})();
` + theme.ThemeToggleJS + `
</script>
</body>
</html>`

// resolveType extracts the non-null type from the Type field.
func (n *SchemaNode) resolveType() string {
	switch t := n.Type.(type) {
	case string:
		return t
	case []interface{}:
		for _, v := range t {
			if s, ok := v.(string); ok && s != "null" {
				return s
			}
		}
	}
	return ""
}
