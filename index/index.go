package index

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sholdee/crd-schema-publisher/schemametadata"
	"github.com/sholdee/crd-schema-publisher/theme"
)

type schemaEntry struct {
	Name     string
	Path     string
	HTMLPath string
}

type groupData struct {
	Name    string
	Schemas []schemaEntry
}

type sourceSectionData struct {
	Source        string
	Label         string
	OpenByDefault bool
	Groups        []groupData
	Count         int
}

type indexData struct {
	Sources         []sourceSectionData
	Groups          []groupData
	GroupedBySource bool
	GroupCount      int
	TotalCount      int
	UpdatedAt       string
	BasePath        string
}

const sourceUnknown = "unknown"

var orderedSources = []string{
	string(schemametadata.SchemaSourceCRD),
	string(schemametadata.SchemaSourceBuiltin),
	string(schemametadata.SchemaSourceKustomize),
	sourceUnknown,
}

var indexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kubernetes CRD Schemas</title>
<link rel="icon" type="image/svg+xml" href="{{.BasePath}}/favicon.svg">
<style>` + theme.CSSVars + theme.CSSBase + `
  header { margin-bottom: 2rem; }
  .title-row { display: flex; align-items: center; justify-content: space-between; margin-bottom: 0.25rem; }
  .title-group { display: flex; align-items: center; gap: 0.6rem; }
  .title-icon { width: 28px; height: 28px; flex-shrink: 0; }
  h1 { font-size: 1.6rem; font-weight: 700; letter-spacing: -0.02em; }
  .subtitle { color: var(--fg-muted); font-size: 0.9rem; margin-bottom: 1.5rem; }
  .stats {
    display: flex; gap: 1.5rem; margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }
  .stat { font-size: 0.9rem; color: var(--fg-muted); }
  .stat strong { color: var(--stat-fg); font-size: 1.1rem; font-weight: 700; margin-right: 0.3rem; }
` + theme.SearchCSS + `
  .search-input-wrap { margin-bottom: 1.5rem; }
  .toolbar {
    display: flex; justify-content: flex-end; margin-bottom: 0.75rem;
  }
  .toolbar button {
    background: none; border: none; color: var(--fg-muted); cursor: pointer;
    font-size: 0.875rem; padding: 0.2rem 0;
    transition: color 0.15s;
  }
  .toolbar button:hover { color: var(--accent); }
  .usage-section { margin-bottom: 1.5rem; }
  .usage-section details { border: 1px solid var(--border); border-radius: 6px; }
  .usage-section summary {
    padding: 0.65rem 1rem; cursor: pointer; font-weight: 600;
    font-size: 0.9rem; color: var(--fg-muted);
    background: var(--surface-background); border-radius: 6px;
    list-style: none;
  }
  .usage-section summary::-webkit-details-marker { display: none; }
  .usage-section summary::before { content: "▸ "; color: var(--fg); transition: transform 0.2s; }
  .usage-section details[open] summary::before { content: "▾ "; color: var(--accent); }
  .usage-section summary:hover { color: var(--fg); }
  .usage-content {
    padding: 1rem; font-size: 0.9rem;
    border-top: 1px solid var(--border);
  }
  .usage-content p { margin-bottom: 0.5rem; color: var(--fg-muted); }
  .usage-content code {
    display: block; background: var(--bg); border: 1px solid var(--border);
    border-radius: 4px; padding: 0.75rem 1rem; font-size: 0.875rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    overflow-x: auto; white-space: pre; color: var(--fg);
  }
  .group {
    border: 1px solid var(--border); border-radius: 6px;
    margin-bottom: 0.5rem; transition: border-color 0.2s;
  }
  .group[open] { border-color: var(--border-active); border-left-width: 2px; }
  .group summary {
    padding: 0.7rem 1rem; cursor: pointer; font-weight: 600;
    font-size: 0.9rem; background: var(--surface-background); border-radius: 6px;
    list-style: none; display: flex; align-items: center; gap: 0.5rem;
    transition: background 0.15s;
  }
  .group summary::-webkit-details-marker { display: none; }
  .group summary::before { content: "▸"; color: var(--fg); font-size: 0.875rem; transition: transform 0.15s; }
  .group[open] summary::before { content: "▾"; color: var(--accent); }
  .group summary:hover { background: var(--surface-hover-background); }
  .group summary:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 6px; }
  .usage-section summary:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 6px; }
  .source-section { margin-bottom: 1rem; }
  .source-section > summary {
    padding: 0.7rem 0.2rem; cursor: pointer; font-weight: 700;
    font-size: 0.95rem; list-style: none; display: flex; align-items: center; gap: 0.5rem;
    border-bottom: 1px solid var(--border); color: var(--fg);
  }
  .source-section > summary::-webkit-details-marker { display: none; }
  .source-section > summary::before { content: "▸"; color: var(--fg); font-size: 0.875rem; transition: transform 0.15s; }
  .source-section[open] > summary::before { content: "▾"; color: var(--accent); }
  .source-section > summary:hover { color: var(--accent); }
  .source-section > summary:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 6px; }
  .source-label { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .source-groups { padding-top: 0.6rem; }
  .group-name { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .badge {
    background: var(--accent-dim); color: var(--accent);
    font-size: 0.75rem; font-weight: 700; padding: 0.15rem 0.5rem;
    border-radius: 10px;
  }
  .schemas { padding: 0.4rem 1rem 0.75rem; }
  @media (min-width: 640px) {
    .schemas { columns: 2; column-gap: 1.5rem; }
  }
  .schema-row {
    display: flex; align-items: center; gap: 0.35rem;
    break-inside: avoid;
  }
  .schemas a {
    flex: 0 1 auto; min-width: 0; min-height: 1.5rem; display: inline-flex; align-items: center;
    padding: 0.2rem 0; color: var(--accent);
    text-decoration: none; font-size: 0.875rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    overflow-wrap: anywhere;
  }
  .schemas a:hover { text-decoration: underline; }
  .schema-copy {
    flex: 0 0 auto; border: 0; background: transparent;
    color: var(--fg-muted); cursor: pointer; font-size: 0.75rem;
    font-family: inherit; padding: 0.1rem 0.35rem; border-radius: 4px;
    opacity: 0; transition: background 0.15s, color 0.15s, opacity 0.15s;
  }
  .schema-row:hover .schema-copy,
  .schema-row:focus-within .schema-copy { opacity: 1; }
  .schema-copy:hover,
  .schema-copy:focus-visible { background: var(--accent-dim); color: var(--accent); }
</style>
` + theme.HeadScript + `
</head>
<body data-base-path="{{.BasePath}}">
<a class="skip-nav" href="#search">Skip to search</a>
<main id="main"><header>
  <div class="title-row">
    <div class="title-group">
      <svg class="title-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none" aria-hidden="true" focusable="false">
        <line x1="16" y1="3" x2="28.38" y2="7.96" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round"/>
        <line x1="28.38" y1="7.96" x2="27.02" y2="21.53" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round"/>
        <line x1="27.02" y1="21.53" x2="19.04" y2="28.51" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round"/>
        <line x1="19.04" y1="28.51" x2="12.96" y2="28.51" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round"/>
        <line x1="12.96" y1="28.51" x2="4.98" y2="21.53" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round"/>
        <line x1="4.98" y1="21.53" x2="3.62" y2="7.96" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round"/>
        <line x1="3.62" y1="7.96" x2="16" y2="3" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round"/>
        <circle cx="16" cy="3" r="4" fill="var(--accent)" opacity="0.2"/>
        <circle cx="28.38" cy="7.96" r="4" fill="var(--accent)" opacity="0.2"/>
        <circle cx="27.02" cy="21.53" r="4" fill="var(--accent)" opacity="0.2"/>
        <circle cx="19.04" cy="28.51" r="4" fill="var(--accent)" opacity="0.2"/>
        <circle cx="12.96" cy="28.51" r="4" fill="var(--accent)" opacity="0.2"/>
        <circle cx="4.98" cy="21.53" r="4" fill="var(--accent)" opacity="0.2"/>
        <circle cx="3.62" cy="7.96" r="4" fill="var(--accent)" opacity="0.2"/>
        <circle cx="16" cy="3" r="2.5" fill="var(--fg)"/>
        <circle cx="28.38" cy="7.96" r="2.5" fill="var(--fg)"/>
        <circle cx="27.02" cy="21.53" r="2.5" fill="var(--fg)"/>
        <circle cx="19.04" cy="28.51" r="2.5" fill="var(--fg)"/>
        <circle cx="12.96" cy="28.51" r="2.5" fill="var(--fg)"/>
        <circle cx="4.98" cy="21.53" r="2.5" fill="var(--fg)"/>
        <circle cx="3.62" cy="7.96" r="2.5" fill="var(--fg)"/>
      </svg>
      <h1>Kubernetes CRD Schemas</h1>
    </div>
    ` + theme.ThemeToggleButton + `
  </div>
  <p class="subtitle">JSON schemas extracted from live CustomResourceDefinitions</p>
  <div class="stats">
    <div class="stat"><strong id="stat-groups">{{.GroupCount}}</strong> API groups</div>
    <div class="stat"><strong id="stat-schemas">{{.TotalCount}}</strong> schemas</div>
    <div class="stat">Updated <strong>{{.UpdatedAt}}</strong></div>
  </div>
</header>
<div class="usage-section">
<details>
<summary>Usage — yaml-language-server</summary>
<div class="usage-content">
<p>Add a modeline to any YAML file. Works in VS Code, Neovim, Helix, and any editor with yaml-language-server:</p>
<code>
# yaml-language-server: $schema=https://YOUR_DOMAIN/cert-manager.io/certificate_v1.json
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example
</code>
<p style="margin-top:0.75rem;">Or configure schemas globally in VS Code settings:</p>
<code>
// .vscode/settings.json
"yaml.schemas": {
  "https://YOUR_DOMAIN/cert-manager.io/certificate_v1.json": ["**/certificates/*.yaml"]
}
</code>
</div>
</details>
</div>
<div class="search-input-wrap">
  <input type="search" class="search-box" placeholder="Search groups and schemas...  ` + theme.SearchHintText + `" id="search" autocomplete="off" spellcheck="false" aria-label="Search groups and schemas" aria-controls="groups" aria-describedby="search-status">
  <button type="button" class="search-clear" id="search-clear" aria-label="Clear search" title="Clear search" hidden></button>
</div>
<div class="visually-hidden" id="search-status" role="status" aria-live="polite" aria-atomic="true"></div>
<div class="toolbar"><button id="toggle-all">Expand all</button></div>
<div id="groups">
{{if .GroupedBySource}}
{{range .Sources}}
{{$source := .Source}}
<details class="source-section" data-source="{{.Source}}" data-source-label="{{.Label}}" data-default-open="{{.OpenByDefault}}"{{if .OpenByDefault}} open{{end}}>
<summary><span class="source-label">{{.Label}}</span> <span class="badge source-badge">{{.Count}}</span></summary>
<div class="source-groups">
{{range .Groups}}
<details class="group" data-source="{{$source}}" data-group="{{.Name}}">
<summary><span class="group-name">{{.Name}}</span> <span class="badge">{{len .Schemas}}</span></summary>
<div class="schemas">
{{range .Schemas}}<div class="schema-row" data-schema="{{.Name}}"><a href="{{$.BasePath}}/{{.HTMLPath}}" data-url="{{$.BasePath}}/{{.Path}}">{{.Name}}</a><button type="button" class="schema-copy" data-url="{{$.BasePath}}/{{.Path}}" aria-label="Copy schema URL for {{.Name}}" title="Copy schema URL">copy URL</button></div>
{{end}}</div>
</details>
{{end}}
</div>
</details>
{{end}}
{{else}}
{{range .Groups}}
<details class="group" data-group="{{.Name}}">
<summary><span class="group-name">{{.Name}}</span> <span class="badge">{{len .Schemas}}</span></summary>
<div class="schemas">
{{range .Schemas}}<div class="schema-row" data-schema="{{.Name}}"><a href="{{$.BasePath}}/{{.HTMLPath}}" data-url="{{$.BasePath}}/{{.Path}}">{{.Name}}</a><button type="button" class="schema-copy" data-url="{{$.BasePath}}/{{.Path}}" aria-label="Copy schema URL for {{.Name}}" title="Copy schema URL">copy URL</button></div>
{{end}}</div>
</details>
{{end}}
{{end}}
</div>
<p class="no-results" id="no-results">No matching sources, groups, or schemas.</p>
</main>
` + theme.ToastDiv + `
` + theme.FooterHTML + `
` + theme.BackToTopButton + `
<script>
` + theme.SearchHashStateJS + `
` + theme.BackToTopJS + `
` + theme.CopyToastJS + `
` + theme.IndexPageJS + `
` + theme.ThemeToggleJS + `
</script>
</body>
</html>`

func Generate(outputDir, basePath string) error {
	sources, groupCount, totalCount, err := collectSourceSections(outputDir)
	if err != nil {
		return err
	}

	if err := theme.WriteFavicon(outputDir); err != nil {
		return fmt.Errorf("writing favicon: %w", err)
	}

	tmpl, err := template.New("index").Parse(indexTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	f, err := os.Create(filepath.Join(outputDir, "index.html"))
	if err != nil {
		return fmt.Errorf("creating index.html: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, indexData{
		Sources:         sources,
		Groups:          flatGroupsForSingleSource(sources),
		GroupedBySource: len(sources) > 1,
		GroupCount:      groupCount,
		TotalCount:      totalCount,
		UpdatedAt:       time.Now().UTC().Format("2006-01-02 15:04 UTC"),
		BasePath:        basePath,
	}); err != nil {
		return err
	}
	return f.Close()
}

func flatGroupsForSingleSource(sources []sourceSectionData) []groupData {
	if len(sources) != 1 {
		return nil
	}
	return sources[0].Groups
}

func collectSourceSections(outputDir string) ([]sourceSectionData, int, int, error) {
	metadata := loadSchemaMetadata(outputDir)
	sourceGroups := map[string]map[string][]schemaEntry{}
	groupNames := map[string]struct{}{}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("reading output dir: %w", err)
	}

	totalCount := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "master-standalone" || entry.Name() == schemametadata.MetadataDirName {
			continue
		}
		groupName := entry.Name()
		groupDir := filepath.Join(outputDir, groupName)
		files, err := os.ReadDir(groupDir)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("reading group dir %s: %w", groupName, err)
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			jsonPath := filepath.ToSlash(filepath.Join(groupName, f.Name()))
			htmlPath := jsonPath
			htmlFile := strings.TrimSuffix(f.Name(), ".json") + ".html"
			if _, err := os.Stat(filepath.Join(groupDir, htmlFile)); err == nil {
				htmlPath = filepath.ToSlash(filepath.Join(groupName, htmlFile))
			}
			source := classifySource(metadata[jsonPath])
			if sourceGroups[source] == nil {
				sourceGroups[source] = map[string][]schemaEntry{}
			}
			sourceGroups[source][groupName] = append(sourceGroups[source][groupName], schemaEntry{
				Name:     f.Name(),
				Path:     jsonPath,
				HTMLPath: htmlPath,
			})
			groupNames[groupName] = struct{}{}
			totalCount++
		}
	}

	sections := make([]sourceSectionData, 0, len(sourceGroups))
	for _, source := range orderedSources {
		groups := sourceGroups[source]
		if len(groups) == 0 {
			continue
		}
		section := sourceSectionData{
			Source: source,
			Label:  sourceLabel(source),
			Groups: sortedGroupData(groups),
		}
		for _, group := range section.Groups {
			section.Count += len(group.Schemas)
		}
		sections = append(sections, section)
	}
	applyDefaultOpenSources(sections)

	return sections, len(groupNames), totalCount, nil
}

func loadSchemaMetadata(outputDir string) map[string]schemametadata.SchemaMetadataEntry {
	manifestPath := filepath.Join(outputDir, schemametadata.MetadataDirName, schemametadata.SchemaMetadataManifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	var metadata map[string]schemametadata.SchemaMetadataEntry
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil
	}
	return metadata
}

func classifySource(entry schemametadata.SchemaMetadataEntry) string {
	switch entry.Source {
	case "", schemametadata.SchemaSourceCRD:
		return string(schemametadata.SchemaSourceCRD)
	case schemametadata.SchemaSourceBuiltin:
		return string(schemametadata.SchemaSourceBuiltin)
	case schemametadata.SchemaSourceKustomize:
		return string(schemametadata.SchemaSourceKustomize)
	default:
		return sourceUnknown
	}
}

func sourceLabel(source string) string {
	switch source {
	case string(schemametadata.SchemaSourceCRD):
		return "Custom Resources"
	case string(schemametadata.SchemaSourceBuiltin):
		return "Kubernetes Built-ins"
	case string(schemametadata.SchemaSourceKustomize):
		return "Kustomize"
	default:
		return "Unknown"
	}
}

func sortedGroupData(groups map[string][]schemaEntry) []groupData {
	sortedGroups := make([]groupData, 0, len(groups))
	for name, schemas := range groups {
		sort.Slice(schemas, func(i, j int) bool { return schemas[i].Name < schemas[j].Name })
		sortedGroups = append(sortedGroups, groupData{Name: name, Schemas: schemas})
	}
	sort.Slice(sortedGroups, func(i, j int) bool { return sortedGroups[i].Name < sortedGroups[j].Name })
	return sortedGroups
}

func applyDefaultOpenSources(sections []sourceSectionData) {
	if len(sections) == 0 {
		return
	}
	defaultIndex := 0
	for i := range sections {
		if sections[i].Source == string(schemametadata.SchemaSourceCRD) {
			defaultIndex = i
			break
		}
	}
	sections[defaultIndex].OpenByDefault = true
}
