const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function makeClassList() {
  const values = new Set();
  return {
    add(...names) {
      names.forEach((name) => values.add(name));
    },
    remove(...names) {
      names.forEach((name) => values.delete(name));
    },
    toggle(name, force) {
      const enabled = force === undefined ? !values.has(name) : !!force;
      if (enabled) values.add(name);
      else values.delete(name);
      return enabled;
    },
    contains(name) {
      return values.has(name);
    },
  };
}

function makeElement(id, dataset = {}, tagName = 'DIV') {
  const listeners = new Map();
  const attrs = new Set();
  return {
    id,
    tagName,
    dataset,
    style: {},
    classList: makeClassList(),
    hidden: false,
    disabled: false,
    open: false,
    textContent: '',
    value: '',
    selectionStart: 0,
    selectionEnd: 0,
    focus() {},
    blur() {},
    setAttribute(name) {
      attrs.add(name);
      if (name === 'open') this.open = true;
    },
    removeAttribute(name) {
      attrs.delete(name);
      if (name === 'open') this.open = false;
    },
    hasAttribute(name) {
      return attrs.has(name);
    },
    addEventListener(type, fn) {
      listeners.set(type, fn);
    },
    click() {
      listeners.get('click')?.call(this, { preventDefault() {} });
    },
    dispatchEvent(event) {
      listeners.get(event.type)?.call(this, event);
    },
  };
}

function extractGoRawString(source, name) {
  const pattern = new RegExp(`const ${name} = \`([\\s\\S]*?)\``);
  const match = source.match(pattern);
  if (!match) throw new Error(`missing ${name}`);
  return match[1];
}

function loadSchemaPageScript() {
  const pageScript = fs.readFileSync(path.join(__dirname, 'schema_page.js'), 'utf8');
  const themeSource = fs.readFileSync(path.join(__dirname, 'theme.go'), 'utf8');
  const script = `${extractGoRawString(themeSource, 'SearchHashStateJS')}\n${pageScript}`;
  const elements = new Map();
  const documentListeners = new Map();
  const detailsA = makeElement('details-a', { path: 'spec', name: 'spec', text: 'Spec', parentPath: '' }, 'DETAILS');
  const detailsB = makeElement('details-b', { path: 'spec.replicas', name: 'replicas', text: 'Replica count', parentPath: 'spec' }, 'DETAILS');
  [detailsA, detailsB].forEach((el) => elements.set(el.id, el));

  function element(id) {
    if (!elements.has(id)) {
      const dataset = {};
      if (id === 'search-status') dataset.emptyMessage = 'Tip';
      if (id === 'no-results') dataset.noResultsMessage = 'No matches';
      if (id === 'copy-url') dataset.url = '/docs/example.io/test_v1.json';
      elements.set(id, makeElement(id, dataset));
    }
    return elements.get(id);
  }

  const context = {
    console,
    Event,
    clearTimeout,
    setTimeout(fn) {
      fn();
      return 1;
    },
    localStorage: {
      getItem() { return null; },
      setItem() {},
    },
    location: {
      origin: 'https://schemas.example.test',
      hash: '',
      href: '',
      pathname: '/docs/example.io/test_v1.html',
    },
    history: {
      replaceState() {},
    },
    navigator: {
      clipboard: {
        writeText(value) {
          context.__copied = value;
          return Promise.resolve();
        },
      },
    },
    document: {
      body: { dataset: { basePath: '/docs' } },
      activeElement: null,
      querySelectorAll(selector) {
        if (selector === '.prop') return [detailsA, detailsB];
        if (selector === '[data-prop-row]') return [detailsA, detailsB];
        return [];
      },
      getElementById: element,
      addEventListener(type, fn) {
        documentListeners.set(type, fn);
      },
    },
    window: {
      SchemaSearch: {
        bestCompletionForPaths() { return ''; },
        completionCandidatesForPaths() { return []; },
        ghostPrefixForCompletion() { return ''; },
        ghostSuffixForCompletion() { return ''; },
        dotAdvanceForPathSearch() { return ''; },
        isPathLikeQuery() { return false; },
        matchesPathQuery(pathValue, query) { return pathValue === query.replace(/^\./, ''); },
        splitPathSegments(query) { return { segments: query.split('.').filter(Boolean) }; },
        trimPathSearch(query) { return query; },
      },
    },
    __copied: '',
  };
  context.window.document = context.document;

  vm.runInNewContext(script, context, { filename: 'schema_page.js' });
  return { context, element, detailsA, detailsB, documentListeners };
}

test('schema page expands and collapses details', () => {
  const { element, detailsA, detailsB } = loadSchemaPageScript();
  element('expand-all').click();
  assert.equal(detailsA.open, true);
  assert.equal(detailsB.open, true);
  element('collapse-all').click();
  assert.equal(detailsA.open, false);
  assert.equal(detailsB.open, false);
});

test('schema page copy button writes absolute schema URL', async () => {
  const { context, element } = loadSchemaPageScript();
  element('copy-url').click();
  await Promise.resolve();
  assert.equal(context.__copied, 'https://schemas.example.test/docs/example.io/test_v1.json');
});
