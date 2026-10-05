#!/usr/bin/env node
// Requires an externally installed playwright-core and Chrome; no app dependency.
const { chromium } = require(process.env.EVIE_PLAYWRIGHT_MODULE || 'playwright-core');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');

const root = path.resolve(__dirname, '..');
const output = path.join(root, '.scratch/chat-model-selection');
const server = spawn(path.join(output, 'browser.test'), ['-test.run=^TestChatModelsBrowserFixture$', '-test.timeout=2m'], {
  cwd: root, env: { ...process.env, EVIE_MODELS_BROWSER_FIXTURE: '1' }, stdio: ['pipe', 'pipe', 'pipe'],
});
let browser;
const report = { cases: [], pageErrors: [] };

(async () => {
  const fixture = await new Promise((resolve, reject) => {
    let text = '';
    const timer = setTimeout(() => reject(new Error('Fixture startup timed out')), 30000);
    server.stdout.on('data', chunk => {
      text += chunk;
      const match = text.match(/MODELS_BROWSER_READY=(\S+) (\S+)/);
      if (match) { clearTimeout(timer); resolve({ url: match[1], sessionId: match[2] }); }
    });
    server.once('exit', code => { clearTimeout(timer); reject(new Error(`Fixture exited: ${code}`)); });
    server.stderr.on('data', chunk => process.stderr.write(chunk));
  });
  browser = await chromium.launch({ executablePath: process.env.EVIE_CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 850 } });
  page.on('pageerror', error => report.pageErrors.push(error.message));
  const post = async (route, body) => {
    const response = await page.request.post(fixture.url + route, { data: body, headers: { Origin: fixture.url } });
    assert.equal(response.status(), 200, await response.text());
    return response.json();
  };
  await post('/api/context-sessions/select', { sessionId: fixture.sessionId });
  await page.goto(fixture.url);
  const picker = page.getByRole('dialog', { name: 'Choose model' });
  const menu = page.getByRole('listbox', { name: 'Commands' });
  const draft = page.locator('textarea');
  const openPicker = async () => { await draft.fill('/model'); await draft.press('Enter'); await picker.waitFor(); };
  const choose = async name => { await openPicker(); await picker.getByRole('button', { name, exact: true }).click(); };
  // The marked model in the picker is the only place the UI names it.
  const currentModel = async name => {
    let seen;
    for (let attempt = 0; attempt < 50; attempt++) {
      if (await draft.isEnabled()) {
        await openPicker();
        seen = await picker.locator('[aria-current="true"]').textContent();
        await page.keyboard.press('Escape');
        await picker.waitFor({ state: 'detached' });
        if (seen === name) return;
      }
      await page.waitForTimeout(100);
    }
    throw new Error(`Current model stayed ${seen}, expected ${name}`);
  };

  await draft.fill('/');
  await menu.waitFor();
  assert.deepEqual(await menu.getByRole('option').evaluateAll(nodes => nodes.map(node => node.firstElementChild.textContent)), ['/model', '/compact']);
  await draft.press('ArrowDown');
  assert.equal(await menu.getByRole('option', { selected: true }).evaluate(node => node.firstElementChild.textContent), '/compact');
  await draft.press('Escape');
  await menu.waitFor({ state: 'detached' });
  assert.equal(await draft.inputValue(), '/');
  await draft.fill('/m');
  await menu.waitFor();
  await page.screenshot({ path: path.join(output, 'desktop-commands.png') });
  await draft.press('Tab');
  assert.equal(await draft.inputValue(), '/model');
  await draft.press('Enter');
  await picker.waitFor();
  assert.equal(await draft.inputValue(), '');
  assert.equal(await page.getByRole('button', { name: /^Chat model/ }).count(), 0);
  assert.deepEqual(await picker.getByRole('group').evaluateAll(nodes => nodes.map(node => node.getAttribute('aria-label'))), ['Anthropic', 'broken', 'OpenAI']);
  assert.equal(await picker.locator('[aria-current="true"]').textContent(), 'GPT Test');
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('aria-label')), 'Search models');
  await page.keyboard.type('claude');
  await page.keyboard.press('ArrowDown');
  assert.equal(await page.evaluate(() => document.activeElement?.textContent), 'Claude Test');
  await page.screenshot({ path: path.join(output, 'desktop-picker.png') });
  await page.keyboard.press('Escape');
  await picker.waitFor({ state: 'detached' });
  assert.equal(await page.evaluate(() => document.activeElement?.tagName), 'TEXTAREA');
  assert.equal(await page.getByText('/model', { exact: true }).count(), 0);
  report.cases.push('Slash menu filters, navigates, dismisses and completes /model; picker is searchable, keyboard-driven and unsent');

  await choose('Claude Test');
  await currentModel('Claude Test');
  await page.reload();
  await currentModel('Claude Test');
  report.cases.push('Selection survives reload');

  await choose('Unavailable Test');
  await page.getByRole('alert').filter({ hasText: 'This model could not be selected' }).waitFor();
  await draft.fill('Draft survives a failed model selection.');
  assert.equal(await draft.isEnabled(), true);
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await page.getByRole('button', { name: 'Retry', exact: true }).waitFor({ state: 'detached' });
  assert.equal(await draft.inputValue(), 'Draft survives a failed model selection.');
  await currentModel('Claude Test');
  report.cases.push('Failed selection retains current model and later draft');

  let release;
  const gate = new Promise(resolve => { release = resolve; });
  await page.route('**/api/chat', async route => { await gate; await route.continue(); });
  await draft.fill('Say hello from the chosen model.');
  await draft.press('Enter');
  await openPicker();
  await picker.getByText('The model can change once the current reply finishes.').waitFor();
  assert.equal(await picker.locator('button[data-model]:not(:disabled)').count(), 0);
  await page.keyboard.press('Escape');
  await picker.waitFor({ state: 'detached' });
  release();
  await page.getByText('resumed', { exact: true }).waitFor();
  await page.unroute('**/api/chat');
  assert.equal(await page.getByText('/model', { exact: true }).count(), 0);
  report.cases.push('/model during a reply opens a disabled picker without queueing; chosen model can send');

  await post('/api/models/select', { sessionId: fixture.sessionId, model: 'openai/test', revision: 1 });
  await draft.fill('A stale tab must not silently use a different model.');
  await draft.press('Enter');
  await page.getByRole('button', { name: 'Retry', exact: true }).waitFor();
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await currentModel('GPT Test');
  report.cases.push('Cross-tab stale send is rejected and Retry loads current model');

  let releaseQueued;
  const queuedGate = new Promise(resolve => { releaseQueued = resolve; });
  await page.route('**/api/chat', async route => { await queuedGate; await route.continue(); });
  await draft.fill('The turn that becomes stale.');
  await draft.press('Enter');
  await draft.fill('Queued message one.');
  await draft.press('Enter');
  await draft.fill('Queued message two.');
  await draft.press('Enter');
  await post('/api/models/select', { sessionId: fixture.sessionId, model: 'anthropic/test', revision: 2 });
  releaseQueued();
  const returnQueue = page.getByRole('button', { name: 'Return queued messages to draft' });
  await returnQueue.waitFor();
  await returnQueue.click();
  assert.equal(await draft.inputValue(), 'Queued message one.\n\nQueued message two.');
  await page.unroute('**/api/chat');
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await page.getByRole('button', { name: 'Retry', exact: true }).waitFor({ state: 'detached' });
  assert.equal(await draft.inputValue(), 'Queued message one.\n\nQueued message two.');
  await page.screenshot({ path: path.join(output, 'desktop.png') });
  await currentModel('Claude Test');
  report.cases.push('Parked queue returns to draft and recovers from model conflict without losing text');

  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: path.join(output, 'mobile.png') });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await draft.fill('/');
  const menuBox = await menu.boundingBox();
  assert.ok(menuBox && menuBox.x >= 0 && menuBox.y >= 0 && menuBox.x + menuBox.width <= 390);
  await openPicker();
  const pickerBox = await picker.boundingBox();
  assert.ok(pickerBox && pickerBox.x >= 0 && pickerBox.y >= 0 && pickerBox.x + pickerBox.width <= 390);
  await page.screenshot({ path: path.join(output, 'mobile-picker.png') });
  report.cases.push('Command menu and picker fit a 390px viewport');
  assert.deepEqual(report.pageErrors, []);
  report.status = 'passed';
  fs.writeFileSync(path.join(output, 'browser-report.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2));
})().catch(error => { console.error(error); process.exitCode = 1; }).finally(async () => {
  if (browser) await browser.close();
  server.stdin.end('\n');
});
