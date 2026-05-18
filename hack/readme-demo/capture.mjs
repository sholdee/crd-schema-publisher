import { spawn } from 'node:child_process';
import { mkdir, rm, writeFile } from 'node:fs/promises';
import { request } from 'node:http';
import { Buffer } from 'node:buffer';

const chromePath = process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const previewURL = process.env.PREVIEW_URL || 'http://127.0.0.1:8989';
const previewHost = new URL(previewURL).host;
const frameDir = process.env.FRAME_DIR || '/private/tmp/crd-demo-frames';
const profileDir = process.env.CHROME_PROFILE_DIR || '/private/tmp/crd-demo-chrome-profile';
const port = Number(process.env.CHROME_DEBUG_PORT || '9333');
const width = Number(process.env.DEMO_WIDTH || '1280');
const height = Number(process.env.DEMO_HEIGHT || '720');
const schemaCount = process.env.SCHEMA_COUNT || '191';
const extractTime = process.env.EXTRACT_TIME || '2026-05-18T00:41:44';
const previewTime = process.env.PREVIEW_TIME || '2026-05-18T00:43:02';
const installedVersion = process.env.INSTALLED_VERSION || 'v2026.515.13351';

const frameBorderCSS = `
  body::after {
    content: "";
    position: fixed;
    inset: 0;
    border: 1px solid rgba(107, 193, 254, 0.45);
    box-sizing: border-box;
    pointer-events: none;
    z-index: 2147483647;
  }
`;

const frames = [];

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function escapeHTML(value) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}

function typedCommandStages(command, delay = 0.03, finalDelay = 0.45) {
  const stages = [];
  for (let i = 1; i <= command.length; i += 1) {
    stages.push({
      lines: [`$ ${command.slice(0, i)}`],
      cursor: true,
      delay: i === command.length ? finalDelay : delay,
    });
  }
  return stages;
}

function appendTypedCommandStages(stages, command, delay = 0.03, finalDelay = 0.45) {
  stages.push(...typedCommandStages(command, delay, finalDelay));
}

function httpGetJSON(url) {
  return new Promise((resolve, reject) => {
    request(url, res => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', chunk => {
        body += chunk;
      });
      res.on('end', () => {
        try {
          resolve(JSON.parse(body));
        } catch (err) {
          reject(err);
        }
      });
    }).on('error', reject).end();
  });
}

async function waitForTargets() {
  const listURL = `http://127.0.0.1:${port}/json/list`;
  for (let i = 0; i < 80; i += 1) {
    try {
      const targets = await httpGetJSON(listURL);
      const page = targets.find(target => target.type === 'page' && target.webSocketDebuggerUrl);
      if (page) {
        return page.webSocketDebuggerUrl;
      }
    } catch {
      // Chrome is still starting.
    }
    await sleep(100);
  }
  throw new Error('Chrome DevTools target did not become available');
}

class CDP {
  constructor(url) {
    this.url = url;
    this.nextID = 1;
    this.pending = new Map();
  }

  async connect() {
    this.ws = new WebSocket(this.url);
    this.ws.onmessage = event => {
      const message = JSON.parse(event.data);
      if (!message.id || !this.pending.has(message.id)) {
        return;
      }
      const { resolve, reject } = this.pending.get(message.id);
      this.pending.delete(message.id);
      if (message.error) {
        reject(new Error(message.error.message));
      } else {
        resolve(message.result || {});
      }
    };
    await new Promise((resolve, reject) => {
      this.ws.onopen = resolve;
      this.ws.onerror = reject;
    });
  }

  send(method, params = {}) {
    const id = this.nextID;
    this.nextID += 1;
    this.ws.send(JSON.stringify({ id, method, params }));
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
    });
  }

  close() {
    this.ws.close();
  }
}

function terminalHTML(lines, cursorOnLastLine = false) {
  const rendered = lines.map((line, index) => {
    const prompt = line.startsWith('$ ');
    const klass = prompt ? 'prompt' : line.startsWith('time=') ? 'log' : line.startsWith('# ') ? 'comment' : '';
    const cursor = cursorOnLastLine && index === lines.length - 1 ? ' cursor' : '';
    return `<div class="line ${klass}${cursor}">${escapeHTML(line)}</div>`;
  }).join('');

  return `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<style>
  * { box-sizing: border-box; }
  body {
    margin: 0;
    width: ${width}px;
    height: ${height}px;
    overflow: hidden;
    background:
      radial-gradient(circle at 80% 0%, rgba(107, 193, 254, 0.16), transparent 34%),
      #08090d;
    color: #e7eef7;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  }
  .window {
    position: absolute;
    inset: 44px 58px;
    border: 1px solid rgba(255,255,255,0.12);
    border-radius: 10px;
    background: rgba(8, 10, 16, 0.94);
    box-shadow: 0 28px 80px rgba(0,0,0,0.45);
    overflow: hidden;
  }
  .bar {
    height: 38px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 14px;
    background: rgba(255,255,255,0.07);
    color: #9ca9b8;
    font: 13px -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  }
  .dot { width: 12px; height: 12px; border-radius: 999px; display: inline-block; }
  .red { background: #ff5f57; }
  .yellow { background: #ffbd2e; }
  .green { background: #28c840; }
  .title { margin-left: auto; margin-right: auto; transform: translateX(-30px); }
  .terminal {
    padding: 28px 32px;
    font-size: 20px;
    line-height: 1.5;
    white-space: pre-wrap;
  }
  .line { min-height: 30px; color: #d8e2ef; }
  .prompt { color: #ffffff; }
  .prompt::first-letter { color: #6bc1fe; }
  .log { color: #aebbd0; }
  .comment { color: #6bc1fe; }
  .cursor::after {
    content: "";
    display: inline-block;
    width: 10px;
    height: 22px;
    margin-left: 4px;
    background: #6bc1fe;
    transform: translateY(4px);
  }
</style>
</head>
<body>
  <div class="window">
    <div class="bar">
      <span class="dot red"></span><span class="dot yellow"></span><span class="dot green"></span>
      <span class="title">crd-schema-publisher demo</span>
    </div>
    <div class="terminal">${rendered}</div>
  </div>
</body>
</html>`;
}

async function navigateHTML(cdp, html) {
  const encoded = Buffer.from(html).toString('base64');
  await cdp.send('Page.navigate', { url: `data:text/html;base64,${encoded}` });
  await sleep(250);
}

async function navigate(cdp, url) {
  await cdp.send('Page.navigate', { url });
  await sleep(900);
}

async function evalJS(cdp, expression) {
  const result = await cdp.send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.text || 'Runtime.evaluate failed');
  }
  return result.result?.value;
}

async function screenshot(cdp, name, delay) {
  await evalJS(cdp, `(() => {
    if (document.getElementById('demo-frame-border')) return;
    const style = document.createElement('style');
    style.id = 'demo-frame-border';
    style.textContent = ${JSON.stringify(frameBorderCSS)};
    document.head.appendChild(style);
  })()`);
  const result = await cdp.send('Page.captureScreenshot', {
    format: 'png',
    fromSurface: true,
    captureBeyondViewport: false,
  });
  const file = `${frameDir}/${String(frames.length + 1).padStart(3, '0')}-${name}.png`;
  await writeFile(file, Buffer.from(result.data, 'base64'));
  frames.push({ file, delay });
}

async function focusSearch(cdp) {
  await evalJS(cdp, `(() => {
    const input = document.querySelector('#search');
    input.focus();
    input.setSelectionRange(input.value.length, input.value.length);
  })()`);
}

async function pressKey(cdp, key, code, keyCode) {
  await cdp.send('Input.dispatchKeyEvent', {
    type: 'keyDown',
    key,
    code,
    windowsVirtualKeyCode: keyCode,
    nativeVirtualKeyCode: keyCode,
  });
  await cdp.send('Input.dispatchKeyEvent', {
    type: 'keyUp',
    key,
    code,
    windowsVirtualKeyCode: keyCode,
    nativeVirtualKeyCode: keyCode,
  });
  await sleep(300);
}

async function typeSearch(cdp, selector, text, framePrefix) {
  for (let i = 1; i <= text.length; i += 1) {
    const value = text.slice(0, i);
    await evalJS(cdp, `(() => {
      const input = document.querySelector(${JSON.stringify(selector)});
      input.focus();
      input.value = ${JSON.stringify(value)};
      input.dispatchEvent(new Event('input', { bubbles: true }));
    })()`);
    await sleep(250);
    await screenshot(cdp, `${framePrefix}-${i}`, i === text.length ? 1.0 : 0.25);
  }
}

async function setSearchValue(cdp, value, frameName, delay = 0.8) {
  await evalJS(cdp, `(() => {
    const input = document.querySelector('#search');
    input.focus();
    input.value = ${JSON.stringify(value)};
    input.setSelectionRange(input.value.length, input.value.length);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  })()`);
  await sleep(300);
  await screenshot(cdp, frameName, delay);
}

async function captureTerminal(cdp) {
  const terminalStages = [];
  terminalStages.push(
    {
      lines: [
        '$ curl -fsSL https://crdsp.shold.io | bash',
      ],
      cursor: true,
      delay: 1.3,
    },
    {
      lines: [
        '$ curl -fsSL https://crdsp.shold.io | bash',
        ' crd-schema-publisher Installer',
        '===============================',
        '',
        ' Detected target darwin/arm64',
        ' Downloading checksum manifest for sholdee/crd-schema-publisher latest',
      ],
      delay: 1.4,
    },
    {
      lines: [
        '$ curl -fsSL https://crdsp.shold.io | bash',
        ' crd-schema-publisher Installer',
        '===============================',
        '',
        ' Detected target darwin/arm64',
        ' Downloaded release assets',
        ' Checksum verified',
        ' Cosign bundle verified',
      ],
      delay: 1.5,
    },
    {
      lines: [
        '$ curl -fsSL https://crdsp.shold.io | bash',
        ' Ready to install/update crd-schema-publisher',
        '',
        ' Release:       latest',
        ' Target:        /usr/local/bin/crd-schema-publisher',
        ' Binary asset:  crd-schema-publisher-darwin-arm64',
        ' Checksum:      verified',
        ' Cosign:        verified',
        '',
        'Continue with install/update? [Y/n] y',
      ],
      delay: 2.0,
    },
    {
      lines: [
        '$ curl -fsSL https://crdsp.shold.io | bash',
        ' Installing binary to /usr/local/bin/crd-schema-publisher',
        ' Installed /usr/local/bin/crd-schema-publisher',
        ' Installed version',
        installedVersion,
        ' Completed successfully.',
      ],
      delay: 1.6,
    },
  );

  appendTypedCommandStages(terminalStages, 'crd-schema-publisher extract -o ./output');
  terminalStages.push({
    lines: [
      '$ crd-schema-publisher extract -o ./output',
      `time=${extractTime} level=INFO msg="building kubernetes client"`,
      `time=${extractTime} level=INFO msg="extract complete" count=${schemaCount} dir=./output`,
    ],
    delay: 1.8,
  });

  appendTypedCommandStages(terminalStages, 'crd-schema-publisher preview -o ./output');
  terminalStages.push(
    {
      lines: [
        '$ crd-schema-publisher preview -o ./output',
        `time=${previewTime} level=INFO msg="rendering schema pages"`,
      ],
      delay: 0.9,
    },
    {
      lines: [
        '$ crd-schema-publisher preview -o ./output',
        `time=${previewTime} level=INFO msg="rendering schema pages"`,
        `time=${previewTime} level=INFO msg="generating index"`,
      ],
      delay: 0.9,
    },
    {
      lines: [
        '$ crd-schema-publisher preview -o ./output',
        `time=${previewTime} level=INFO msg="rendering schema pages"`,
        `time=${previewTime} level=INFO msg="generating index"`,
        `time=2026-05-18T00:43:03 level=INFO msg="serving preview" addr=${previewHost}`,
      ],
      delay: 1.8,
    },
  );

  for (let i = 0; i < terminalStages.length; i += 1) {
    const stage = terminalStages[i];
    await navigateHTML(cdp, terminalHTML(stage.lines, !!stage.cursor));
    await screenshot(cdp, `terminal-${i + 1}`, stage.delay);
  }
}

async function selectPreferredSchema(cdp) {
  return evalJS(cdp, `(() => {
    const preferred = document.querySelector('a[href*="grafana.integreatly.org/grafanadashboard_v1beta1.html"]')
      || document.querySelector('a[href*="grafanadashboard_v1beta1.html"]')
      || document.querySelector('a[href*="grafanadashboard"]')
      || [...document.querySelectorAll('details.group a')].find(link => !link.closest('details.group').hidden)
      || document.querySelector('details.group a');
    return preferred ? preferred.href : '';
  })()`);
}

async function captureIndexAndSchema(cdp) {
  await navigate(cdp, `${previewURL}/index.html`);
  await evalJS(cdp, `(() => {
    localStorage.setItem('theme', 'dark');
    localStorage.setItem('crd-schema-publisher:path-search-learned', '1');
    sessionStorage.clear();
  })()`);
  await screenshot(cdp, 'index', 1.1);
  await typeSearch(cdp, '#search', 'grafanadas', 'index-search');

  const href = await selectPreferredSchema(cdp);
  if (!href) {
    throw new Error('could not find a schema link to capture');
  }

  await navigate(cdp, href);
  await screenshot(cdp, 'schema-page', 1.1);
  await focusSearch(cdp);

  const hasInstanceSelector = await evalJS(cdp, `(() => {
    return !!document.querySelector('[data-path="spec.instanceSelector.matchExpressions[].operator"]');
  })()`);

  if (hasInstanceSelector) {
    await setSearchValue(cdp, '.', 'path-dot', 0.45);
    await setSearchValue(cdp, '.s', 'path-s-ghost', 0.6);
    await setSearchValue(cdp, '.sp', 'path-sp-ghost', 0.9);
    await pressKey(cdp, 'Tab', 'Tab', 9);
    await screenshot(cdp, 'path-accepted-spec', 0.9);
    await setSearchValue(cdp, '.spec.', 'path-spec-dot', 0.7);
    await pressKey(cdp, 'ArrowDown', 'ArrowDown', 40);
    await screenshot(cdp, 'path-cycle-down-1', 0.65);
    await pressKey(cdp, 'ArrowDown', 'ArrowDown', 40);
    await screenshot(cdp, 'path-cycle-down-2', 0.65);
    await pressKey(cdp, 'ArrowUp', 'ArrowUp', 38);
    await screenshot(cdp, 'path-cycle-up', 0.65);
    await setSearchValue(cdp, '.spec.in', 'path-instance-ghost', 0.85);
    await pressKey(cdp, 'Tab', 'Tab', 9);
    await screenshot(cdp, 'path-instance-accepted', 0.95);
    await setSearchValue(cdp, '.spec.instanceSelector.matchExpressions[].op', 'path-operator-ghost', 0.85);
    await pressKey(cdp, 'Tab', 'Tab', 9);
    await screenshot(cdp, 'schema-final', 1.8);
    return;
  }

  await setSearchValue(cdp, '.', 'path-dot', 0.45);
  await setSearchValue(cdp, '.s', 'path-s-ghost', 0.6);
  await setSearchValue(cdp, '.sp', 'path-sp-ghost', 0.9);
  await pressKey(cdp, 'Tab', 'Tab', 9);
  await screenshot(cdp, 'path-accepted-spec', 0.9);
  await setSearchValue(cdp, '.spec.', 'path-spec-dot', 0.9);
  await pressKey(cdp, 'ArrowDown', 'ArrowDown', 40);
  await screenshot(cdp, 'path-cycle-down', 0.8);
}

async function main() {
  if (typeof WebSocket !== 'function') {
    throw new Error('global WebSocket is unavailable; use Node.js 22 or newer');
  }

  await rm(frameDir, { recursive: true, force: true });
  await rm(profileDir, { recursive: true, force: true });
  await mkdir(frameDir, { recursive: true });

  const chrome = spawn(chromePath, [
    '--headless=new',
    '--disable-gpu',
    '--hide-scrollbars',
    '--no-first-run',
    '--no-default-browser-check',
    `--window-size=${width},${height}`,
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${profileDir}`,
    'about:blank',
  ], { stdio: 'ignore' });

  try {
    const wsURL = await waitForTargets();
    const cdp = new CDP(wsURL);
    await cdp.connect();
    await cdp.send('Page.enable');
    await cdp.send('Runtime.enable');
    await cdp.send('Emulation.setDeviceMetricsOverride', {
      width,
      height,
      deviceScaleFactor: 1,
      mobile: false,
    });

    await captureTerminal(cdp);
    await captureIndexAndSchema(cdp);

    cdp.close();
  } finally {
    chrome.kill('SIGTERM');
  }

  const manifest = frames.map(frame => `${frame.file}\t${frame.delay}`).join('\n') + '\n';
  await writeFile(`${frameDir}/frames.tsv`, manifest);
  console.log(`${frameDir}/frames.tsv`);
}

main().catch(err => {
  console.error(err);
  process.exit(1);
});
