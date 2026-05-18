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
  const attrs = new Map();
  return {
    id,
    dataset,
    style: {},
    classList: makeClassList(),
    textContent: '',
    value: '',
    href: '',
    hidden: false,
    focused: false,
    focus() { this.focused = true; },
    blur() {},
    setAttribute(name, value = '') { attrs.set(name, String(value)); },
    removeAttribute(name) { attrs.delete(name); },
    hasAttribute(name) { return attrs.has(name); },
    getAttribute(name) { return attrs.has(name) ? attrs.get(name) : null; },
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
      if (selector === '.schema-row') return this.__rows || [];
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

function optionalGoRawString(source, name) {
  const pattern = new RegExp(`const ${name} = \`([\\s\\S]*?)\``);
  const match = source.match(pattern);
  return match ? match[1] : '';
}

function loadIndexPageScript({ hash = '', savedState = '', reducedMotion = false, clipboard = 'success' } = {}) {
  const indexScript = fs.readFileSync(path.join(__dirname, 'index_page.js'), 'utf8');
  const themeSource = fs.readFileSync(path.join(__dirname, 'theme.go'), 'utf8');
  const script = `${extractGoRawString(themeSource, 'SearchHashStateJS')}\n${extractGoRawString(themeSource, 'BackToTopJS')}\n${optionalGoRawString(themeSource, 'CopyToastJS')}\n${indexScript}\n${extractGoRawString(themeSource, 'ThemeToggleJS')}`;
  const calls = { replaceState: [], localStorageWrites: [], scrolls: [], timers: [] };
  const elements = new Map();
  const documentListeners = new Map();

  const row = makeElement('schema-row', { schema: 'test_v1.json' });
  const link = makeElement('schema-link', { url: '/docs/example.io/test_v1.json' });
  const copyButton = makeElement('schema-copy', { url: '/docs/example.io/test_v1.json' });
  copyButton.classList.toggle('schema-copy', true);
  copyButton.closest = (selector) => selector === '.schema-copy' ? copyButton : null;
  link.closest = (selector) => selector === '.schema-row a' || selector === '.schemas a' ? link : null;
  const group = makeElement('group-a', { group: 'example.io' });
  group.__rows = [row];
  group.__links = [link];

  function element(id) {
    if (!elements.has(id)) {
      elements.set(id, makeElement(id));
    }
    return elements.get(id);
  }

  element('search');
  element('search-clear').hidden = true;
  element('groups');
  element('no-results');
  element('search-status');
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
      calls.timers.push(fn);
      return calls.timers.length;
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
    navigator: clipboard === null ? {} : {
      clipboard: {
        writeText(value) {
          if (clipboard === 'reject') return Promise.reject(new Error('denied'));
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
      matchMedia(query) {
        return { matches: reducedMotion && query === '(prefers-reduced-motion: reduce)' };
      },
      scrollTo(...args) { calls.scrolls.push(args); },
    },
    document: {
      body: {},
      documentElement: { classList: makeClassList() },
      activeElement: null,
      querySelectorAll(selector) {
        if (selector === '.group') return [group];
        if (selector === '.schema-row') return [row];
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
  return { context, calls, element, group, row, link, copyButton, storage, documentListeners };
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

test('index page announces search result counts', () => {
  const { element } = loadIndexPageScript();
  const search = element('search');
  const status = element('search-status');

  search.value = 'test';
  search.dispatchEvent(new Event('input'));

  assert.equal(status.textContent, '1 of 1 API groups, 1 of 1 schemas shown.');

  search.value = 'missing';
  search.dispatchEvent(new Event('input'));

  assert.equal(element('no-results').style.display, 'block');
  assert.equal(status.textContent, 'No matching groups or schemas.');

  search.value = '';
  search.dispatchEvent(new Event('input'));

  assert.equal(status.textContent, '');
});

test('index page clear button follows and clears search input', () => {
  const { calls, element } = loadIndexPageScript();
  const search = element('search');
  const clear = element('search-clear');

  assert.equal(clear.hidden, true);

  search.value = 'certificate';
  search.dispatchEvent(new Event('input'));

  assert.equal(clear.hidden, false);

  clear.click();

  assert.equal(search.value, '');
  assert.equal(clear.hidden, true);
  assert.equal(search.focused, true);
  assert.equal(calls.replaceState.at(-1), '/docs/');
});

test('index page copy button writes absolute schema URL without saving navigation state', async () => {
  const { context, element, copyButton, storage } = loadIndexPageScript();
  const groups = element('groups');

  groups.click({ target: copyButton, preventDefault() {} });
  await Promise.resolve();

  assert.equal(context.__copied, 'https://schemas.example.test/docs/example.io/test_v1.json');
  assert.equal(element('toast').textContent, 'Copied!');
  assert.equal(element('toast').classList.contains('show'), true);
  assert.equal(storage.has('indexState'), false);
});

test('index page copy button reports unavailable clipboard', async () => {
  const { context, element, copyButton } = loadIndexPageScript({ clipboard: null });
  const groups = element('groups');

  groups.click({ target: copyButton, preventDefault() {} });
  await Promise.resolve();

  assert.equal(context.__copied, undefined);
  assert.equal(element('toast').textContent, 'Copy unavailable');
  assert.equal(element('toast').classList.contains('show'), true);
});

test('index page theme toggle still writes localStorage', () => {
  const { context, calls } = loadIndexPageScript();

  context.toggleTheme();

  assert.deepEqual(calls.localStorageWrites.at(-1), ['theme', 'light']);
});

test('index page theme toggle updates accessible state', () => {
  const { context, element } = loadIndexPageScript();
  const toggle = element('theme-toggle');

  assert.equal(toggle.getAttribute('aria-pressed'), 'false');
  assert.equal(toggle.getAttribute('aria-label'), 'Light mode');

  context.toggleTheme();

  assert.equal(toggle.getAttribute('aria-pressed'), 'true');
  assert.equal(toggle.getAttribute('aria-label'), 'Light mode');
});

test('index page avoids smooth scroll when reduced motion is requested', () => {
  const { calls, element } = loadIndexPageScript({ reducedMotion: true });

  element('back-to-top').click();

  assert.equal(calls.scrolls.at(-1)[0].top, 0);
  assert.equal(calls.scrolls.at(-1)[0].behavior, 'auto');
});
