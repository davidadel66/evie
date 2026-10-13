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
  // evaluate throws when the page's expression throws or its promise rejects.
  const evaluate = async expression => {
    const response = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    const failure = response.result?.exceptionDetails;
    if (failure) throw new Error(failure.exception?.description || failure.text);
    return response.result?.result?.value;
  };
  const open = async url => { await send('Page.navigate', { url }); await sleep(1500); };
  return { send, evaluate, open, close: () => socket.close() };
}

// watchFrameViolations records the page's own frame-src refusals, which is
// where a tab's attempt to navigate its frame away is reported.
const watchFrameViolations = `window.__frameViolations = [];
  document.addEventListener('securitypolicyviolation', event => window.__frameViolations.push(event.effectiveDirective + ' ' + event.blockedURI));`;

// assertNavigationRefused waits for Evie's page to refuse the tab's attempt
// to navigate its frame to the outside stand-in, then checks that no
// request of any kind reached it.
async function assertNavigationRefused(where, page, base) {
  const refused = await page.evaluate(`new Promise(resolve => {
    let waited = 0;
    const poll = setInterval(() => {
      const seen = window.__frameViolations.find(entry => entry.startsWith('frame-src http://localhost:'));
      if (seen || (waited += 100) > 5000) { clearInterval(poll); resolve(seen || null); }
    }, 100);
  })`);
  assert.ok(refused, `${where}: Evie's page did not refuse the tab's navigation`);
  await sleep(500);
  assert.equal(await (await fetch(`${base}/fixture/leaks`)).text(), '0', `${where}: a request reached the outside`);
  return refused;
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
  for (const name of ['apiFetch', 'externalFetch', 'webSocket', 'beacon', 'imageLeak', 'otherTabScript', 'localStorage', 'popup']) {
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
  await page.send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 850, deviceScaleFactor: 1, mobile: false });

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
  await page.evaluate(watchFrameViolations);
  const framed = await page.evaluate(framedReport(tabURL));
  assertSealed('framed by Evie', framed);
  assert.match(String(framed.parentDocument), /^blocked/, `framed: parentDocument = ${framed.parentDocument}`);
  assert.match(String(framed.parentStorage), /^blocked/, `framed: parentStorage = ${framed.parentStorage}`);
  await sleep(500);
  assert.notEqual(await page.evaluate('location.hash'), '#tab-escaped', 'framed: the tab navigated Evie');
  assert.equal(await pageCount(port), pagesBefore, 'framed: the tab opened a window');
  const framedNavigation = await assertNavigationRefused('framed', page, base);
  results.push({ case: 'framed by Evie', report: framed, navigation: framedNavigation });

  // 3. Framed by another origin (localhost is not 127.0.0.1): refused.
  const foreign = base.replace('127.0.0.1', 'localhost');
  await page.open(`${foreign}/`);
  const stranger = await page.evaluate(framedReport(tabURL));
  assert.equal(stranger, null, 'a foreign page framed the tab');
  results.push({ case: 'framed by another origin', report: 'refused' });

  // 4. In Evie itself: open the Workspace and its Hostile tab.
  const until = (expression, what) => page.evaluate(`new Promise((resolve, reject) => {
    let waited = 0;
    const poll = setInterval(() => {
      const value = (() => { ${expression} })();
      if (value) { clearInterval(poll); resolve(value); }
      else if ((waited += 100) > 10000) { clearInterval(poll); reject(new Error(${JSON.stringify(what)} + ' at ' + location.href + ' — buttons: ' + [...document.querySelectorAll('button')].map(b => b.textContent.trim() + (b.offsetParent ? '' : '(hidden)')).join('|'))); }
    }, 100);
  })`);
  const clickText = (selector, text) => until(`const element = [...document.querySelectorAll(${JSON.stringify(selector)})]
    .find(candidate => candidate.offsetParent !== null && candidate.textContent.trim() === ${JSON.stringify(text)});
    if (element) element.click();
    return !!element;`, `no ${selector} "${text}"`);
  await page.open(`${base}/`);
  await clickText('#evie-navigation button', 'Lab');
  await page.evaluate(`window.__tabReport = null; addEventListener('message', event => { if (event.data && event.data.hostileTab) window.__tabReport = event.data.hostileTab; })`);
  await page.evaluate(watchFrameViolations);
  await clickText('[role=tab]', 'Hostile');
  const inEvie = await until('return window.__tabReport', 'the Hostile tab sent no report inside Evie');
  assertSealed('inside Evie', inEvie);
  assert.match(String(inEvie.parentDocument), /^blocked/, `inside Evie: parentDocument = ${inEvie.parentDocument}`);
  assert.equal(await page.evaluate(`document.querySelector('#workspace-tab-panel iframe').getAttribute('sandbox')`), 'allow-scripts');
  assert.notEqual(await page.evaluate('location.hash'), '#tab-escaped', 'inside Evie: the tab navigated Evie');
  const appNavigation = await assertNavigationRefused('inside Evie', page, base);
  results.push({ case: 'inside Evie', report: inEvie, navigation: appNavigation });

  // 5. Story 2: the Data tab reads nothing until the owner allows its data
  // in Settings → Tabs, then reads what it declared and nothing else; the
  // Thief tab it navigates its frame to, with its key, gets nothing.
  await page.evaluate(`window.__tabMessages = []; addEventListener('message', event => { if (event.data && (event.data.dataTab || event.data.thief)) window.__tabMessages.push(event.data); })`);
  await clickText('[role=tab]', 'Data');
  const before = await until(`const found = window.__tabMessages.find(message => message.dataTab); return found && found.dataTab;`, 'the Data tab sent no report before approval');
  assert.match(String(before.file), /^refused: .*until you allow its data/, `before approval: file = ${before.file}`);
  assert.match(String(before.query), /^refused/, `before approval: query = ${before.query}`);
  await clickText('[role=tab]', 'Settings');
  await clickText('button', 'Allow this data');
  await until(`return [...document.querySelectorAll('section[aria-label=Tabs] span')].some(span => span.textContent === 'Allowed')`, 'the Data tab was not allowed');
  await page.evaluate('window.__tabMessages = []');
  await clickText('[role=tab]', 'Data');
  const after = await until(`const found = window.__tabMessages.find(message => message.dataTab); return found && found.dataTab;`, 'the Data tab sent no report after approval');
  assert.equal(after.file, '{"loss":1}', `after approval: file = ${after.file}`);
  assert.deepEqual(after.query, { columns: ['n'], rows: [[1]] }, `after approval: query = ${JSON.stringify(after.query)}`);
  assert.match(String(after.undeclared), /^refused: tab.json does not declare the database/, `after approval: undeclared = ${after.undeclared}`);
  const thief = await until(`const found = window.__tabMessages.find(message => message.thief); return found && found.thief;`, 'the Thief tab sent no report');
  assert.equal(thief, 'no answer', `the Thief tab got: ${thief}`);
  results.push({ case: 'data channel', report: { before, after, thief } });

  // 6. At phone width, tabs that do not fit are in More and nothing scrolls.
  await page.send('Emulation.setDeviceMetricsOverride', { width: 360, height: 780, deviceScaleFactor: 2, mobile: true });
  await page.open(`${base}/`);
  await until(`const open = document.querySelector('button[aria-label="Open navigation"]'); if (open) open.click(); return true;`, 'no navigation');
  await clickText('#evie-navigation button', 'Lab');
  await until(`return document.querySelector('[aria-haspopup=menu]')`, 'no More menu at phone width');
  const phone = JSON.parse(await page.evaluate(`JSON.stringify({
    shown: [...document.querySelectorAll('[role=tab]')].map(tab => tab.textContent.trim()),
    pageScrolls: document.documentElement.scrollWidth > innerWidth,
    headerOverflows: (() => { const list = document.querySelector('[role=tablist][aria-label="Workspace sections"]'); return list.scrollWidth > list.clientWidth || list.getBoundingClientRect().right > innerWidth; })(),
  })`));
  assert.equal(phone.pageScrolls, false, 'the page scrolls sideways at phone width');
  assert.equal(phone.headerOverflows, false, `the header overflows at phone width: ${phone.shown}`);
  assert.ok(phone.shown.includes('Settings') && !phone.shown.includes('Budget'), `phone tabs: ${phone.shown}`);
  await clickText('[aria-haspopup=menu]', 'More');
  await clickText('[role=menuitem]', 'Budget');
  const picked = await until(`const tab = [...document.querySelectorAll('[role=tab]')].find(candidate => candidate.textContent.trim() === 'Budget');
    return tab && tab.getAttribute('aria-selected') === 'true' && tab.getBoundingClientRect().right <= innerWidth ? 'selected and visible' : '';`, 'Budget did not become the visible selected tab');
  results.push({ case: 'phone width', report: { shown: phone.shown, budgetFromMore: picked } });

  assert.equal(await (await fetch(`${base}/fixture/leaks`)).text(), '0', 'a request reached the outside');
  results.push({ case: 'requests that reached the outside', report: 0 });
  page.close();
  console.log(JSON.stringify({ ok: true, results }, null, 2));
} catch (error) {
  console.error(JSON.stringify({ ok: false, error: String(error && error.message || error), results }, null, 2));
  process.exitCode = 1;
} finally {
  await cleanup();
}
