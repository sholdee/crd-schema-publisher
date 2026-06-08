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

	"github.com/sholdee/crd-schema-publisher/jsonschema"
	"github.com/sholdee/crd-schema-publisher/theme"
)

const (
	metadataDirName   = "_meta"
	kindsManifestName = "kinds.json"

	longConstraintValueThreshold = 180
	constraintPreviewLength      = 120
)

// SchemaNode represents a JSON Schema node for rendering.
type SchemaNode struct {
	Type                 interface{}            `json:"type,omitempty"`
	Ref                  string                 `json:"$ref,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Properties           map[string]*SchemaNode `json:"properties,omitempty"`
	Items                *SchemaNode            `json:"items,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Definitions          map[string]*SchemaNode `json:"definitions,omitempty"`
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
	if n.Type == nil {
		if typ := n.displayOneOfType(); typ != "" {
			return typ
		}
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

func (n *SchemaNode) displayOneOfType() string {
	if len(n.OneOf) == 0 {
		return ""
	}

	types := jsonschema.UniqueNonNullBranchTypes(n.OneOf, func(node *SchemaNode) string {
		return node.DisplayType()
	})
	if len(types) == 0 {
		return "null"
	}
	return strings.Join(types, " | ")
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

type RenderConstraint struct {
	Label   string
	Value   string
	Text    string
	Preview string
	Long    bool
}

type schemaResolver struct {
	definitions map[string]*SchemaNode
}

func newSchemaResolver(root *SchemaNode) schemaResolver {
	if root == nil {
		return schemaResolver{}
	}
	return schemaResolver{definitions: root.Definitions}
}

func (r schemaResolver) resolve(node *SchemaNode, seen map[string]struct{}) (*SchemaNode, map[string]struct{}) {
	if node == nil {
		return nil, seen
	}

	resolved, nextSeen := r.resolveLocalRef(node, seen)
	resolved, nextSeen = r.resolveNullableComposition(resolved, nextSeen)
	return r.resolveArrayItems(resolved, nextSeen)
}

func (r schemaResolver) resolveLocalRef(node *SchemaNode, seen map[string]struct{}) (*SchemaNode, map[string]struct{}) {
	refName, ok := localDefinitionRef(node.Ref)
	if !ok {
		return node, seen
	}

	target := r.definitions[refName]
	if target == nil {
		return node, seen
	}

	if _, repeated := seen[refName]; repeated {
		return mergeSchemaNode(leafSchemaNode(target), node), seen
	}

	nextSeen := copySeenWith(seen, refName)
	resolved, targetSeen := r.resolve(target, nextSeen)
	return mergeSchemaNode(resolved, node), targetSeen
}

func (r schemaResolver) resolveArrayItems(node *SchemaNode, seen map[string]struct{}) (*SchemaNode, map[string]struct{}) {
	if node == nil || node.Items == nil {
		return node, seen
	}

	items, itemSeen := r.resolve(node.Items, seen)
	if items == node.Items {
		return node, itemSeen
	}

	cloned := cloneSchemaNode(node)
	cloned.Items = items
	return cloned, itemSeen
}

func (r schemaResolver) resolveNullableComposition(node *SchemaNode, seen map[string]struct{}) (*SchemaNode, map[string]struct{}) {
	if node == nil {
		return node, seen
	}
	if branch := singleNonNullCompositionBranch(node.AnyOf); branch != nil {
		resolved, branchSeen := r.resolve(branch, seen)
		return mergeSchemaNode(resolved, node), branchSeen
	}
	if branch := singleNonNullCompositionBranch(node.OneOf); branch != nil {
		resolved, branchSeen := r.resolve(branch, seen)
		return mergeSchemaNode(resolved, node), branchSeen
	}
	return node, seen
}

func singleNonNullCompositionBranch(branches []*SchemaNode) *SchemaNode {
	if len(branches) == 0 {
		return nil
	}

	var nonNull *SchemaNode
	hasNull := false
	for _, branch := range branches {
		if branch == nil {
			continue
		}
		if branch.resolveType() == "null" {
			hasNull = true
			continue
		}
		if nonNull != nil {
			return nil
		}
		nonNull = branch
	}
	if !hasNull {
		return nil
	}
	return nonNull
}

func localDefinitionRef(ref string) (string, bool) {
	name, found := strings.CutPrefix(ref, "#/definitions/")
	if !found || name == "" {
		return "", false
	}
	name = strings.ReplaceAll(name, "~1", "/")
	name = strings.ReplaceAll(name, "~0", "~")
	return name, true
}

func copySeen(seen map[string]struct{}) map[string]struct{} {
	copied := make(map[string]struct{}, len(seen))
	for name := range seen {
		copied[name] = struct{}{}
	}
	return copied
}

func copySeenWith(seen map[string]struct{}, name string) map[string]struct{} {
	copied := copySeen(seen)
	copied[name] = struct{}{}
	return copied
}

func cloneSchemaNode(node *SchemaNode) *SchemaNode {
	if node == nil {
		return nil
	}
	cloned := *node
	return &cloned
}

func leafSchemaNode(node *SchemaNode) *SchemaNode {
	leaf := cloneSchemaNode(node)
	leaf.Properties = nil
	leaf.Items = nil
	return leaf
}

func mergeSchemaNode(base, overlay *SchemaNode) *SchemaNode {
	merged := cloneSchemaNode(base)
	if merged == nil {
		merged = &SchemaNode{}
	}

	mergeSchemaShape(merged, overlay)
	mergeSchemaComposition(merged, overlay)
	mergeSchemaValidation(merged, overlay)
	merged.Ref = ""
	return merged
}

func mergeSchemaShape(merged, overlay *SchemaNode) {
	if overlay.Type != nil {
		merged.Type = overlay.Type
	}
	if overlay.Description != "" {
		merged.Description = overlay.Description
	}
	if len(overlay.Properties) > 0 {
		merged.Properties = overlay.Properties
	}
	if overlay.Items != nil {
		merged.Items = overlay.Items
	}
	if len(overlay.Required) > 0 {
		merged.Required = overlay.Required
	}
	if len(overlay.Definitions) > 0 {
		merged.Definitions = overlay.Definitions
	}
}

func mergeSchemaComposition(merged, overlay *SchemaNode) {
	if len(overlay.Enum) > 0 {
		merged.Enum = overlay.Enum
	}
	if len(overlay.OneOf) > 0 {
		merged.OneOf = overlay.OneOf
	}
	if len(overlay.AnyOf) > 0 {
		merged.AnyOf = overlay.AnyOf
	}
	if len(overlay.AllOf) > 0 {
		merged.AllOf = overlay.AllOf
	}
	if overlay.Format != "" {
		merged.Format = overlay.Format
	}
	if overlay.Pattern != "" {
		merged.Pattern = overlay.Pattern
	}
}

func mergeSchemaValidation(merged, overlay *SchemaNode) {
	if overlay.Minimum != nil {
		merged.Minimum = overlay.Minimum
	}
	if overlay.Maximum != nil {
		merged.Maximum = overlay.Maximum
	}
	if overlay.MinLength != nil {
		merged.MinLength = overlay.MinLength
	}
	if overlay.MaxLength != nil {
		merged.MaxLength = overlay.MaxLength
	}
	if overlay.MinItems != nil {
		merged.MinItems = overlay.MinItems
	}
	if overlay.MaxItems != nil {
		merged.MaxItems = overlay.MaxItems
	}
	if overlay.Default != nil {
		merged.Default = overlay.Default
	}
	if overlay.AdditionalProperties != nil {
		merged.AdditionalProperties = overlay.AdditionalProperties
	}
}

// Expandable returns true when the property renders as an expandable details row.
func (p RenderProperty) Expandable() bool {
	return len(p.Children) > 0
}

// IsRequired returns true if the given property name is in this node's required list.
func (n *SchemaNode) IsRequired(name string) bool {
	return jsonschema.IsRequired(jsonschema.RequiredSet(n.Required), name)
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
	cs := n.localConstraints()
	for _, branch := range n.OneOf {
		branchType := branch.DisplayType()
		if branchType == "null" {
			continue
		}
		for _, c := range branch.flattenedConstraints() {
			cs = append(cs, branchType+" "+c)
		}
	}
	return cs
}

func (n *SchemaNode) RenderConstraints() []RenderConstraint {
	rawConstraints := n.Constraints()
	constraints := make([]RenderConstraint, 0, len(rawConstraints))
	for _, raw := range rawConstraints {
		constraints = append(constraints, newRenderConstraint(raw))
	}
	return constraints
}

func newRenderConstraint(raw string) RenderConstraint {
	label, value, ok := strings.Cut(raw, ": ")
	if !ok {
		label = ""
		value = raw
	}
	long := len([]rune(value)) > longConstraintValueThreshold
	preview := value
	if long {
		preview = truncateRunes(value, constraintPreviewLength) + "..."
	}
	return RenderConstraint{
		Label:   label,
		Value:   value,
		Text:    raw,
		Preview: preview,
		Long:    long,
	}
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func (n *SchemaNode) flattenedConstraints() []string {
	cs := n.localConstraints()
	for _, sub := range n.AllOf {
		cs = append(cs, sub.flattenedConstraints()...)
	}
	return cs
}

func (n *SchemaNode) localConstraints() []string {
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
	APIVersion     string
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
		APIVersion:     renderAPIVersion(group, version),
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
	return buildRenderPropertiesWithResolver(newSchemaResolver(node), node, parentPath, parentRowPath, arraySuffix, map[string]struct{}{})
}

func buildRenderPropertiesWithResolver(
	resolver schemaResolver,
	node *SchemaNode,
	parentPath,
	parentRowPath,
	arraySuffix string,
	seen map[string]struct{},
) []RenderProperty {
	node, parentSeen := resolver.resolve(node, seen)
	if node == nil {
		return nil
	}

	entries := node.SortedProperties()
	props := make([]RenderProperty, 0, len(entries))
	for _, entry := range entries {
		path := joinPropertyPath(parentPath, entry.Name, arraySuffix)
		childNode, childSeen := resolver.resolve(entry.Node, copySeen(parentSeen))
		childArraySuffix := ""
		if childNode.resolveType() == "array" {
			childArraySuffix = "[]"
		}

		prop := RenderProperty{
			Name:       entry.Name,
			Path:       path,
			PathKey:    buildPathSearchKey(path),
			ParentPath: parentRowPath,
			SearchText: buildSearchText(childNode),
			Required:   node.IsRequired(entry.Name),
			Node:       childNode,
		}
		if childNode.HasChildren() {
			nestedNode := childNode
			if childNode.Items != nil && len(childNode.Items.Properties) > 0 {
				nestedNode = childNode.Items
			}
			prop.Children = buildRenderPropertiesWithResolver(resolver, nestedNode, path+childArraySuffix, path, "", childSeen)
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

func renderAPIVersion(group, version string) string {
	if group == "" || group == "core" {
		return version
	}
	if version == "" {
		return group
	}
	return group + "/" + version
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
  .schema-title {
    font-size: 1.75rem; font-weight: 700; letter-spacing: -0.02em;
    margin-bottom: 0.25rem;
  }
  .schema-title-group {
    color: var(--fg-muted); font-size: 0.95rem; margin-bottom: 1.25rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
  }
  .back-link { font-size: 0.875rem; display: flex; align-items: center; gap: 0.4rem; }
  .back-link kbd {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    font-size: 0.75rem; color: var(--fg-muted); background: var(--bg-surface);
    border: 1px solid var(--border); border-radius: 4px;
    padding: 0.1rem 0.35rem; line-height: 1;
  }
  .yaml-block {
    background: var(--surface-background); border: 1px solid var(--border);
    border-radius: 6px; padding: 0.75rem 1rem; margin-bottom: 1.5rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    font-size: 0.875rem; white-space: pre; overflow-x: auto; color: var(--fg);
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
  .search-ghost {
    grid-area: 1 / 1;
    position: absolute; inset: 0;
    padding: 0 2.75rem 0 1rem; line-height: inherit;
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
    font-size: 0.875rem; padding: 0.2rem 0; transition: color 0.15s;
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
    font-size: 0.85rem; background: var(--surface-background); border-radius: 6px;
    list-style: none; display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;
    transition: background 0.15s;
  }
  .prop.search-match > summary,
  .prop-leaf.search-match { background: var(--surface-accent-background); }
  .prop > summary::-webkit-details-marker { display: none; }
  .prop > summary::before { content: "\25B8"; color: var(--fg); font-size: 0.8rem; }
  .prop[open] > summary::before { content: "\25BE"; color: var(--accent); }
  .prop > summary:hover { background: var(--surface-hover-background); }
  .prop-content { padding: 0.5rem 0.75rem 0.75rem; padding-left: 1.5rem; min-width: 0; }
  .prop-leaf {
    padding: 0.5rem 0.75rem;
    font-size: 0.85rem; display: flex; align-items: flex-start; gap: 0.5rem; flex-wrap: wrap;
    border: 1px solid var(--border); border-radius: 6px;
    margin-bottom: 0.35rem; background: var(--surface-background);
  }
  .prop-leaf .prop-name { min-width: 0; }
  .prop-name {
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    color: var(--accent); font-weight: 600; min-width: 0;
    overflow-wrap: anywhere; white-space: normal;
  }
  .type-badge {
    background: var(--accent-dim); color: var(--accent);
    font-size: 0.75rem; font-weight: 700; padding: 0.15rem 0.5rem;
    border-radius: 8px; white-space: nowrap;
  }
  .required-badge {
    background: var(--required-bg); color: var(--required-fg);
    font-size: 0.75rem; font-weight: 700; padding: 0.15rem 0.5rem;
    border-radius: 8px; white-space: nowrap;
  }
  .prop-desc { color: var(--fg-muted); font-size: 0.9rem; margin-top: 0.25rem; overflow-wrap: anywhere; }
  .prop-constraints {
    color: var(--fg-muted); font-size: 0.875rem; margin-top: 0.2rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
  }
  .prop-children { margin-top: 0.5rem; }
  .leaf-desc { color: var(--fg-muted); font-size: 0.9rem; flex: 1 1 12rem; min-width: min(100%, 12rem); overflow-wrap: anywhere; }
  .leaf-constraints {
    color: var(--fg-muted); font-size: 0.875rem; margin-top: 0.15rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
  }
  .schema-constraint { max-width: 100%; min-width: 0; }
  .schema-constraint code {
    white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word;
  }
  .schema-constraint-label { font-weight: 700; color: var(--fg-muted); }
  .schema-constraint-value { color: var(--fg-muted); }
  .schema-constraint-long > summary {
    cursor: pointer; display: flex; align-items: baseline; gap: 0.35rem;
    min-width: 0;
  }
  .schema-constraint-long > summary code { min-width: 0; }
  .schema-constraint-long[open] .schema-constraint-preview { display: none; }
  .schema-constraint-full {
    display: block; margin-top: 0.25rem; padding-left: 0.75rem;
  }
  @media (max-width: 640px) {
    .leaf-desc,
    .leaf-constraints {
      flex: 0 0 100%;
    }
    .prop-content {
      padding-left: 0.75rem;
    }
  }
</style>
` + theme.HeadScript + `
</head>
<body data-base-path="{{.BasePath}}">
<a class="skip-nav" href="#search">Skip to search</a>
<main id="main">
<nav class="nav-row" aria-label="Page navigation">
  <a href="{{.BasePath}}/" class="back-link">← Back to index <kbd>Esc</kbd></a>
  ` + theme.ThemeToggleButton + `
</nav>
<header>
  <h1 class="schema-title">{{.Kind}}</h1>
  <p class="schema-title-group">{{.Group}} / {{.Version}}</p>
</header>
<div class="yaml-block">apiVersion: {{.APIVersion}}
kind: {{.Kind}}
metadata:
  name: example</div>
<div class="search-row">
  <div class="search-input-wrap">
    <input type="search" class="search-box" placeholder="Search schema fields...  ` + theme.SearchHintText + `" id="search" autocomplete="off" spellcheck="false" aria-label="Search schema fields" aria-controls="properties" aria-describedby="search-status">
    <div class="search-ghost" id="search-ghost" aria-hidden="true"><span class="search-ghost-prefix" id="search-ghost-prefix"></span><span class="search-ghost-suffix" id="search-ghost-suffix"></span></div>
    <button type="button" class="search-clear" id="search-clear" aria-label="Clear search" title="Clear search" hidden></button>
  </div>
  <div class="search-status" id="search-status" data-empty-message="Tip: use {{.SearchPathHint}} for path-only search" role="status" aria-live="polite" aria-atomic="true"></div>
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
{{- define "constraint"}}
{{- if .Long}}
<details class="schema-constraint schema-constraint-long" title="{{.Text}}">
  <summary>{{if .Label}}<span class="schema-constraint-label">{{.Label}}:</span>{{end}} <code class="schema-constraint-value schema-constraint-preview">{{.Preview}}</code></summary>
  <code class="schema-constraint-value schema-constraint-full">{{.Value}}</code>
</details>
{{- else}}
<div class="schema-constraint" title="{{.Text}}">{{if .Label}}<span class="schema-constraint-label">{{.Label}}:</span>{{end}} <code class="schema-constraint-value">{{.Value}}</code></div>
{{- end}}
{{- end}}
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
  {{- range .Node.RenderConstraints}}<div class="prop-constraints">{{template "constraint" .}}</div>{{end}}
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
    {{- range .Node.RenderConstraints}}<div class="leaf-constraints">{{template "constraint" .}}</div>{{end}}
  </div>
</div>
{{- end}}
{{- end}}
<div id="properties">
{{- range .Properties}}{{template "property" .}}{{end}}
</div>
<p class="no-results" id="no-results" data-no-results-message="No matches. Try {{.SearchPathHint}} for an exact path">No matches. Try {{.SearchPathHint}} for an exact path</p>
</main>
` + theme.ToastDiv + `
` + theme.FooterHTML + `
` + theme.BackToTopButton + `
<script src="{{.BasePath}}/` + theme.SchemaSearchAssetName + `"></script>
<script>
` + theme.SearchHashStateJS + `
` + theme.BackToTopJS + `
` + theme.CopyToastJS + `
` + theme.SchemaPageJS + `
` + theme.ThemeToggleJS + `
</script>
</body>
</html>`

// resolveType extracts the non-null type from the Type field.
func (n *SchemaNode) resolveType() string {
	return jsonschema.NonNullType(n.Type)
}
