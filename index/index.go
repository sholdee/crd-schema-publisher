package index

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

type indexData struct {
	Groups     []groupData
	GroupCount int
	TotalCount int
	UpdatedAt  string
	BasePath   string
}

const metadataDirName = "_meta"

const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none">
<line x1="16" y1="3" x2="28.38" y2="7.96" stroke="#6bc1fe" stroke-width="1.5" stroke-linecap="round"/>
<line x1="28.38" y1="7.96" x2="27.02" y2="21.53" stroke="#6bc1fe" stroke-width="1.5" stroke-linecap="round"/>
<line x1="27.02" y1="21.53" x2="19.04" y2="28.51" stroke="#6bc1fe" stroke-width="1.5" stroke-linecap="round"/>
<line x1="19.04" y1="28.51" x2="12.96" y2="28.51" stroke="#6bc1fe" stroke-width="1.5" stroke-linecap="round"/>
<line x1="12.96" y1="28.51" x2="4.98" y2="21.53" stroke="#6bc1fe" stroke-width="1.5" stroke-linecap="round"/>
<line x1="4.98" y1="21.53" x2="3.62" y2="7.96" stroke="#6bc1fe" stroke-width="1.5" stroke-linecap="round"/>
<line x1="3.62" y1="7.96" x2="16" y2="3" stroke="#6bc1fe" stroke-width="1.5" stroke-linecap="round"/>
<circle cx="16" cy="3" r="4" fill="#6bc1fe" opacity="0.2"/>
<circle cx="28.38" cy="7.96" r="4" fill="#6bc1fe" opacity="0.2"/>
<circle cx="27.02" cy="21.53" r="4" fill="#6bc1fe" opacity="0.2"/>
<circle cx="19.04" cy="28.51" r="4" fill="#6bc1fe" opacity="0.2"/>
<circle cx="12.96" cy="28.51" r="4" fill="#6bc1fe" opacity="0.2"/>
<circle cx="4.98" cy="21.53" r="4" fill="#6bc1fe" opacity="0.2"/>
<circle cx="3.62" cy="7.96" r="4" fill="#6bc1fe" opacity="0.2"/>
<circle cx="16" cy="3" r="2.5" fill="#fff"/>
<circle cx="28.38" cy="7.96" r="2.5" fill="#fff"/>
<circle cx="27.02" cy="21.53" r="2.5" fill="#fff"/>
<circle cx="19.04" cy="28.51" r="2.5" fill="#fff"/>
<circle cx="12.96" cy="28.51" r="2.5" fill="#fff"/>
<circle cx="4.98" cy="21.53" r="2.5" fill="#fff"/>
<circle cx="3.62" cy="7.96" r="2.5" fill="#fff"/>
</svg>`

func writeFavicon(outputDir string) error {
	return os.WriteFile(filepath.Join(outputDir, "favicon.svg"), []byte(faviconSVG), 0o644)
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
  .subtitle { color: var(--fg-muted); font-size: 0.85rem; margin-bottom: 1.5rem; }
  .stats {
    display: flex; gap: 1.5rem; margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }
  .stat { font-size: 0.85rem; color: var(--fg-muted); }
  .stat strong { color: var(--stat-fg); font-size: 1.1rem; font-weight: 700; margin-right: 0.3rem; }
  .search-box {
    width: 100%; padding: 0.65rem 1rem; font-size: 0.95rem;
    background: var(--bg-surface); color: var(--fg);
    border: 1px solid var(--border); border-radius: 6px;
    outline: none; transition: border-color 0.2s;
    margin-bottom: 1.5rem;
  }
  .search-box::placeholder { color: var(--fg-muted); }
  .search-box:focus { border-color: var(--accent); }
  .toolbar {
    display: flex; justify-content: flex-end; margin-bottom: 0.75rem;
  }
  .toolbar button {
    background: none; border: none; color: var(--fg-muted); cursor: pointer;
    font-size: 0.8rem; padding: 0.2rem 0;
    transition: color 0.15s;
  }
  .toolbar button:hover { color: var(--accent); }
  .no-results {
    text-align: center; color: var(--fg-muted); padding: 3rem 1rem;
    font-size: 0.95rem; display: none;
  }
  .usage-section { margin-bottom: 1.5rem; }
  .usage-section details { border: 1px solid var(--border); border-radius: 6px; }
  .usage-section summary {
    padding: 0.65rem 1rem; cursor: pointer; font-weight: 600;
    font-size: 0.85rem; color: var(--fg-muted);
    background: var(--bg-surface); border-radius: 6px;
    list-style: none;
  }
  .usage-section summary::-webkit-details-marker { display: none; }
  .usage-section summary::before { content: "▸ "; transition: transform 0.2s; }
  .usage-section details[open] summary::before { content: "▾ "; }
  .usage-section summary:hover { color: var(--fg); }
  .usage-content {
    padding: 1rem; font-size: 0.85rem;
    border-top: 1px solid var(--border);
  }
  .usage-content p { margin-bottom: 0.5rem; color: var(--fg-muted); }
  .usage-content code {
    display: block; background: var(--bg); border: 1px solid var(--border);
    border-radius: 4px; padding: 0.75rem 1rem; font-size: 0.8rem;
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
    font-size: 0.9rem; background: var(--bg-surface); border-radius: 6px;
    list-style: none; display: flex; align-items: center; gap: 0.5rem;
    transition: background 0.15s;
  }
  .group summary::-webkit-details-marker { display: none; }
  .group summary::before { content: "▸"; color: var(--fg-muted); font-size: 0.75rem; transition: transform 0.15s; }
  .group[open] summary::before { content: "▾"; color: var(--accent); }
  .group summary:hover { background: var(--bg-hover); }
  .group summary:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 6px; }
  .usage-section summary:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 6px; }
  .group-name { flex: 1; }
  .badge {
    background: var(--accent-dim); color: var(--accent);
    font-size: 0.7rem; font-weight: 700; padding: 0.15rem 0.5rem;
    border-radius: 10px;
  }
  .schemas { padding: 0.4rem 1rem 0.75rem; }
  @media (min-width: 640px) {
    .schemas { columns: 2; column-gap: 1.5rem; }
  }
  .schemas a {
    display: block; padding: 0.2rem 0; color: var(--accent);
    text-decoration: none; font-size: 0.82rem;
    font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
    break-inside: avoid;
  }
  .schemas a:hover { text-decoration: underline; }
  .schemas a .copy-hint {
    display: none; margin-left: 0.5rem;
    font-size: 0.65rem; color: var(--fg-muted); font-family: inherit;
    padding: 0.1rem 0.4rem; border-radius: 4px;
    transition: background 0.15s, color 0.15s;
  }
  .schemas a:hover .copy-hint { display: inline; }
  .schemas a .copy-hint:hover { background: var(--accent-dim); color: var(--accent); }
  .back-to-top {
    position: fixed; bottom: 1.5rem; right: 1.5rem;
    background: var(--bg-surface); color: var(--fg-muted);
    border: 1px solid var(--border); border-radius: 50%;
    width: 2.5rem; height: 2.5rem; font-size: 1.1rem;
    cursor: pointer; display: none; align-items: center; justify-content: center;
    transition: color 0.2s, border-color 0.2s;
    z-index: 10;
  }
  .back-to-top:hover { color: var(--accent); border-color: var(--accent); }
  .back-to-top.visible { display: flex; }
</style>
` + theme.HeadScript + `
</head>
<body data-base-path="{{.BasePath}}">
` + theme.FlareDiv + `
<header>
  <div class="title-row">
    <div class="title-group">
      <svg class="title-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none">
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
  <p class="subtitle">JSON schemas extracted from live CRD definitions</p>
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
<input type="search" class="search-box" placeholder="Search groups and schemas…  ( / to focus, Esc to clear )" id="search" autocomplete="off" spellcheck="false">
<div class="toolbar"><button id="toggle-all">Expand all</button></div>
<div id="groups">
{{range .Groups}}
<details class="group" data-group="{{.Name}}">
<summary><span class="group-name">{{.Name}}</span> <span class="badge">{{len .Schemas}}</span></summary>
<div class="schemas">
{{range .Schemas}}<a href="{{$.BasePath}}/{{.HTMLPath}}" data-schema="{{.Name}}" data-url="{{$.BasePath}}/{{.Path}}">{{.Name}}<span class="copy-hint">copy URL</span></a>
{{end}}</div>
</details>
{{end}}
</div>
<p class="no-results" id="no-results">No matching groups or schemas.</p>
` + theme.ToastDiv + `
` + theme.FooterHTML + `
<button class="back-to-top" id="back-to-top" title="Back to top" aria-label="Back to top">&#8593;</button>
<script>
(function(){
  var input = document.getElementById('search');
  var groups = document.querySelectorAll('.group');
  var noResults = document.getElementById('no-results');
  var statGroups = document.getElementById('stat-groups');
  var statSchemas = document.getElementById('stat-schemas');
  var totalGroups = groups.length;
  var totalSchemas = document.querySelectorAll('.schemas a').length;

  input.addEventListener('input', function(){
    var q = this.value.toLowerCase().trim();
    var visible = 0;
    groups.forEach(function(g){
      if (!q) {
        g.style.display = '';
        g.removeAttribute('open');
        g.querySelectorAll('.schemas a').forEach(function(a){ a.style.display = ''; });
        visible++;
        return;
      }
      var groupName = g.dataset.group.toLowerCase();
      var links = g.querySelectorAll('.schemas a');
      var groupMatch = groupName.indexOf(q) !== -1;
      var schemaMatch = false;
      links.forEach(function(a){
        if (a.dataset.schema.toLowerCase().indexOf(q) !== -1) {
          a.style.display = '';
          schemaMatch = true;
        } else {
          a.style.display = groupMatch ? '' : 'none';
        }
      });
      if (groupMatch || schemaMatch) {
        g.style.display = '';
        g.setAttribute('open','');
        visible++;
      } else {
        g.style.display = 'none';
        g.removeAttribute('open');
      }
    });
    noResults.style.display = visible ? 'none' : 'block';
    if (!q) {
      statGroups.textContent = totalGroups;
      statSchemas.textContent = totalSchemas;
    } else {
      var visibleSchemas = 0;
      groups.forEach(function(g){
        if (g.style.display === 'none') return;
        g.querySelectorAll('.schemas a').forEach(function(a){
          if (a.style.display !== 'none') visibleSchemas++;
        });
      });
      statGroups.textContent = visible + ' / ' + totalGroups;
      statSchemas.textContent = visibleSchemas + ' / ' + totalSchemas;
    }
    history.replaceState(null, '', q ? '#q=' + encodeURIComponent(q) : location.pathname);
  });

  document.addEventListener('keydown', function(e){
    if (e.key === 'Escape') {
      e.preventDefault();
      if (input.value) {
        input.value = '';
        input.dispatchEvent(new Event('input'));
        input.blur();
      } else {
        var hadOpen = false;
        groups.forEach(function(g){ if (g.hasAttribute('open')) { hadOpen = true; g.removeAttribute('open'); } });
        if (hadOpen) {
          allExpanded = false;
          toggleAll.textContent = 'Expand all';
        } else {
          window.scrollTo({top: 0, behavior: 'smooth'});
        }
        if (document.activeElement && document.activeElement !== document.body) {
          document.activeElement.blur();
        }
      }
    }
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && document.activeElement !== input) {
      e.preventDefault();
      input.focus();
    }
  });

  var toast = document.getElementById('toast');
  var toastTimer;
  document.getElementById('groups').addEventListener('click', function(e){
    if (!e.target.classList.contains('copy-hint')) return;
    e.preventDefault();
    var link = e.target.closest('.schemas a');
    var url = location.origin + link.dataset.url;
    navigator.clipboard.writeText(url).then(function(){
      clearTimeout(toastTimer);
      toast.classList.add('show');
      toastTimer = setTimeout(function(){ toast.classList.remove('show'); }, 1500);
    });
  });

  var toggleAll = document.getElementById('toggle-all');
  var allExpanded = false;
  toggleAll.addEventListener('click', function(){
    allExpanded = !allExpanded;
    groups.forEach(function(g){
      if (g.style.display === 'none') return;
      if (allExpanded) g.setAttribute('open','');
      else g.removeAttribute('open');
    });
    toggleAll.textContent = allExpanded ? 'Collapse all' : 'Expand all';
  });

  (function(){
    var hash = location.hash;
    if (hash.indexOf('#q=') === 0) {
      input.value = decodeURIComponent(hash.slice(3));
      input.dispatchEvent(new Event('input'));
    }
  })();

  var btt = document.getElementById('back-to-top');
  window.addEventListener('scroll', function(){
    btt.classList.toggle('visible', window.scrollY > 300);
  }, {passive: true});
  btt.addEventListener('click', function(){
    window.scrollTo({top: 0, behavior: 'smooth'});
  });

  document.querySelectorAll('.usage-content code').forEach(function(el){
    var base = document.body.dataset.basePath || '';
    el.textContent = el.textContent.replace(/https:\/\/YOUR_DOMAIN/g, location.origin + base);
  });

  // Save view state before navigating to a schema page
  document.getElementById('groups').addEventListener('click', function(e){
    var link = e.target.closest('.schemas a');
    if (!link || e.target.classList.contains('copy-hint')) return;
    var expanded = [];
    groups.forEach(function(g){ if (g.hasAttribute('open')) expanded.push(g.dataset.group); });
    sessionStorage.setItem('indexState', JSON.stringify({
      expanded: expanded,
      scroll: window.scrollY,
      toggleAll: allExpanded,
      search: input.value
    }));
  });

  // Restore view state when returning from a schema page
  var saved = sessionStorage.getItem('indexState');
  if (saved) {
    sessionStorage.removeItem('indexState');
    try {
      var state = JSON.parse(saved);
      if (state.search) {
        input.value = state.search;
        input.dispatchEvent(new Event('input'));
      }
      if (state.expanded && state.expanded.length) {
        groups.forEach(function(g){
          if (state.expanded.indexOf(g.dataset.group) !== -1) g.setAttribute('open','');
        });
      }
      if (state.toggleAll) {
        allExpanded = true;
        toggleAll.textContent = 'Collapse all';
      }
      if (state.scroll) {
        window.scrollTo(0, state.scroll);
      }
    } catch(e) {}
  }
})();

` + theme.ThemeToggleJS + `
</script>
</body>
</html>`

func Generate(outputDir, basePath string) error {
	groups, totalCount, err := collectGroups(outputDir)
	if err != nil {
		return err
	}

	var sortedGroups []groupData
	for name, schemas := range groups {
		sort.Slice(schemas, func(i, j int) bool { return schemas[i].Name < schemas[j].Name })
		sortedGroups = append(sortedGroups, groupData{Name: name, Schemas: schemas})
	}
	sort.Slice(sortedGroups, func(i, j int) bool { return sortedGroups[i].Name < sortedGroups[j].Name })

	if err := writeFavicon(outputDir); err != nil {
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
		Groups:     sortedGroups,
		GroupCount: len(sortedGroups),
		TotalCount: totalCount,
		UpdatedAt:  time.Now().UTC().Format("2006-01-02 15:04 UTC"),
		BasePath:   basePath,
	}); err != nil {
		return err
	}
	return f.Close()
}

func collectGroups(outputDir string) (map[string][]schemaEntry, int, error) {
	groups := map[string][]schemaEntry{}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, 0, fmt.Errorf("reading output dir: %w", err)
	}

	totalCount := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "master-standalone" || entry.Name() == metadataDirName {
			continue
		}
		groupName := entry.Name()
		groupDir := filepath.Join(outputDir, groupName)
		files, err := os.ReadDir(groupDir)
		if err != nil {
			return nil, 0, fmt.Errorf("reading group dir %s: %w", groupName, err)
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			jsonPath := groupName + "/" + f.Name()
			htmlPath := jsonPath
			htmlFile := strings.TrimSuffix(f.Name(), ".json") + ".html"
			if _, err := os.Stat(filepath.Join(groupDir, htmlFile)); err == nil {
				htmlPath = groupName + "/" + htmlFile
			}
			groups[groupName] = append(groups[groupName], schemaEntry{
				Name:     f.Name(),
				Path:     jsonPath,
				HTMLPath: htmlPath,
			})
			totalCount++
		}
	}
	return groups, totalCount, nil
}
