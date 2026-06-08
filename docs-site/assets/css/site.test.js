const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");

const css = fs.readFileSync(path.join(__dirname, "site.css"), "utf8");

test("uses custom docs-site scrollbars", () => {
  for (const expected of [
    "--scrollbar-thumb: rgba(255, 255, 255, 0.18);",
    "--scrollbar-thumb-active: rgba(255, 255, 255, 0.36);",
    "scrollbar-color: transparent transparent;",
    "scrollbar-color: var(--scrollbar-thumb) transparent;",
    "scrollbar-width: thin;",
    "-webkit-overflow-scrolling: touch;",
    "width: 12px;",
    "height: 12px;",
    "border: 3px solid transparent;",
    "border-radius: 20px;",
    "background-clip: content-box;",
  ]) {
    assert.match(css, new RegExp(escapeRegExp(expected)), `missing ${expected}`);
  }
});

test("applies custom scrollbars to docs scroll regions", () => {
  for (const selector of [
    ".site-sidebar",
    ".site-toc",
    ".doc-content pre",
    ".doc-content table",
    ".search-panel ul",
  ]) {
    assert.match(css, new RegExp(`${escapeRegExp(selector)}\\s*[,\\{]`), `missing ${selector}`);
  }
});

test("reserves a gutter between navigation highlights and scrollbars", () => {
  assert.match(css, /\.site-sidebar,\n\.site-toc\s*\{[^}]*padding-inline-end: 0\.35rem;/s);
  assert.match(css, /\.site-sidebar,\n\.site-toc\s*\{[^}]*scrollbar-gutter: stable;/s);
  assert.match(css, /\.docs-nav a,\n\.site-toc a\s*\{[^}]*margin-inline-end: 0\.65rem;/s);
});

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
