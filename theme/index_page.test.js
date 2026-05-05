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

function makeElement(id, dataset = {}) {
  const listeners = new Map();
  const attrs = new Set();
  return {
    id,
    dataset,
    style: {},
    classList: makeClassList(),
    textContent: '',
    value: '',
    href: '',
    focus() {},
    blur() {},
    setAttribute(name) { attrs.add(name); },
    removeAttribute(name) { attrs.delete(name); },
    hasAttribute(name) { return attrs.has(name); },
    addEventListener(type, fn) {
      if (!listeners.has(type)) listeners.set(type, []);
      listeners.get(type).push(fn);
    },
    click(event = { preventDefault() {}, target: this }) {
      for (const fn of listeners.get('click') || []) fn.call(this, event);
    },
    dispatchEvent(event) {
      for (const fn of listeners.get(event.type) || []) fn.call(this, event);
    },
    querySelectorAll(selector) {
      if (selector === '.schemas a') return this.__links || [];
      return [];
    },
  };
}

function extractGoRawString(source, name) {
  const pattern = new RegExp(`const ${name} = \`([\\s\\S]*?)\``);
  const match = source.match(pattern);
  if (!match) throw new Error(`missing ${name}`);
  return match[1];
}

function loadIndexPageScript({ hash = '', savedState = '' } = {}) {
  const indexScript = fs.readFileSync(path.join(__dirname, 'index_page.js'), 'utf8');
  const themeSource = fs.readFileSync(path.join(__dirname, 'theme.go'), 'utf8');
  const script = `${extractGoRawString(themeSource, 'SearchHashStateJS')}\n${indexScript}\n${extractGoRawString(themeSource, 'ThemeToggleJS')}`;
  const calls = { replaceState: [], localStorageWrites: [], scrolls: [] };
  const elements = new Map();
  const documentListeners = new Map();

  const link = makeElement('schema-link', { schema: 'test_v1.json', url: '/docs/example.io/test_v1.json' });
  const copyHint = makeElement('copy-hint');
  copyHint.classList.toggle('copy-hint', true);
  copyHint.closest = (selector) => selector === '.schemas a' ? link : null;
  link.closest = (selector) => selector === '.schemas a' ? link : null;
  const group = makeElement('group-a', { group: 'example.io' });
  group.__links = [link];

  function element(id) {
    if (!elements.has(id)) {
      elements.set(id, makeElement(id));
    }
    return elements.get(id);
  }

  element('search');
  element('groups');
  element('no-results');
  element('stat-groups').textContent = '1';
  element('stat-schemas').textContent = '1';
  element('toggle-all').textContent = 'Expand all';
  element('toast');
  element('back-to-top');

  const storage = new Map([['indexState', savedState]].filter(([, value]) => value));
  const context = {
    console,
    Event,
    clearTimeout,
    setTimeout(fn) {
      fn();
      return 1;
    },
    location: {
      origin: 'https://schemas.example.test',
      hash,
      pathname: '/docs/',
    },
    history: {
      replaceState(_state, _title, url) {
        calls.replaceState.push(url);
      },
    },
    localStorage: {
      getItem(key) {
        return key === 'theme' ? 'dark' : null;
      },
      setItem(key, value) {
        calls.localStorageWrites.push([key, value]);
      },
    },
    navigator: {
      clipboard: {
        writeText(value) {
          context.__copied = value;
          return Promise.resolve();
        },
      },
    },
    sessionStorage: {
      getItem(key) { return storage.get(key) || null; },
      setItem(key, value) { storage.set(key, value); },
      removeItem(key) { storage.delete(key); },
    },
    window: {
      scrollY: 0,
      addEventListener() {},
      scrollTo(...args) { calls.scrolls.push(args); },
    },
    document: {
      body: {},
      documentElement: { classList: makeClassList() },
      activeElement: null,
      querySelectorAll(selector) {
        if (selector === '.group') return [group];
        if (selector === '.schemas a') return [link];
        if (selector === '.usage-content code') return [];
        return [];
      },
      getElementById: element,
      addEventListener(type, fn) {
        documentListeners.set(type, fn);
      },
    },
    __calls: calls,
  };
  context.window.document = context.document;
  context.window.scrollTo = context.window.scrollTo;
  context.window.SchemaSearch = {};

  vm.runInNewContext(script, context, { filename: 'index_page.js' });
  return { context, calls, element, group, link, copyHint, storage, documentListeners };
}

test('index page initializes search from hash and preserves navigation state', () => {
  const savedState = JSON.stringify({ expanded: ['example.io'], search: 'old', scroll: 10 });
  const { calls, element, group, storage } = loadIndexPageScript({ hash: '#q=deployment', savedState });

  assert.equal(element('search').value, 'deployment');
  assert.deepEqual(calls.replaceState, ['#q=deployment']);
  assert.equal(group.hasAttribute('open'), false);
  assert.equal(storage.has('indexState'), true);
});

test('index page writes hash when search query changes', () => {
  const { calls, element } = loadIndexPageScript();
  const search = element('search');

  search.value = 'certificate';
  search.dispatchEvent(new Event('input'));

  assert.equal(calls.replaceState.at(-1), '#q=certificate');
});

test('index page copy hint writes absolute schema URL', async () => {
  const { context, element, copyHint } = loadIndexPageScript();
  const groups = element('groups');

  groups.click({ target: copyHint, preventDefault() {} });
  await Promise.resolve();

  assert.equal(context.__copied, 'https://schemas.example.test/docs/example.io/test_v1.json');
});

test('index page theme toggle still writes localStorage', () => {
  const { context, calls } = loadIndexPageScript();

  context.toggleTheme();

  assert.deepEqual(calls.localStorageWrites.at(-1), ['theme', 'light']);
});
