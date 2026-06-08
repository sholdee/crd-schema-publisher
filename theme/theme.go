package theme

// CSSVars contains CSS custom properties for dark and light themes.
// This is the union of variables used across all pages (index + schema renderer).
const CSSVars = `
  :root {
    --bg: #09090b;
    --bg-surface: rgba(24, 24, 27, 0.6);
    --bg-hover: rgba(24, 24, 27, 0.8);
    --surface-background: linear-gradient(var(--bg-surface), var(--bg-surface)), var(--bg);
    --surface-hover-background: linear-gradient(var(--bg-hover), var(--bg-hover)), var(--bg);
    --surface-accent-background: linear-gradient(var(--accent-dim), var(--accent-dim)), var(--bg);
    --fg: #fafafa;
    --fg-muted: #b0b3bc;
    --accent: #6bc1fe;
    --accent-dim: rgba(107, 193, 254, 0.15);
    --border: rgba(255, 255, 255, 0.1);
    --border-active: #6bc1fe;
    --stat-fg: #fafafa;
    --required-bg: rgba(251, 191, 36, 0.25);
    --required-fg: #f59e0b;
  }
  .light {
    --bg: #f5f7fa;
    --bg-surface: #ffffff;
    --bg-hover: #edf0f4;
    --fg: #18181b;
    --fg-muted: #374151;
    --accent: #2563b0;
    --accent-dim: rgba(37, 99, 176, 0.08);
    --border: #d8dde4;
    --border-active: #2563b0;
    --stat-fg: #18181b;
    --required-bg: rgba(217, 119, 6, 0.15);
    --required-fg: #92400e;
  }`

// CSSBase contains shared base styles: reset, body, theme toggle, toast, and footer.
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
  .light body::before { display: none; }
  .skip-nav {
    position: absolute; left: -999px; top: auto; width: 1px; height: 1px;
    overflow: hidden; z-index: 100;
  }
  .skip-nav:focus, .skip-nav:active {
    left: auto; width: auto; height: auto; padding: 0.5rem 1rem;
    background: var(--accent); color: #09090b;
    border-radius: 6px; font-weight: 600; text-decoration: none;
    margin: 0.5rem; outline: 2px solid var(--accent);
  }
  .visually-hidden {
    position: absolute; width: 1px; height: 1px; padding: 0;
    margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0);
    white-space: nowrap; border: 0;
  }
  :focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after {
      animation-duration: 0.01ms !important; animation-iteration-count: 1 !important;
      transition-duration: 0.01ms !important; scroll-behavior: auto !important;
    }
  }
  .theme-toggle {
    background: none; border: 1px solid var(--border); border-radius: 6px;
    color: var(--fg-muted); cursor: pointer; padding: 0.35rem 0.6rem; font-size: 0.9rem;
    transition: border-color 0.2s, color 0.2s;
  }
  .theme-toggle:hover { border-color: var(--accent); color: var(--accent); }
  .copied-toast {
    position: fixed; bottom: 1.5rem; left: 50%; transform: translateX(-50%);
    background: var(--accent); color: #09090b; padding: 0.4rem 1rem;
    border-radius: 6px; font-size: 0.875rem; font-weight: 600;
    opacity: 0; transition: opacity 0.2s;
    pointer-events: none; z-index: 10;
  }
  .copied-toast.show { opacity: 1; }
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
  footer {
    margin-top: 3rem; padding-top: 1.5rem;
    border-top: 1px solid var(--border);
    text-align: center; font-size: 0.875rem; color: var(--fg-muted);
  }
  footer a { color: var(--accent); text-decoration: none; }
  footer a:hover { text-decoration: underline; }`

// SearchCSS contains shared search input, status, and empty-state styles.
const SearchCSS = `
  .search-box {
    width: 100%; padding: 0.65rem 1rem; font-size: 0.95rem;
    background: var(--surface-background); color: var(--fg);
    border: 1px solid var(--border); border-radius: 6px;
    outline: none; transition: border-color 0.2s;
  }
  .search-box::placeholder { color: var(--fg-muted); }
  .search-box:focus { border-color: var(--accent); }
  .search-input-wrap {
    display: grid;
    position: relative;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface-background);
    font: inherit; font-size: 0.95rem; line-height: 1.2;
    font-family: inherit; font-weight: inherit; letter-spacing: inherit;
    text-transform: inherit; text-indent: inherit; font-kerning: inherit;
    transition: border-color 0.2s;
  }
  .search-input-wrap:focus-within { border-color: var(--accent); }
  .search-input-wrap .search-box {
    grid-area: 1 / 1;
    position: relative; z-index: 1;
    background: transparent; border-color: transparent;
    padding-right: 2.75rem;
    margin: 0; appearance: none; -webkit-appearance: none;
    font: inherit; line-height: inherit;
    font-family: inherit; font-size: inherit; font-weight: inherit; letter-spacing: inherit;
    text-transform: inherit; text-indent: inherit; font-kerning: inherit;
    width: 100%;
  }
  .search-input-wrap .search-box:focus { border-color: transparent; }
  .search-input-wrap .search-box::-webkit-search-decoration,
  .search-input-wrap .search-box::-webkit-search-cancel-button,
  .search-input-wrap .search-box::-webkit-search-results-button,
  .search-input-wrap .search-box::-webkit-search-results-decoration { display: none; }
  .search-clear {
    position: absolute; right: 0.55rem; top: 50%; transform: translateY(-50%);
    z-index: 2; width: 1.5rem; height: 1.5rem; padding: 0;
    display: flex; align-items: center; justify-content: center;
    border: 1px solid transparent; border-radius: 999px;
    background: transparent; color: var(--accent); cursor: pointer;
    opacity: 0.85; font: inherit; font-size: 0; line-height: 1;
    transition: background 0.15s, opacity 0.15s;
  }
  .search-clear::before,
  .search-clear::after {
    content: ""; position: absolute; left: 50%; top: 50%;
    width: 0.7rem; height: 2px; border-radius: 999px;
    background: currentColor; transform-origin: center;
  }
  .search-clear::before { transform: translate(-50%, -50%) rotate(45deg); }
  .search-clear::after { transform: translate(-50%, -50%) rotate(-45deg); }
  .search-clear:hover,
  .search-clear:focus-visible {
    background: var(--accent-dim); opacity: 1;
  }
  .search-clear[hidden] { display: none; }
  .search-status {
    color: var(--fg-muted); font-size: 0.875rem;
    min-height: 1.25rem; overflow-wrap: anywhere;
  }
  .search-status.has-results { color: var(--accent); }
  .no-results {
    text-align: center; color: var(--fg-muted); padding: 3rem 1rem;
    font-size: 0.95rem; display: none;
  }`

// HeadScript is the FOUC prevention script placed in <head>.
const HeadScript = `<script>if(localStorage.getItem('theme')==='light')document.documentElement.className='light';</script>`

// ThemeToggleButton is the light/dark mode toggle.
const ThemeToggleButton = `<button class="theme-toggle" id="theme-toggle" onclick="toggleTheme()" title="Toggle light/dark mode" aria-label="Light mode" aria-pressed="false">☀/☾</button>`

// ToastDiv is the clipboard copy toast notification.
const ToastDiv = `<div class="copied-toast" id="toast" role="status" aria-live="polite" aria-atomic="true"></div>`

// BackToTopButton is the floating scroll-to-top control.
const BackToTopButton = `<button class="back-to-top" id="back-to-top" title="Back to top" aria-label="Back to top">&#8593;</button>`

// FooterHTML is the page footer.
const FooterHTML = `<footer>
  Generated by <a href="https://sholdee.github.io/crd-schema-publisher/">crd-schema-publisher</a>
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

// BackToTopJS wires the floating scroll-to-top button and respects reduced-motion preferences.
const BackToTopJS = `function scrollToTop(){
  var behavior = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth';
  window.scrollTo({top: 0, behavior: behavior});
}
(function(){
  var btt = document.getElementById('back-to-top');
  if (!btt) return;
  window.addEventListener('scroll', function(){
    btt.classList.toggle('visible', window.scrollY > 300);
  }, {passive: true});
  btt.addEventListener('click', function(){
    scrollToTop();
  });
})();`

// CopyToastJS contains shared clipboard copy feedback for pages with ToastDiv.
const CopyToastJS = `var copyToastTimer;
function showCopyToast(message){
  var toast = document.getElementById('toast');
  if (!toast) return;
  clearTimeout(copyToastTimer);
  toast.textContent = message;
  toast.classList.add('show');
  copyToastTimer = setTimeout(function(){
    toast.classList.remove('show');
    toast.textContent = '';
  }, 1500);
}
function copyURLWithToast(url){
  if (typeof navigator === 'undefined' || !navigator.clipboard || !navigator.clipboard.writeText) {
    showCopyToast('Copy unavailable');
    return Promise.resolve(false);
  }
  return navigator.clipboard.writeText(url).then(function(){
    showCopyToast('Copied!');
    return true;
  }).catch(function(){
    showCopyToast('Copy failed');
    return false;
  });
}`

// ThemeToggleJS is the JavaScript function for toggling the theme.
const ThemeToggleJS = `function updateThemeToggle(){
  var toggle = document.getElementById('theme-toggle');
  if (!toggle) return;
  var light = document.documentElement.classList.contains('light');
  toggle.setAttribute('aria-pressed', light ? 'true' : 'false');
  toggle.setAttribute('aria-label', 'Light mode');
  toggle.setAttribute('title', 'Toggle light/dark mode');
}
function toggleTheme(){
  document.documentElement.classList.toggle('light');
  localStorage.setItem('theme', document.documentElement.classList.contains('light') ? 'light' : 'dark');
  updateThemeToggle();
}
updateThemeToggle();`
