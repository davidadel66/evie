#!/usr/bin/env node
// Browser check that a Workspace tab is sealed (workspace-tabs.spec.md,
// story 1). It serves Evie with the hostile test tab
// (internal/web/testdata/workspace-tabs/hostile) and drives headless Chrome
// over the DevTools protocol, with no npm dependency:
//
//   node scripts/workspace-tabs-browser.mjs
//
// Needs Chrome (EVIE_CHROME_PATH overrides the macOS default) and a built UI
// (npm --prefix internal/web/ui run build), since the fixture embeds it.
import { spawn, execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const work = mkdtempSync(path.join(tmpdir(), 'evie-workspace-tabs-'));
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const children = [];
// cleanup stops the fixture and Chrome and waits for them to exit before
// removing their files, which Chrome writes to until it is gone.
const cleanup = async () => {
  await Promise.all(children.map(child => new Promise(resolve => {
    if (child.exitCode !== null || child.signalCode !== null) return resolve();
    child.once('exit', resolve);
    child.kill();
    setTimeout(resolve, 5000);
  })));
  rmSync(work, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
};

async function startFixture() {
  const binary = path.join(work, 'web.test');
  execFileSync('go', ['test', '-c', '-o', binary, './internal/web'], { cwd: root, stdio: 'inherit' });
  const server = spawn(binary, ['-test.run=^TestWorkspaceTabsBrowserFixture$', '-test.timeout=5m'], {
    cwd: path.join(root, 'internal/web'), env: { ...process.env, EVIE_WORKSPACE_TABS_BROWSER_FIXTURE: '1' }, stdio: ['pipe', 'pipe', 'inherit'],
  });
  children.push(server);
  return new Promise((resolve, reject) => {
    let text = '';
    const timer = setTimeout(() => reject(new Error('fixture startup timed out')), 60000);
    server.stdout.on('data', chunk => {
      text += chunk;
      const match = text.match(/WORKSPACE_TABS_BROWSER_READY=(\S+)/);
      if (match) { clearTimeout(timer); resolve(match[1]); }
    });
    server.once('exit', code => { clearTimeout(timer); reject(new Error(`fixture exited: ${code}`)); });
  });
}

async function startChrome() {
  const profile = path.join(work, 'chrome');
  const chrome = spawn(process.env.EVIE_CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    ['--headless=new', '--remote-debugging-port=0', `--user-data-dir=${profile}`, '--no-first-run', 'about:blank'], { stdio: 'ignore' });
  children.push(chrome);
  for (let i = 0; i < 100; i++) {
    await sleep(100);
    try { return Number(readFileSync(path.join(profile, 'DevToolsActivePort'), 'utf8').split('\n')[0]); } catch { /* not yet */ }
  }
  throw new Error('Chrome did not start');
}

async function connect(port) {
  const pages = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const socket = new WebSocket(pages.find(page => page.type === 'page').webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
  let next = 0;
  const waiting = new Map();
  socket.onmessage = event => {
    const message = JSON.parse(event.data);
    waiting.get(message.id)?.(message);
    waiting.delete(message.id);
  };
  const send = (method, params = {}) => new Promise(resolve => {
    const id = ++next;
    waiting.set(id, resolve);
    socket.send(JSON.stringify({ id, method, params }));
  });
  const evaluate = async expression => (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result?.result?.value;
  const open = async url => { await send('Page.navigate', { url }); await sleep(1500); };
  return { send, evaluate, open, close: () => socket.close() };
}

// pageCount is how many page targets Chrome has, to see a popup.
async function pageCount(port) {
  return (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()).filter(page => page.type === 'page').length;
}

// framedReport frames the tab from the current page as Evie will (sandbox
// allow-scripts only) and waits for its report.
const framedReport = tabURL => `new Promise(resolve => {
  addEventListener('message', event => { if (event.data && event.data.hostileTab) resolve(event.data.hostileTab); });
  const frame = document.createElement('iframe');
  frame.setAttribute('sandbox', 'allow-scripts');
  frame.src = ${JSON.stringify(tabURL)};
  document.body.appendChild(frame);
  setTimeout(() => resolve(null), 25000);
})`;

function assertSealed(where, report) {
  assert.ok(report, `${where}: the tab sent no report`);
  assert.equal(report.inlineScript, true, `${where}: inline script ran`);
  assert.equal(report.externalScript, true, `${where}: the tab's own app.js ran`);
  assert.equal(report.origin, 'null', `${where}: the tab has an opaque origin`);
  for (const name of ['apiFetch', 'externalFetch', 'webSocket', 'beacon', 'imageLeak', 'localStorage', 'popup']) {
    assert.match(String(report[name]), /^blocked/, `${where}: ${name} = ${report[name]}`);
  }
}

const results = [];
try {
  const base = await startFixture();
  const tabURL = `${base}/workspace-tabs/lab/hostile/`;
  const port = await startChrome();
  const page = await connect(port);
  await page.send('Page.enable');
  await page.send('Runtime.enable');

  // 1. Opened directly: the response's own sandbox applies.
  await page.open(tabURL);
  const direct = JSON.parse(await page.evaluate(`new Promise(resolve => {
    const poll = setInterval(() => {
      const text = document.getElementById('report').textContent;
      if (text !== 'running') { clearInterval(poll); resolve(text); }
    }, 100);
  })`));
  assertSealed('opened directly', direct);
  results.push({ case: 'opened directly', report: direct });

  // 2. Framed by Evie's page, as the Workspace header will.
  await page.open(`${base}/`);
  const pagesBefore = await pageCount(port);
  const framed = await page.evaluate(framedReport(tabURL));
  assertSealed('framed by Evie', framed);
  assert.match(String(framed.parentDocument), /^blocked/, `framed: parentDocument = ${framed.parentDocument}`);
  assert.match(String(framed.parentStorage), /^blocked/, `framed: parentStorage = ${framed.parentStorage}`);
  await sleep(500);
  assert.notEqual(await page.evaluate('location.hash'), '#tab-escaped', 'framed: the tab navigated Evie');
  assert.equal(await pageCount(port), pagesBefore, 'framed: the tab opened a window');
  results.push({ case: 'framed by Evie', report: framed });

  // 3. Framed by another origin (localhost is not 127.0.0.1): refused.
  const foreign = base.replace('127.0.0.1', 'localhost');
  await page.open(`${foreign}/`);
  const stranger = await page.evaluate(framedReport(tabURL));
  assert.equal(stranger, null, 'a foreign page framed the tab');
  results.push({ case: 'framed by another origin', report: 'refused' });

  page.close();
  console.log(JSON.stringify({ ok: true, results }, null, 2));
} catch (error) {
  console.error(JSON.stringify({ ok: false, error: String(error && error.message || error), results }, null, 2));
  process.exitCode = 1;
} finally {
  await cleanup();
}
