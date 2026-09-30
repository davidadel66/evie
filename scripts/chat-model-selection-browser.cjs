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
  const selector = page.getByRole('combobox', { name: 'Chat model' });
  await page.waitForFunction(() => document.querySelector('select[aria-label="Chat model"]')?.value === 'openai/test');
  assert.deepEqual(await selector.locator('optgroup').evaluateAll(nodes => nodes.map(node => node.label)), ['Anthropic', 'broken', 'OpenAI']);
  await selector.focus();
  assert.equal(await selector.evaluate(element => document.activeElement === element), true);
  report.cases.push('Accessible selector grouped by provider');

  const draft = page.getByRole('textbox');
  await draft.fill('Keep this draft while switching models.');
  await selector.selectOption('anthropic/test');
  await page.waitForFunction(() => document.querySelector('select[aria-label="Chat model"]')?.value === 'anthropic/test');
  assert.equal(await draft.inputValue(), 'Keep this draft while switching models.');
  await page.reload();
  await page.waitForFunction(() => document.querySelector('select[aria-label="Chat model"]')?.value === 'anthropic/test');
  report.cases.push('Selection preserves draft and survives reload');

  await draft.fill('Draft survives a failed model selection.');
  await selector.selectOption('broken/test');
  await page.getByRole('alert').filter({ hasText: 'This model could not be selected' }).waitFor();
  assert.equal(await selector.inputValue(), 'anthropic/test');
  assert.equal(await draft.inputValue(), 'Draft survives a failed model selection.');
  report.cases.push('Failed selection retains current model and draft');
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await page.waitForFunction(() => !document.querySelector('select[aria-label="Chat model"]').disabled);

  let release;
  const gate = new Promise(resolve => { release = resolve; });
  await page.route('**/api/chat', async route => { await gate; await route.continue(); });
  await draft.fill('Say hello from the chosen model.');
  await draft.press('Enter');
  await page.waitForFunction(() => document.querySelector('select[aria-label="Chat model"]').disabled);
  release();
  await page.getByText('resumed', { exact: true }).waitFor();
  await page.unroute('**/api/chat');
  await page.waitForFunction(() => !document.querySelector('select[aria-label="Chat model"]').disabled);
  report.cases.push('Selector is disabled during a reply; chosen model can send');

  await post('/api/models/select', { sessionId: fixture.sessionId, model: 'openai/test', revision: 1 });
  await draft.fill('A stale tab must not silently use a different model.');
  await draft.press('Enter');
  await page.getByRole('button', { name: 'Retry', exact: true }).waitFor();
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await page.waitForFunction(() => document.querySelector('select[aria-label="Chat model"]')?.value === 'openai/test');
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
  await page.waitForFunction(() => document.querySelector('select[aria-label="Chat model"]')?.value === 'anthropic/test');
  assert.equal(await draft.inputValue(), 'Queued message one.\n\nQueued message two.');
  assert.equal(await page.getByRole('button', { name: 'Retry', exact: true }).count(), 0);
  report.cases.push('Parked queue returns to draft and recovers from model conflict without losing text');

  await page.screenshot({ path: path.join(output, 'desktop.png') });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: path.join(output, 'mobile.png') });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  const box = await selector.boundingBox();
  assert.ok(box && box.x >= 0 && box.x + box.width <= 390);
  report.cases.push('Selector fits a 390px viewport');
  assert.deepEqual(report.pageErrors, []);
  report.status = 'passed';
  fs.writeFileSync(path.join(output, 'browser-report.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2));
})().catch(error => { console.error(error); process.exitCode = 1; }).finally(async () => {
  if (browser) await browser.close();
  server.stdin.end('\n');
});
