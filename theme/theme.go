package theme

// CSSVars contains CSS custom properties for dark and light themes.
// This is the union of variables used across all pages (index + schema renderer).
const CSSVars = `
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
    --required-bg: rgba(251, 191, 36, 0.15);
    --required-fg: #fbbf24;
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
    --required-bg: rgba(217, 119, 6, 0.1);
    --required-fg: #b45309;
    --stripes-dark: none;
    --rainbow: none;
  }`

// CSSBase contains shared base styles: reset, body, starfield, flare, theme toggle, toast, and footer.
const CSSBase = `
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
  .theme-toggle {
    background: none; border: 1px solid var(--border); border-radius: 6px;
    color: var(--fg-muted); cursor: pointer; padding: 0.35rem 0.6rem; font-size: 0.85rem;
    transition: border-color 0.2s, color 0.2s;
  }
  .theme-toggle:hover { border-color: var(--accent); color: var(--accent); }
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
  footer a:hover { text-decoration: underline; }`

// SearchCSS contains shared search input, status, and empty-state styles.
const SearchCSS = `
  .search-box {
    width: 100%; padding: 0.65rem 1rem; font-size: 0.95rem;
    background: var(--bg-surface); color: var(--fg);
    border: 1px solid var(--border); border-radius: 6px;
    outline: none; transition: border-color 0.2s;
  }
  .search-box::placeholder { color: var(--fg-muted); }
  .search-box:focus { border-color: var(--accent); }
  .search-status {
    color: var(--fg-muted); font-size: 0.8rem;
    min-height: 1.25rem; white-space: nowrap;
  }
  .search-status.has-results { color: var(--accent); }
  .no-results {
    text-align: center; color: var(--fg-muted); padding: 3rem 1rem;
    font-size: 0.95rem; display: none;
  }`

// HeadScript is the FOUC prevention script placed in <head>.
const HeadScript = `<script>if(localStorage.getItem('theme')==='light')document.documentElement.className='light';</script>`

// FlareDiv is the flare background element.
const FlareDiv = `<div class="flare"></div>`

// ThemeToggleButton is the light/dark mode toggle.
const ThemeToggleButton = `<button class="theme-toggle" onclick="toggleTheme()" title="Toggle light/dark mode">☀/☾</button>`

// ToastDiv is the clipboard copy toast notification.
const ToastDiv = `<div class="copied-toast" id="toast">Copied!</div>`

// FooterHTML is the page footer.
const FooterHTML = `<footer>
  Generated by <a href="https://github.com/sholdee/crd-schema-publisher">crd-schema-publisher</a>
</footer>`

// SearchHintText is the shared keyboard hint used in search placeholders.
const SearchHintText = `( / to focus, Esc to clear )`

// SearchHashStateJS contains small shared helpers for hash-based search state.
const SearchHashStateJS = `function hasHashSearchQuery(){
  return (location.hash || '').indexOf('#q=') === 0;
}
function readHashSearchQuery(){
  var hash = location.hash || '';
  if (hasHashSearchQuery()) {
    try {
      return decodeURIComponent(hash.slice(3));
    } catch (err) {
      return '';
    }
  }
  return '';
}
function writeHashSearchQuery(q){
  history.replaceState(null, '', q ? '#q=' + encodeURIComponent(q) : location.pathname);
}`

// ThemeToggleJS is the JavaScript function for toggling the theme.
const ThemeToggleJS = `function toggleTheme(){
  document.documentElement.classList.toggle('light');
  localStorage.setItem('theme', document.documentElement.classList.contains('light') ? 'light' : 'dark');
}`
