package index

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type schemaEntry struct {
	Name string
	Path string
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
}

const indexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kubernetes CRD Schemas</title>
<style>
  :root {
    --bg: #09090b;
    --bg-surface: rgba(24, 24, 27, 0.6);
    --bg-hover: rgba(24, 24, 27, 0.8);
    --fg: #fafafa;
    --fg-muted: #a1a1aa;
    --accent: #6bc1fe;
    --accent-dim: rgba(107, 193, 254, 0.15);
    --border: rgba(255, 255, 255, 0.1);
    --border-active: #6bc1fe;
    --stat-fg: #fafafa;
    --stripes-dark: repeating-linear-gradient(100deg, #000 0%, #000 7%, transparent 10%, transparent 12%, #000 16%);
    --rainbow: repeating-linear-gradient(100deg, #fff 10%, #fff 16%, #fff 22%, #fff 30%);
  }
  .light {
    --bg: #f5f7fa;
    --bg-surface: #ffffff;
    --bg-hover: #edf0f4;
    --fg: #18181b;
    --fg-muted: #6b7785;
    --accent: #2563b0;
    --accent-dim: rgba(37, 99, 176, 0.08);
    --border: #d8dde4;
    --border-active: #2563b0;
    --stat-fg: #18181b;
    --stripes-dark: none;
    --rainbow: none;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: var(--bg); color: var(--fg);
    max-width: 920px; margin: 0 auto; padding: 2.5rem 1.25rem;
    position: relative; z-index: 1;
    transition: background 0.2s, color 0.2s;
  }
  body::before {
    content: '';
    position: fixed; inset: 0; z-index: -2;
    pointer-events: none;
    background-image:
      radial-gradient(1.5px 1.5px at 31px 47px, rgba(255,255,255,1), transparent),
      radial-gradient(1px 1px at 212px 23px, rgba(255,255,255,0.7), transparent),
      radial-gradient(1.5px 1.5px at 68px 289px, rgba(255,255,255,0.84), transparent),
      radial-gradient(1px 1px at 313px 151px, rgba(255,255,255,0.56), transparent),
      radial-gradient(1px 1px at 157px 371px, rgba(255,255,255,0.6), transparent),
      radial-gradient(2px 2px at 19px 83px, rgba(255,255,255,0.96), transparent),
      radial-gradient(1px 1px at 301px 41px, rgba(255,255,255,0.6), transparent),
      radial-gradient(1.5px 1.5px at 127px 409px, rgba(255,255,255,0.8), transparent),
      radial-gradient(1px 1px at 443px 237px, rgba(255,255,255,0.5), transparent),
      radial-gradient(1.5px 1.5px at 67px 491px, rgba(255,255,255,0.72), transparent),
      radial-gradient(1px 1px at 11px 37px, rgba(255,255,255,0.72), transparent),
      radial-gradient(1.5px 1.5px at 191px 213px, rgba(255,255,255,0.9), transparent),
      radial-gradient(1px 1px at 53px 7px, rgba(255,255,255,0.5), transparent),
      radial-gradient(1px 1px at 271px 103px, rgba(255,255,255,0.64), transparent);
    background-size:
      397px 397px, 397px 397px, 397px 397px, 397px 397px, 397px 397px,
      509px 509px, 509px 509px, 509px 509px, 509px 509px, 509px 509px,
      311px 311px, 311px 311px, 311px 311px, 311px 311px;
    mask-image: linear-gradient(to bottom, black 0%, rgba(0,0,0,0.35) 45%, transparent 80%);
    -webkit-mask-image: linear-gradient(to bottom, black 0%, rgba(0,0,0,0.35) 45%, transparent 80%);
  }
  .flare {
    position: fixed; top: 0; right: 0;
    width: 100vw; height: 450px; z-index: -1;
    pointer-events: none;
    background-image: var(--stripes-dark), var(--rainbow);
    background-size: 300% 200%;
    background-position: 50% 50%;
    filter: opacity(50%) saturate(200%);
    opacity: 0.25;
    mask-image: radial-gradient(ellipse at 100% 0%, black 40%, transparent 70%);
    -webkit-mask-image: radial-gradient(ellipse at 100% 0%, black 40%, transparent 70%);
  }
  .flare::after {
    content: '';
    position: absolute; inset: 0;
    background-image: var(--stripes-dark), var(--rainbow);
    background-size: 200% 100%;
    background-attachment: fixed;
    mix-blend-mode: difference;
  }
  .light body::before, .light .flare { display: none; }
  header { margin-bottom: 2rem; }
  .title-row { display: flex; align-items: center; justify-content: space-between; margin-bottom: 0.25rem; }
  h1 { font-size: 1.6rem; font-weight: 700; letter-spacing: -0.02em; }
  .subtitle { color: var(--fg-muted); font-size: 0.85rem; margin-bottom: 1.5rem; }
  .theme-toggle {
    background: none; border: 1px solid var(--border); border-radius: 6px;
    color: var(--fg-muted); cursor: pointer; padding: 0.35rem 0.6rem; font-size: 0.85rem;
    transition: border-color 0.2s, color 0.2s;
  }
  .theme-toggle:hover { border-color: var(--accent); color: var(--accent); }
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
  }
  .schemas a:hover .copy-hint { display: inline; }
  .copied-toast {
    position: fixed; bottom: 1.5rem; left: 50%; transform: translateX(-50%);
    background: var(--accent); color: #09090b; padding: 0.4rem 1rem;
    border-radius: 6px; font-size: 0.8rem; font-weight: 600;
    opacity: 0; transition: opacity 0.2s;
    pointer-events: none; z-index: 10;
  }
  .copied-toast.show { opacity: 1; }
  footer {
    margin-top: 3rem; padding-top: 1.5rem;
    border-top: 1px solid var(--border);
    text-align: center; font-size: 0.8rem; color: var(--fg-muted);
  }
  footer a { color: var(--accent); text-decoration: none; }
  footer a:hover { text-decoration: underline; }
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
<script>if(localStorage.getItem('theme')==='light')document.documentElement.className='light';</script>
</head>
<body>
<div class="flare"></div>
<header>
  <div class="title-row">
    <h1>Kubernetes CRD Schemas</h1>
    <button class="theme-toggle" onclick="toggleTheme()" title="Toggle light/dark mode">☀/☾</button>
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
{{range .Schemas}}<a href="/{{.Path}}" data-schema="{{.Name}}" data-url="/{{.Path}}">{{.Name}}<span class="copy-hint">copy URL</span></a>
{{end}}</div>
</details>
{{end}}
</div>
<p class="no-results" id="no-results">No matching groups or schemas.</p>
<div class="copied-toast" id="toast">Copied!</div>
<footer>
  Generated by <a href="https://github.com/sholdee/crd-schema-publisher">crd-schema-publisher</a>
</footer>
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

  input.addEventListener('keydown', function(e){
    if (e.key === 'Escape') {
      this.value = '';
      this.dispatchEvent(new Event('input'));
      this.blur();
    }
  });

  document.addEventListener('keydown', function(e){
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
    el.textContent = el.textContent.replace(/https:\/\/YOUR_DOMAIN/g, location.origin);
  });
})();

function toggleTheme(){
  document.documentElement.classList.toggle('light');
  localStorage.setItem('theme', document.documentElement.classList.contains('light') ? 'light' : 'dark');
}
</script>
</body>
</html>`

func Generate(outputDir string) error {
	groups := map[string][]schemaEntry{}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("reading output dir: %w", err)
	}

	totalCount := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "master-standalone" {
			continue
		}
		groupName := entry.Name()
		groupDir := filepath.Join(outputDir, groupName)
		files, err := os.ReadDir(groupDir)
		if err != nil {
			return fmt.Errorf("reading group dir %s: %w", groupName, err)
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			groups[groupName] = append(groups[groupName], schemaEntry{
				Name: f.Name(),
				Path: groupName + "/" + f.Name(),
			})
			totalCount++
		}
	}

	var sortedGroups []groupData
	for name, schemas := range groups {
		sort.Slice(schemas, func(i, j int) bool { return schemas[i].Name < schemas[j].Name })
		sortedGroups = append(sortedGroups, groupData{Name: name, Schemas: schemas})
	}
	sort.Slice(sortedGroups, func(i, j int) bool { return sortedGroups[i].Name < sortedGroups[j].Name })

	tmpl, err := template.New("index").Parse(indexTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	f, err := os.Create(filepath.Join(outputDir, "index.html"))
	if err != nil {
		return fmt.Errorf("creating index.html: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, indexData{
		Groups:     sortedGroups,
		GroupCount: len(sortedGroups),
		TotalCount: totalCount,
		UpdatedAt:  time.Now().UTC().Format("2006-01-02 15:04 UTC"),
	})
}
