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

// RenderProperty holds the rendered metadata for a schema property row.
type RenderProperty struct {
	Name       string
	Path       string
	PathKey    string
	ParentPath string
	SearchText string
	Required   bool
	Node       *SchemaNode
	Children   []RenderProperty
}

// Expandable returns true when the property renders as an expandable details row.
func (p RenderProperty) Expandable() bool {
	return len(p.Children) > 0
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
	Kind           string
	Group          string
	Version        string
	JSONPath       string
	BasePath       string
	Schema         *SchemaNode
	Properties     []RenderProperty
	SearchPathHint string
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

	properties := buildRenderProperties(&schema, "", "", "")
	pageData := schemaPageData{
		Kind:           kind,
		Group:          group,
		Version:        version,
		JSONPath:       basePath + "/" + group + "/" + filename,
		BasePath:       basePath,
		Schema:         &schema,
		Properties:     properties,
		SearchPathHint: searchPathExampleForProperties(properties),
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

func buildRenderProperties(node *SchemaNode, parentPath, parentRowPath, arraySuffix string) []RenderProperty {
	entries := node.SortedProperties()
	props := make([]RenderProperty, 0, len(entries))
	for _, entry := range entries {
		path := joinPropertyPath(parentPath, entry.Name, arraySuffix)
		childArraySuffix := ""
		if entry.Node.resolveType() == "array" {
			childArraySuffix = "[]"
		}

		prop := RenderProperty{
			Name:       entry.Name,
			Path:       path,
			PathKey:    buildPathSearchKey(path),
			ParentPath: parentRowPath,
			SearchText: buildSearchText(entry.Node),
			Required:   node.IsRequired(entry.Name),
			Node:       entry.Node,
		}
		if entry.Node.HasChildren() {
			childNode := entry.Node
			if entry.Node.Items != nil && len(entry.Node.Items.Properties) > 0 {
				childNode = entry.Node.Items
			}
			prop.Children = buildRenderProperties(childNode, path+childArraySuffix, path, "")
		}
		props = append(props, prop)
	}
	return props
}

func searchPathExampleForProperties(props []RenderProperty) string {
	paths := collectSearchPaths(props)
	if len(paths) == 0 {
		return ".spec"
	}

	if match := firstMatchingPath(paths, func(path string) bool {
		return strings.HasPrefix(path, "spec.") && pathDepth(path) >= 2 && pathDepth(path) <= 4 && !isGenericHintPath(path)
	}); match != "" {
		return "." + match
	}
	if match := firstMatchingPath(paths, func(path string) bool {
		return strings.HasPrefix(path, "spec.") && !isGenericHintPath(path)
	}); match != "" {
		return "." + match
	}
	if match := firstMatchingPath(paths, func(path string) bool {
		return pathDepth(path) >= 2 && pathDepth(path) <= 4 && !isGenericHintPath(path)
	}); match != "" {
		return "." + match
	}
	if match := firstMatchingPath(paths, func(path string) bool {
		return !isGenericHintPath(path)
	}); match != "" {
		return "." + match
	}
	return "." + paths[0]
}

func collectSearchPaths(props []RenderProperty) []string {
	paths := make([]string, 0)
	for _, prop := range props {
		paths = append(paths, prop.Path)
		if len(prop.Children) > 0 {
			paths = append(paths, collectSearchPaths(prop.Children)...)
		}
	}
	return paths
}

func firstMatchingPath(paths []string, match func(string) bool) string {
	for _, path := range paths {
		if match(path) {
			return path
		}
	}
	return ""
}

func pathDepth(path string) int {
	return len(strings.Split(path, "."))
}

func isGenericHintPath(path string) bool {
	lowerPath := strings.ToLower(strings.TrimSpace(path))
	if lowerPath == "" {
		return true
	}

	parts := strings.Split(lowerPath, ".")
	last := parts[len(parts)-1]
	switch parts[0] {
	case "apiversion", "kind", "metadata", "status":
		return true
	}
	switch last {
	case "name", "namespace", "labels", "annotations":
		return true
	}
	return false
}

func joinPropertyPath(parentPath, name, arraySuffix string) string {
	base := name
	if parentPath != "" {
		base = parentPath + "." + name
	}
	return base + arraySuffix
}

func buildPathSearchKey(path string) string {
	if path == "" {
		return "|"
	}
	parts := strings.Split(path, ".")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return "|" + strings.Join(filtered, "|") + "|"
}

func buildSearchText(node *SchemaNode) string {
	parts := make([]string, 0, 1+len(node.Constraints()))
	if node.Description != "" {
		parts = append(parts, node.Description)
	}
	parts = append(parts, node.Constraints()...)
	return strings.Join(parts, " ")
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
	if err := theme.WriteSchemaSearchAsset(outputDir); err != nil {
		return fmt.Errorf("writing schema search asset: %w", err)
	}

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
` + theme.SearchCSS + `
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
  .search-row {
    width: 100%;
    display: flex; flex-direction: column; gap: 0.35rem;
    margin-bottom: 1rem;
  }
  .search-input-wrap {
    display: grid;
    position: relative;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-surface);
    font: inherit; font-size: 0.95rem; line-height: 1.2;
    font-family: inherit; font-weight: inherit; letter-spacing: inherit;
    text-transform: inherit; text-indent: inherit; font-kerning: inherit;
  }
  .search-input-wrap:focus-within { border-color: var(--accent); }
  .search-row .search-box {
    grid-area: 1 / 1;
    position: relative; z-index: 1;
    background: transparent; border-color: transparent;
    margin: 0; appearance: none; -webkit-appearance: none;
    font: inherit; line-height: inherit;
    font-family: inherit; font-size: inherit; font-weight: inherit; letter-spacing: inherit;
    text-transform: inherit; text-indent: inherit; font-kerning: inherit;
    width: 100%;
  }
  .search-row .search-box:focus { border-color: transparent; }
  .search-row .search-box::-webkit-search-decoration,
  .search-row .search-box::-webkit-search-cancel-button,
  .search-row .search-box::-webkit-search-results-button,
  .search-row .search-box::-webkit-search-results-decoration { display: none; }
  .search-ghost {
    grid-area: 1 / 1;
    position: absolute; inset: 0;
    padding: 0 1rem; line-height: inherit;
    display: flex; align-items: center;
    pointer-events: none; white-space: pre; overflow: hidden;
    font: inherit; letter-spacing: inherit;
    font-family: inherit; font-size: inherit; font-weight: inherit;
    text-transform: inherit; text-indent: inherit; font-kerning: inherit;
  }
  .search-ghost-prefix,
  .search-ghost-suffix {
    font: inherit; line-height: inherit; letter-spacing: inherit;
    font-family: inherit; font-size: inherit; font-weight: inherit;
    text-transform: inherit; text-indent: inherit; font-kerning: inherit;
  }
  .search-ghost-prefix { visibility: hidden; }
  .search-ghost-suffix { color: var(--fg-muted); opacity: 0.75; }
  .toolbar-groups {
    display: contents;
  }
  .toolbar-left { display: flex; gap: 0.75rem; flex-wrap: wrap; }
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
  .prop.search-match,
  .prop-leaf.search-match {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent-dim);
  }
  .prop.search-ancestor,
  .prop-leaf.search-ancestor {
    border-color: var(--border-active);
  }
  .prop > summary {
    padding: 0.5rem 0.75rem; cursor: pointer;
    font-size: 0.85rem; background: var(--bg-surface); border-radius: 6px;
    list-style: none; display: flex; align-items: center; gap: 0.5rem;
    transition: background 0.15s;
  }
  .prop.search-match > summary,
  .prop-leaf.search-match { background: var(--accent-dim); }
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
<div class="search-row">
  <div class="search-input-wrap">
    <input type="search" class="search-box" placeholder="Search schema fields...  ` + theme.SearchHintText + `" id="search" autocomplete="off" spellcheck="false">
    <div class="search-ghost" id="search-ghost" aria-hidden="true"><span class="search-ghost-prefix" id="search-ghost-prefix"></span><span class="search-ghost-suffix" id="search-ghost-suffix"></span></div>
  </div>
  <div class="search-status" id="search-status" data-empty-message="Tip: use {{.SearchPathHint}} for path-only search" aria-live="polite">Tip: use {{.SearchPathHint}} for path-only search</div>
</div>
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
{{- define "property"}}
{{- if .Expandable}}
<details class="prop" data-prop-row data-path="{{.Path}}" data-path-key="{{.PathKey}}" data-parent-path="{{.ParentPath}}" data-name="{{.Name}}" data-text="{{.SearchText}}">
<summary>
  <span class="prop-name">{{.Name}}</span>
  <span class="type-badge">{{.Node.DisplayType}}</span>
  {{- if .Required}} <span class="required-badge">required</span>{{end}}
</summary>
<div class="prop-content">
  {{- if .Node.Description}}<div class="prop-desc">{{.Node.Description}}</div>{{end}}
  {{- range .Node.Constraints}}<div class="prop-constraints">{{safeHTML .}}</div>{{end}}
  <div class="prop-children">
  {{- range .Children}}{{template "property" .}}{{end}}
  </div>
</div>
</details>
{{- else}}
<div class="prop-leaf" data-prop-row data-path="{{.Path}}" data-path-key="{{.PathKey}}" data-parent-path="{{.ParentPath}}" data-name="{{.Name}}" data-text="{{.SearchText}}">
  <span class="prop-name">{{.Name}}</span>
  <span class="type-badge">{{.Node.DisplayType}}</span>
  {{- if .Required}} <span class="required-badge">required</span>{{end}}
  <div class="leaf-desc">
    {{- if .Node.Description}}{{.Node.Description}}{{end}}
    {{- range .Node.Constraints}}<div class="leaf-constraints">{{safeHTML .}}</div>{{end}}
  </div>
</div>
{{- end}}
{{- end}}
<div id="properties">
{{- range .Properties}}{{template "property" .}}{{end}}
</div>
<p class="no-results" id="no-results" data-no-results-message="No matches. Try {{.SearchPathHint}} for an exact path">No matches. Try {{.SearchPathHint}} for an exact path</p>
` + theme.ToastDiv + `
` + theme.FooterHTML + `
<script src="{{.BasePath}}/` + theme.SchemaSearchAssetName + `"></script>
<script>
` + theme.SearchHashStateJS + `
(function(){
  var input = document.getElementById('search');
  var searchGhost = document.getElementById('search-ghost');
  var searchGhostPrefix = document.getElementById('search-ghost-prefix');
  var searchGhostSuffix = document.getElementById('search-ghost-suffix');
  var searchStatus = document.getElementById('search-status');
  var noResults = document.getElementById('no-results');
  var props = document.querySelectorAll('.prop');
  var rows = Array.prototype.slice.call(document.querySelectorAll('[data-prop-row]'));
  var learnedPathSearchStorageKey = 'crd-schema-publisher:path-search-learned';
  var completionCandidates = [];
  var completionIndex = -1;
  var rowByPath = {};
  rows.forEach(function(row){
    rowByPath[row.dataset.path] = row;
  });

  function visibleRows() {
    return rows.filter(function(row){ return row.style.display !== 'none'; });
  }

  function setSearchStatus(message, hasResults) {
    searchStatus.textContent = message;
    searchStatus.classList.toggle('has-results', !!hasResults);
  }

  function hasLearnedPathSearch() {
    try {
      return localStorage.getItem(learnedPathSearchStorageKey) === '1';
    } catch (err) {
      return false;
    }
  }

  function markPathSearchLearned(rawQuery) {
    var query = (rawQuery || '').trim();
    if (query.indexOf('.') !== 0) {
      return;
    }
    var queryState = splitPathSegments(query);
    if (queryState.segments.length < 2) {
      return;
    }
    try {
      localStorage.setItem(learnedPathSearchStorageKey, '1');
    } catch (err) {
      // Ignore storage failures and fall back to showing the tip.
    }
  }

  function currentSuggestions() {
    return rows.map(function(row){ return row.dataset.path || ''; });
  }

  function selectedCompletion() {
    if (completionIndex < 0 || completionIndex >= completionCandidates.length) {
      return '';
    }
    return completionCandidates[completionIndex];
  }

  function bestCompletionForQuery(query) {
    return bestCompletionForPaths(query, currentSuggestions());
  }

  function updateGhostSuggestion(rawQuery) {
    var completion = selectedCompletion() || bestCompletionForQuery(rawQuery);
    var suffix = ghostSuffixForCompletion(input.value, completion);
    var prefix = ghostPrefixForCompletion(input.value, completion);
    searchGhost.hidden = !suffix;
    searchGhostPrefix.textContent = suffix ? prefix : '';
    searchGhostSuffix.textContent = suffix;
  }

  function clearSearchState() {
    rows.forEach(function(row){
      row.style.display = '';
      row.classList.remove('search-match', 'search-ancestor');
      if (row.tagName === 'DETAILS') {
        row.removeAttribute('open');
      }
    });
    noResults.style.display = 'none';
    setSearchStatus(hasLearnedPathSearch() ? '' : (searchStatus.dataset.emptyMessage || ''), false);
    searchGhost.hidden = true;
    searchGhostPrefix.textContent = '';
    searchGhostSuffix.textContent = '';
    completionCandidates = [];
    completionIndex = -1;
  }

  var toast = document.getElementById('toast');
  var toastTimer;
  document.getElementById('copy-url').addEventListener('click', function(){
    var url = location.origin + this.dataset.url;
    navigator.clipboard.writeText(url).then(function(){
      clearTimeout(toastTimer);
      toast.classList.add('show');
      toastTimer = setTimeout(function(){ toast.classList.remove('show'); }, 1500);
    });
  });

  var schemaSearch = window.SchemaSearch;
  if (!schemaSearch) {
    clearSearchState();
    input.value = '';
    input.disabled = true;
    input.placeholder = 'Schema search unavailable';
    setSearchStatus('Search unavailable', false);
    if (window.console && console.warn) {
      console.warn('schema-search.js failed to load; schema search disabled');
    }
    document.getElementById('expand-all').addEventListener('click', function(){
      props.forEach(function(p){ p.setAttribute('open',''); });
    });
    document.getElementById('collapse-all').addEventListener('click', function(){
      props.forEach(function(p){ p.removeAttribute('open'); });
    });
    document.addEventListener('keydown', function(e){
      if (e.key !== 'Escape') {
        return;
      }
      e.preventDefault();
      var hadOpen = false;
      visibleRows().forEach(function(row){
        if (row.tagName === 'DETAILS' && row.hasAttribute('open')) {
          hadOpen = true;
          row.removeAttribute('open');
        }
      });
      if (hadOpen) {
        return;
      }
      if (document.activeElement && document.activeElement !== document.body) {
        document.activeElement.blur();
        return;
      }
      location.href = document.body.dataset.basePath + '/';
    });
    return;
  }

  var bestCompletionForPaths = schemaSearch.bestCompletionForPaths;
  var completionCandidatesForPaths = schemaSearch.completionCandidatesForPaths;
  var ghostPrefixForCompletion = schemaSearch.ghostPrefixForCompletion;
  var ghostSuffixForCompletion = schemaSearch.ghostSuffixForCompletion;
  var dotAdvanceForPathSearch = schemaSearch.dotAdvanceForPathSearch;
  var isPathLikeQuery = schemaSearch.isPathLikeQuery;
  var matchesPathQuery = schemaSearch.matchesPathQuery;
  var splitPathSegments = schemaSearch.splitPathSegments;
  var trimPathSearch = schemaSearch.trimPathSearch;

  function addAncestorPaths(path, visiblePaths, openPaths) {
    var current = path;
    var directMatch = true;
    while (current) {
      visiblePaths[current] = true;
      if (!directMatch) {
        openPaths[current] = true;
      }
      var row = rowByPath[current];
      if (!row || !row.dataset.parentPath) {
        break;
      }
      current = row.dataset.parentPath;
      directMatch = false;
    }
  }

  function applySearch(rawQuery) {
    var trimmedQuery = rawQuery.trim();
    var query = trimmedQuery.toLowerCase();
    if (!query) {
      clearSearchState();
      writeHashSearchQuery('');
      return;
    }

    var pathOnly = query.indexOf('.') === 0;
    if (pathOnly) {
      query = query.slice(1);
    }
    if (pathOnly && !query) {
      clearSearchState();
      writeHashSearchQuery('');
      return;
    }

    markPathSearchLearned(rawQuery);

    completionCandidates = completionCandidatesForPaths(rawQuery.trim(), currentSuggestions());
    completionIndex = -1;

    var directMatches = {};
    var visiblePaths = {};
    var openPaths = {};
    rows.forEach(function(row){
      var path = (row.dataset.path || '').toLowerCase();
      var name = (row.dataset.name || '').toLowerCase();
      var text = (row.dataset.text || '').toLowerCase();
      var pathMatch = query.indexOf('.') !== -1
        ? matchesPathQuery(path, trimmedQuery)
        : path.indexOf(query) !== -1;
      var matched = pathOnly
        ? matchesPathQuery(path, trimmedQuery)
        : pathMatch || name.indexOf(query) !== -1 || text.indexOf(query) !== -1;
      if (!matched) {
        return;
      }
      directMatches[row.dataset.path] = true;
      addAncestorPaths(row.dataset.path, visiblePaths, openPaths);
    });

    var selectedRow = rows.find(function(row){ return !!directMatches[row.dataset.path]; });
    if (selectedRow && selectedRow.tagName === 'DETAILS') {
      openPaths[selectedRow.dataset.path] = true;
    }

    var matchCount = Object.keys(directMatches).length;
    rows.forEach(function(row){
      var path = row.dataset.path;
      var visible = !!visiblePaths[path];
      var open = !!openPaths[path];
      row.style.display = visible ? '' : 'none';
      row.classList.toggle('search-match', !!directMatches[path]);
      row.classList.toggle('search-ancestor', visible && !directMatches[path]);
      if (row.tagName === 'DETAILS') {
        if (open) {
          row.setAttribute('open', '');
        } else {
          row.removeAttribute('open');
        }
      }
    });

    noResults.style.display = matchCount ? 'none' : 'block';
    noResults.textContent = noResults.dataset.noResultsMessage || 'No matches';
    setSearchStatus(matchCount ? matchCount + ' matches' : 'No matches', matchCount > 0);
    updateGhostSuggestion(rawQuery.trim());
    writeHashSearchQuery(rawQuery.trim());
  }

  document.getElementById('expand-all').addEventListener('click', function(){
    if (input.value) {
      visibleRows().forEach(function(row){
        if (row.tagName === 'DETAILS') row.setAttribute('open', '');
      });
      return;
    }
    props.forEach(function(p){ p.setAttribute('open',''); });
  });
  document.getElementById('collapse-all').addEventListener('click', function(){
    if (input.value) {
      applySearch(input.value);
      return;
    }
    props.forEach(function(p){ p.removeAttribute('open'); });
  });
  input.addEventListener('input', function(){
    applySearch(this.value);
  });
  document.addEventListener('keydown', function(e){
    if (e.key === 'ArrowDown' && document.activeElement === input) {
      if (completionCandidates.length) {
        e.preventDefault();
        completionIndex = completionIndex < 0 ? 0 : (completionIndex + 1) % completionCandidates.length;
        updateGhostSuggestion(input.value);
      }
      return;
    }
    if (e.key === 'ArrowUp' && document.activeElement === input) {
      if (completionCandidates.length) {
        e.preventDefault();
        completionIndex = completionIndex < 0 ? completionCandidates.length - 1 : (completionIndex - 1 + completionCandidates.length) % completionCandidates.length;
        updateGhostSuggestion(input.value);
      }
      return;
    }
    if ((e.key === 'Tab' || e.key === 'ArrowRight') && document.activeElement === input) {
      var caretAtEnd = input.selectionStart === input.value.length && input.selectionEnd === input.value.length;
      if (!caretAtEnd) {
        return;
      }
      var completion = selectedCompletion() || bestCompletionForQuery(input.value);
      if (completion) {
        e.preventDefault();
        input.value = completion;
        applySearch(completion);
      }
      return;
    }
    if (e.key === '.' && document.activeElement === input && !e.ctrlKey && !e.metaKey && !e.altKey) {
      var pathLikeQuery = isPathLikeQuery(input.value, currentSuggestions());
      if (pathLikeQuery) {
        if (input.selectionStart === input.value.length && input.selectionEnd === input.value.length) {
          e.preventDefault();
          var dotAdvance = dotAdvanceForPathSearch(input.value, currentSuggestions());
          if (dotAdvance) {
            input.value = dotAdvance;
            applySearch(dotAdvance);
          }
        }
        return;
      }
    }
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && document.activeElement !== input) {
      e.preventDefault();
      input.focus();
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      if (input.value) {
        var trimmed = trimPathSearch(input.value);
        if (trimmed !== input.value) {
          input.value = trimmed;
          applySearch(trimmed);
        } else {
          input.value = '';
          applySearch('');
          input.blur();
        }
        return;
      }
      var hadOpen = false;
      visibleRows().forEach(function(row){
        if (row.tagName === 'DETAILS' && row.hasAttribute('open')) {
          hadOpen = true;
          row.removeAttribute('open');
        }
      });
      if (hadOpen) {
        return;
      }
      if (document.activeElement && document.activeElement !== document.body) {
        document.activeElement.blur();
        return;
      }
      location.href = document.body.dataset.basePath + '/';
    }
  });
  (function(){
    clearSearchState();
    var query = readHashSearchQuery();
    if (!query) return;
    input.value = query;
    applySearch(query);
  })();
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
