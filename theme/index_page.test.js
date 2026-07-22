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
    closest() {
      return null;
    },
    querySelectorAll(selector) {
      if (selector === '.group') return this.__groups || [];
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

function defaultFixtures() {
  return [
    {
      source: 'crd',
      label: 'Custom Resources',
      defaultOpen: true,
      groups: [{ name: 'example.io', schemas: ['test_v1.json'] }],
    },
  ];
}

function mixedFixtures() {
  return [
    {
      source: 'crd',
      label: 'Custom Resources',
      defaultOpen: true,
      groups: [{ name: 'example.io', schemas: ['certificate_v1.json'] }],
    },
    {
      source: 'builtin',
      label: 'Kubernetes Built-ins',
      defaultOpen: false,
      groups: [
        { name: 'apps', schemas: ['deployment_v1.json'] },
        { name: 'core', schemas: ['pod_v1.json', 'service_v1.json'] },
      ],
    },
    {
      source: 'kustomize',
      label: 'Kustomize',
      defaultOpen: false,
      groups: [{ name: 'kustomize.config.k8s.io', schemas: ['kustomization_v1beta1.json'] }],
    },
  ];
}

function buildFixtureDom(fixtures, groupedBySource = fixtures.length > 1) {
	const sources = [];
	const groups = [];
	const rows = [];
  const links = [];
  const copyButtons = [];
  const sourceByKey = new Map();
  const groupByKey = new Map();
  const linkByKey = new Map();

	for (const sourceDef of fixtures) {
		let source = null;
		if (groupedBySource) {
			source = makeElement(`source-${sourceDef.source}`, {
				source: sourceDef.source,
				sourceLabel: sourceDef.label,
				defaultOpen: sourceDef.defaultOpen ? 'true' : 'false',
			});
			if (sourceDef.defaultOpen) source.setAttribute('open', '');
			source.__groups = [];
			sources.push(source);
			sourceByKey.set(sourceDef.source, source);
		}

		for (const groupDef of sourceDef.groups) {
			const dataset = groupedBySource ? { source: sourceDef.source, group: groupDef.name } : { group: groupDef.name };
			const group = makeElement(`${sourceDef.source}-${groupDef.name}`, dataset);
			group.__rows = [];
			group.__links = [];
			if (source) source.__groups.push(group);
			groups.push(group);
			groupByKey.set(`${sourceDef.source}:${groupDef.name}`, group);

      for (const schema of groupDef.schemas) {
        const url = `/docs/${groupDef.name}/${schema}`;
        const row = makeElement(`${sourceDef.source}-${groupDef.name}-${schema}`, { schema });
        const link = makeElement(`link-${sourceDef.source}-${groupDef.name}-${schema}`, { url });
        const copyButton = makeElement(`copy-${sourceDef.source}-${groupDef.name}-${schema}`, { url });
        copyButton.classList.toggle('schema-copy', true);
        copyButton.closest = (selector) => selector === '.schema-copy' ? copyButton : null;
        link.closest = (selector) => selector === '.schema-row a' || selector === '.schemas a' ? link : null;
        group.__rows.push(row);
        group.__links.push(link);
        rows.push(row);
        links.push(link);
        copyButtons.push(copyButton);
        linkByKey.set(`${sourceDef.source}:${groupDef.name}:${schema}`, link);
      }
    }
  }

  return { sources, groups, rows, links, copyButtons, sourceByKey, groupByKey, linkByKey };
}

function uniqueGroupCount(groups) {
  return new Set(groups.map((group) => group.dataset.group)).size;
}

function loadIndexPageScript({
  hash = '',
  savedState = '',
  reducedMotion = false,
  clipboard = 'success',
  fixtures = defaultFixtures(),
} = {}) {
  const indexScript = fs.readFileSync(path.join(__dirname, 'index_page.js'), 'utf8');
  const themeSource = fs.readFileSync(path.join(__dirname, 'theme.go'), 'utf8');
  const script = `${extractGoRawString(themeSource, 'SearchHashStateJS')}\n${extractGoRawString(themeSource, 'BackToTopJS')}\n${optionalGoRawString(themeSource, 'CopyToastJS')}\n${indexScript}\n${extractGoRawString(themeSource, 'ThemeToggleJS')}`;
  const calls = { replaceState: [], localStorageWrites: [], scrolls: [], timers: [] };
  const elements = new Map();
  const documentListeners = new Map();
  const fixtureDom = buildFixtureDom(fixtures);

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
  element('stat-groups').textContent = String(uniqueGroupCount(fixtureDom.groups));
  element('stat-schemas').textContent = String(fixtureDom.rows.length);
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
      body: { dataset: { basePath: '/docs' } },
      documentElement: { classList: makeClassList() },
      activeElement: null,
      querySelectorAll(selector) {
        if (selector === '.source-section') return fixtureDom.sources;
        if (selector === '.group') return fixtureDom.groups;
        if (selector === '.schema-row') return fixtureDom.rows;
        if (selector === '.schemas a') return fixtureDom.links;
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
  context.window.SchemaSearch = {};

  vm.runInNewContext(script, context, { filename: 'index_page.js' });
  return {
    context,
    calls,
    element,
    storage,
    documentListeners,
    ...fixtureDom,
    source(key) { return fixtureDom.sourceByKey.get(key); },
    group(source, group) { return fixtureDom.groupByKey.get(`${source}:${group}`); },
    link(source, group, schema) { return fixtureDom.linkByKey.get(`${source}:${group}:${schema}`); },
  };
}

test('index page initializes search from hash and preserves navigation state', () => {
  const savedState = JSON.stringify({ sources: ['crd'], groups: ['crd:example.io'], search: 'old', scroll: 10 });
  const { calls, element, group, storage } = loadIndexPageScript({ hash: '#q=deployment', savedState });

  assert.equal(element('search').value, 'deployment');
  assert.deepEqual(calls.replaceState, ['#q=deployment']);
  assert.equal(group('crd', 'example.io').hasAttribute('open'), false);
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
  assert.equal(status.textContent, 'No matching sources, groups, or schemas.');

  search.value = '';
  search.dispatchEvent(new Event('input'));

  assert.equal(status.textContent, '');
});

test('index page source-only search shows all groups in matching source', () => {
  const page = loadIndexPageScript({ fixtures: mixedFixtures() });
  const search = page.element('search');

  search.value = 'builtin';
  search.dispatchEvent(new Event('input'));

  assert.equal(page.source('crd').style.display, 'none');
  assert.equal(page.source('builtin').style.display, '');
  assert.equal(page.source('builtin').hasAttribute('open'), true);
  assert.equal(page.source('kustomize').style.display, 'none');
  assert.equal(page.group('builtin', 'apps').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'core').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'core').__rows.every((row) => row.style.display === ''), true);
  assert.equal(page.element('stat-groups').textContent, '2 / 4');
  assert.equal(page.element('stat-schemas').textContent, '3 / 5');
});

test('index page group search opens only matching group unless source label matches', () => {
  const page = loadIndexPageScript({ fixtures: mixedFixtures() });
  const search = page.element('search');

  search.value = 'core';
  search.dispatchEvent(new Event('input'));

  assert.equal(page.source('builtin').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'core').style.display, '');
  assert.equal(page.group('builtin', 'core').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'apps').style.display, 'none');

  search.value = 'kubernetes';
  search.dispatchEvent(new Event('input'));

  assert.equal(page.group('builtin', 'core').style.display, '');
  assert.equal(page.group('builtin', 'apps').style.display, '');
  assert.equal(page.group('builtin', 'core').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'apps').hasAttribute('open'), true);
});

test('index page clear button restores default source and group state', () => {
  const page = loadIndexPageScript({ fixtures: mixedFixtures() });
  const search = page.element('search');
  const clear = page.element('search-clear');

  search.value = 'pod';
  search.dispatchEvent(new Event('input'));

  assert.equal(clear.hidden, false);
  assert.equal(page.source('builtin').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'core').hasAttribute('open'), true);

  clear.click();

  assert.equal(search.value, '');
  assert.equal(clear.hidden, true);
  assert.equal(search.focused, true);
  assert.equal(page.source('crd').hasAttribute('open'), true);
  assert.equal(page.source('builtin').hasAttribute('open'), false);
  assert.equal(page.source('kustomize').hasAttribute('open'), false);
  assert.equal(page.groups.some((group) => group.hasAttribute('open')), false);
  assert.equal(page.calls.replaceState.at(-1), '/docs/');
});

test('index page expand and collapse all affects visible source sections and groups', () => {
  const page = loadIndexPageScript({ fixtures: mixedFixtures() });
  const toggle = page.element('toggle-all');

  toggle.click();

  assert.equal(toggle.textContent, 'Collapse all');
  assert.equal(page.sources.every((source) => source.hasAttribute('open')), true);
  assert.equal(page.groups.every((group) => group.hasAttribute('open')), true);

  toggle.click();

  assert.equal(toggle.textContent, 'Expand all');
  assert.equal(page.source('crd').hasAttribute('open'), true);
  assert.equal(page.source('builtin').hasAttribute('open'), false);
  assert.equal(page.source('kustomize').hasAttribute('open'), false);
  assert.equal(page.groups.every((group) => !group.hasAttribute('open')), true);
});

test('index page saves source-qualified navigation state', () => {
  const page = loadIndexPageScript({ fixtures: mixedFixtures() });
  page.source('builtin').setAttribute('open', '');
  page.group('builtin', 'core').setAttribute('open', '');
  page.context.window.scrollY = 42;

  page.element('groups').click({
    target: page.link('builtin', 'core', 'pod_v1.json'),
    preventDefault() {},
  });

  const state = JSON.parse(page.storage.get('indexState'));
  assert.deepEqual(state.sources, ['crd', 'builtin']);
  assert.deepEqual(state.groups, ['builtin:core']);
  assert.equal(state.scroll, 42);
});

test('index page restores source and group session state', () => {
  const savedState = JSON.stringify({ sources: ['builtin'], groups: ['builtin:core'], scroll: 10 });
  const page = loadIndexPageScript({ fixtures: mixedFixtures(), savedState });

  assert.equal(page.storage.has('indexState'), false);
  assert.equal(page.source('crd').hasAttribute('open'), false);
  assert.equal(page.source('builtin').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'core').hasAttribute('open'), true);
  assert.equal(page.group('builtin', 'apps').hasAttribute('open'), false);
  assert.deepEqual(page.calls.scrolls.at(-1), [0, 10]);
});

test('index page restores old expanded group session state as CRD groups', () => {
  const savedState = JSON.stringify({ expanded: ['example.io'] });
  const page = loadIndexPageScript({ fixtures: mixedFixtures(), savedState });

  assert.equal(page.source('crd').hasAttribute('open'), true);
  assert.equal(page.group('crd', 'example.io').hasAttribute('open'), true);
  assert.equal(page.source('builtin').hasAttribute('open'), false);
});

test('index page copy button writes absolute schema URL without saving navigation state', async () => {
  const { context, element, copyButtons, storage } = loadIndexPageScript();
  const groups = element('groups');

  groups.click({ target: copyButtons[0], preventDefault() {} });
  await Promise.resolve();

  assert.equal(context.__copied, 'https://schemas.example.test/docs/example.io/test_v1.json');
  assert.equal(element('toast').textContent, 'Copied!');
  assert.equal(element('toast').classList.contains('show'), true);
  assert.equal(storage.has('indexState'), false);
});

test('index page copy button reports unavailable clipboard', async () => {
  const { context, element, copyButtons } = loadIndexPageScript({ clipboard: null });
  const groups = element('groups');

  groups.click({ target: copyButtons[0], preventDefault() {} });
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
