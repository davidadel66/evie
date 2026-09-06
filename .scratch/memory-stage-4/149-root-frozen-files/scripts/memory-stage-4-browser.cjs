#!/usr/bin/env node
// Scripted real-browser conformance. Install playwright-core outside the repo
// and point EVIE_PLAYWRIGHT_MODULE at it; no production dependency is required.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const assert = require('node:assert/strict');
const {spawn, execFileSync} = require('node:child_process');
const {chromium} = require(process.env.EVIE_PLAYWRIGHT_MODULE || 'playwright-core');

const root = path.resolve(__dirname, '..');
const output = path.resolve(process.argv[2] || path.join(root, '.scratch/memory-stage-4-browser'));
fs.mkdirSync(output, {recursive: true});
const report = {
  version: 'memory-stage-4-browser-v1', status: 'failed', synthetic: true, owner_pilot: false,
  cases: [], responses: [], page_errors: [],
  provenance: {
    web_interactions: 'Real Chrome, React and guarded HTTP: exact closed-source review, evidence, preview, acceptance, recorded operation and reload for all seven scopes. The accepted global graph and provenance are also browsed.',
    store_checks: 'After browser completion, actual browser operation IDs and hashes bind owner operation inspection in all seven scopes. Active observers read global, Workspace and project Claims and Source Links through the public Kernel. Exact closed-session destinations use temporary projection assertions against accepted operations. Canonical replay verifies accepted state without model calls. All sources stay closed and private acceptance never enters global reads. These are not graphical reads in every scope.',
  },
};
let server, serverDone, browser, page, fixture, verified, binary, stdout = '';
const fingerprint = () => execFileSync('python3', [path.join(root, 'scripts/memory-stage-4-conformance.py'), '--fingerprint'], {cwd: root, encoding: 'utf8'}).trim();

(async () => {
  report.source_sha256 = fingerprint();
  report.driver_sha256 = crypto.createHash('sha256').update(fs.readFileSync(__filename)).digest('hex');
  binary = path.join(output, 'browser-fixture.test');
  const buildLog = fs.openSync(path.join(output, 'build.log'), 'w');
  try {
    execFileSync('npm', ['--prefix', 'internal/web/ui', 'run', 'build'], {cwd: root, stdio: ['ignore', buildLog, buildLog]});
    execFileSync('go', ['test', '-c', '-o', binary, './cmd/evie'], {cwd: root, stdio: ['ignore', buildLog, buildLog]});
  } finally { fs.closeSync(buildLog); }
  server = spawn(binary, ['-test.run=^TestStage4BrowserFixture$', '-test.timeout=3m'], {
    cwd: root, env: {...process.env, EVIE_STAGE4_BROWSER_FIXTURE: '1'}, stdio: ['pipe', 'pipe', 'pipe'],
  });
  serverDone = new Promise(resolve => server.once('exit', (code, signal) => resolve({code, signal})));
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Browser fixture did not become ready')), 30000);
    server.stdout.on('data', chunk => {
      stdout += chunk;
      const ready = stdout.match(/STAGE4_BROWSER_READY=(.+)/);
      if (ready && !fixture) { fixture = JSON.parse(ready[1]); clearTimeout(timer); resolve(); }
      const proof = stdout.match(/STAGE4_BROWSER_VERIFIED=(.+)/);
      if (proof) verified = JSON.parse(proof[1]);
    });
    server.stderr.on('data', chunk => { report.server_stderr = (report.server_stderr || '') + chunk; });
    server.once('exit', code => { clearTimeout(timer); reject(new Error('Fixture exited before readiness: ' + code)); });
  });
  browser = await chromium.launch({executablePath: process.env.EVIE_CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true});
  report.browser_version = browser.version();
  page = await (await browser.newContext({viewport: {width: 1440, height: 1100}})).newPage();
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => report.page_errors.push(error.message));
  page.on('response', response => {
    if (response.url().includes('/api/memory/')) report.responses.push({path: response.url().split('/api/memory/')[1], status: response.status()});
  });
  const button = name => page.getByRole('button', {name, exact: typeof name === 'string'});
  const request = async (action, route) => {
    const pending = page.waitForResponse(response => response.url().endsWith('/api/memory/' + route) && response.request().method() === 'POST');
    const [response] = await Promise.all([pending, action()]);
    const data = await response.json();
    assert.equal(response.status(), 200, JSON.stringify(data));
    await page.waitForFunction(() => !Array.from(document.querySelectorAll('[role=status]')).some(node => /Loading the current review/.test(node.textContent)));
    return data;
  };
  const press = (name, route) => request(() => button(name).click(), 'candidates/' + route);
  await page.goto(fixture.url);
  await button('Memory').click();
  await request(() => page.getByLabel(/^Memory scope/).selectOption('global'), 'objects');
  const before = await request(() => button('Claims').click(), 'objects');
  assert.equal(before.objects.length, 1);
  assert.equal(before.objects[0].claim.object.literal.value, 'tea');
  report.unaccepted_global_isolation = true;
  await button('Review candidates').click();
  for (const entry of fixture.cases) {
    const listing = await request(() => page.getByLabel(/^Review scope/).selectOption(entry.scope), 'candidates/list');
    assert.equal(listing.scope_key, entry.scope);
    assert.equal(listing.candidates.length, 1);
    assert.equal(listing.candidates[0].ref.candidate_id, entry.candidate.ref.candidate_id);
    const item = await press(/^café\s+unresolved/, 'inspect');
    assert.deepEqual(item, entry.candidate);
    const preview = await press('Preview acceptance', 'prepare');
    assert.equal(preview.scope_key, entry.scope);
    assert.equal(preview.effect.scope.scope_key, entry.scope);
    assert.equal(preview.effect.claims[0].claim.scope_key, entry.scope);
    assert.deepEqual(preview.candidates[0].candidate.support, entry.candidate.candidate.support);
    assert((await page.getByRole('article', {name: 'Exact review preview'}).innerText()).includes('Review the exact memory change'));
    await page.getByLabel('Optional reason', {exact: true}).fill('Scripted browser conformance; not a David pilot observation.');
    const operationPending = page.waitForResponse(response => response.url().endsWith('/api/memory/candidates/operation') && response.request().method() === 'POST');
    const [operationResponse, resolution] = await Promise.all([operationPending, press('Accept this exact memory', 'resolve')]);
    assert.equal(operationResponse.status(), 200);
    const operation = await operationResponse.json();
    assert.equal(resolution.preview_id, preview.preview_id);
    assert.equal(operation.operation_id, resolution.operation.operation_id);
    assert.equal(operation.preview.preview_sha256, preview.preview_sha256);
    assert.equal(operation.preview.effect_sha256, preview.effect_sha256);
    await page.getByRole('heading', {name: 'Recorded decision: accepted', exact: true}).waitFor();
    assert((await page.getByRole('article', {name: 'Recorded review decision'}).innerText()).includes('Accepted provenance'));
    report.cases.push({name: entry.name, scope: entry.scope, candidate_id: item.ref.candidate_id,
      preview_sha256: preview.preview_sha256, effect_sha256: preview.effect_sha256, operation_id: operation.operation_id,
      checks: {scope: true, evidence: true, preview: true, resolution: true, reload: false, no_implicit_promotion: false, accepted_graph: false}});
    await page.reload();
    await button('Memory').click();
    await button('Review candidates').click();
    const after = await request(() => page.getByLabel(/^Review scope/).selectOption(entry.scope), 'candidates/list');
    assert.equal(after.candidates.length, 0);
    assert((await page.locator('body').innerText()).includes('No unresolved candidates in this page.'));
    report.cases.at(-1).checks.reload = true;
  }
  await button('Accepted memory').click();
  await request(() => page.getByLabel(/^Memory scope/).selectOption('global'), 'objects');
  const global = await request(() => button('Claims').click(), 'objects');
  assert.equal(global.objects.length, 2);
  const accepted = global.objects.find(object => object.claim.object.literal.value === 'café');
  assert(accepted);
  const detail = await request(() => button(new RegExp(accepted.object_id)).click(), 'inspect');
  assert.equal(detail.object_id, accepted.object_id);
  assert(detail.sources.some(source => source.source.authority === 'owner_statement' && source.source.evidence_sha256));
  assert(detail.operations.some(operation => operation.operation_id === report.cases[0].operation_id));
  const shown = await page.getByRole('region', {name: 'Memory record detail'}).innerText();
  assert(shown.includes('Provenance') && shown.includes('I prefer café in this exact context.'));
  await page.getByRole('region', {name: 'Memory record detail'}).scrollIntoViewIfNeeded();
  await page.screenshot({path: path.join(output, 'accepted-provenance.png')});
  assert.deepEqual(report.page_errors, []);
  await browser.close(); browser = null;
  await new Promise((resolve, reject) => {
    server.stdin.once('error', reject);
    server.stdin.end(JSON.stringify(report.cases.map(({name, candidate_id, operation_id, preview_sha256, effect_sha256}) => ({name, candidate_id, operation_id, preview_sha256, effect_sha256}))), resolve);
  });
  server.kill('SIGUSR1'); report.server_exit = await serverDone;
  assert.deepEqual(report.server_exit, {code: 0, signal: null}, 'Fixture verification failed; see fixture.log');
  assert.equal(verified?.status, 'passed', 'Missing fixture verification; see fixture.log');
  assert.equal(verified.global_claims, 2);
  assert.equal(verified.closed_sources, 7);
  assert.equal(verified.canonical_replay, true);
  assert.equal(verified.replay_model_calls, 0);
  assert.equal(verified.cases.length, report.cases.length);
  for (const entry of report.cases) {
    const proof = verified.cases.find(value => value.name === entry.name);
    assert(proof && proof.accepted_graph && proof.scope === entry.scope && proof.operation_id === entry.operation_id && proof.preview_sha256 === entry.preview_sha256 && proof.effect_sha256 === entry.effect_sha256);
    entry.checks.accepted_graph = true;
    entry.checks.no_implicit_promotion = true;
  }
  report.kernel_proof = verified;
  assert.equal(fingerprint(), report.source_sha256, 'Source changed during browser verification');
  report.fixture_directory_removed = !fs.existsSync(path.dirname(fixture.database));
  assert.equal(report.fixture_directory_removed, true);
  report.status = 'passed';
})().catch(async error => {
  report.error = error.stack;
  if (page && !page.isClosed()) {
    fs.writeFileSync(path.join(output, 'failure-dom.txt'), await page.locator('body').innerText().catch(() => ''));
    await page.screenshot({path: path.join(output, 'failure.png')}).catch(() => {});
  }
  process.exitCode = 1;
}).finally(async () => {
  if (browser) await browser.close();
  if (server && server.exitCode === null && server.signalCode === null) { server.kill('SIGINT'); report.server_exit = await serverDone; }
  if (binary && fs.existsSync(binary)) fs.unlinkSync(binary);
  report.completed_at = new Date().toISOString();
  fs.writeFileSync(path.join(output, 'fixture.log'), stdout);
  fs.writeFileSync(path.join(output, 'browser.json'), JSON.stringify(report, null, 2) + '\n');
  console.log(JSON.stringify({status: report.status, cases: report.cases.map(entry => entry.name), error: report.error, output}));
});
