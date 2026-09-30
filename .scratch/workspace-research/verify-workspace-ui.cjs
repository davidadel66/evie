const { chromium } = require('/Users/davidboktor/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright-core');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');

const root = path.resolve(__dirname, '../..');
const artifacts = path.join(__dirname, `browser-validation-${Date.now()}`);
fs.mkdirSync(artifacts);
const fixtureOutput = path.join(artifacts, 'fixture');
const report = { status: 'running', synthetic: true, cases: [], pageErrors: [], boundaries: [
  'Temporary fixture database and synthetic content only; no installed Evie server or provider requests.',
  'Browser checks permission management and immutable session revision display/API bindings; worker admission, cancellation, and persistent revocation enforcement are covered by separate deterministic Go checks.',
] };
let browser;
let page;
let server;
let serverDone;
const record = name => { report.cases.push(name); console.log(`PASS: ${name}`); };
const waitUntil = async (read, expected, label) => {
  for (let i = 0; i < 80; i++) {
    if (await read() === expected) return;
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw new Error(`Timed out: ${label}`);
};

(async () => {
  server = spawn(path.join(__dirname, 'browser.test'), ['-test.run=^TestWorkspaceSetupBrowserFixture$', '-test.timeout=5m'], {
    cwd: root,
    env: {...process.env, EVIE_SETUP_BROWSER_FIXTURE:'1', EVIE_SETUP_BROWSER_OUTPUT:fixtureOutput},
    stdio:['ignore','pipe','pipe'],
  });
  serverDone = new Promise(resolve => server.once('exit', (code, signal) => resolve({code, signal})));
  let stdout = '', stderr = '';
  server.stderr.on('data', chunk => { stderr += chunk; });
  const fixture = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Fixture startup timed out')), 30000);
    server.stdout.on('data', chunk => {
      stdout += chunk;
      const found = stdout.match(/SETUP_BROWSER_READY=(.+)/);
      if (found) { clearTimeout(timer); resolve(JSON.parse(found[1])); }
    });
    server.once('exit', code => { clearTimeout(timer); reject(new Error(`Fixture exited before readiness: ${code}; ${stderr}`)); });
  });
  browser = await chromium.launch({executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless:true});
  page = await browser.newPage({viewport:{width:1280,height:900}});
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => report.pageErrors.push(error.message));
  const post = async (route, body, expected=200) => {
    const response = await page.request.post(fixture.url + route, {data:body, headers:{Origin:fixture.url}});
    assert.equal(response.status(), expected, `Unexpected HTTP status for ${route}`);
    return response.json();
  };
  const snapshot = () => post('/api/context-sessions/list', {});
  const permission = () => page.getByRole('checkbox', {name:'Allow research delegation', exact:true});
  const responseFor = suffix => page.waitForResponse(response => response.url().endsWith(suffix) && response.request().method()==='POST');
  const create = async (name, enabled, testReset=false) => {
    await page.getByRole('button',{name:'Create workspace',exact:true}).click();
    const dialog = page.getByRole('dialog',{name:'Create workspace'});
    await waitUntil(() => permission().isEnabled(), true, 'creation permission availability');
    assert.equal(await permission().isChecked(), false);
    if (testReset) {
      await permission().focus();
      await permission().press('Space');
      assert.equal(await permission().isChecked(), true);
      await page.getByRole('combobox',{name:'Agent preset',exact:true}).selectOption('research');
      assert.equal(await permission().count(), 0);
      await page.getByRole('combobox',{name:'Agent preset',exact:true}).selectOption('standard');
      assert.equal(await permission().isChecked(), false);
    }
    await page.getByRole('textbox',{name:'Workspace name',exact:true}).fill(name);
    if (enabled) await permission().check();
    await permission().focus();
    assert.equal(await permission().evaluate(el => document.activeElement===el), true);
    await permission().press('Tab');
    assert.equal(await dialog.evaluate(el => el.contains(document.activeElement)), true);
    const pending = responseFor('/api/workspaces/register');
    await dialog.getByRole('button',{name:'Create workspace',exact:true}).click();
    const response = await pending;
    assert.equal(response.status(), 201);
    const request = response.request().postDataJSON();
    assert.equal(request.allowResearchDelegation === true, enabled);
    const created = await response.json();
    assert.deepEqual(created.allowedPresetIds, enabled ? ['standard','research'] : ['standard']);
    await dialog.waitFor({state:'hidden'});
    await page.getByRole('button',{name,exact:true}).waitFor();
    return created;
  };
  const openGeneral = async () => {
    await page.getByRole('button',{name:'General',exact:true}).click();
    await page.getByRole('heading',{name:'General',exact:true}).waitFor();
  };
  const toggle = async enabled => {
    assert.notEqual(await permission().isChecked(), enabled);
    const pending = responseFor('/api/workspaces/research');
    await permission().click();
    const response = await pending;
    assert.equal(response.status(), 200);
    const value = await response.json();
    await waitUntil(() => permission().isEnabled(), true, 'permission save completion');
    assert.equal(await permission().isChecked(), enabled);
    return value;
  };

  await page.goto(fixture.url);
  await page.getByRole('button',{name:'General',exact:true}).waitFor();
  const initial = await snapshot();
  const general = initial.workspaces.find(workspace => workspace.id===fixture.workspaceId);
  assert.deepEqual(general.allowedPresetIds,['standard']);
  await create('Fixture default off', false, true);
  const enabledCreation = await create('Fixture research enabled', true);
  assert.equal((await snapshot()).activeSession.id, initial.activeSession.id);
  record('Creation defaults off; explicit opt-in persists; preset switching clears opt-in; creation preserves active chat');
  record('Research checkbox works with keyboard and focus remains inside creation modal');

  await openGeneral();
  assert.equal(await permission().isChecked(), false);
  await permission().focus();
  const enablePending = responseFor('/api/workspaces/research');
  await permission().press('Space');
  const enabledResponse = await enablePending;
  assert.equal(enabledResponse.status(),200);
  const firstEnabled = await enabledResponse.json();
  await waitUntil(() => permission().isEnabled(),true,'keyboard save');
  assert.notEqual(firstEnabled.currentRevisionId,general.currentRevisionId);
  let current = await snapshot();
  assert.equal(current.sessions.find(session => session.id===fixture.sessionId).workspaceRevisionSnapshot,general.currentRevisionId);
  const newChatPending = responseFor('/api/context-sessions/select');
  await page.getByRole('main').getByRole('button',{name:'New chat',exact:true}).click();
  const newChatResponse = await newChatPending;
  assert.equal(newChatResponse.status(),200);
  const newChat = await newChatResponse.json();
  assert.equal(newChat.session.workspaceRevisionSnapshot,firstEnabled.currentRevisionId);
  record('Settings enable advances Workspace revision; new chat pins it and old chat retains its original revision');

  await openGeneral();
  const disabled = await toggle(false);
  const reenabled = await toggle(true);
  assert.notEqual(disabled.currentRevisionId,firstEnabled.currentRevisionId);
  assert.notEqual(reenabled.currentRevisionId,disabled.currentRevisionId);
  await page.reload();
  await page.getByRole('button',{name:'General',exact:true}).waitFor();
  await openGeneral();
  assert.equal(await permission().isChecked(),true);
  current = await snapshot();
  assert.equal(current.workspaces.find(workspace => workspace.id===general.id).currentRevisionId,reenabled.currentRevisionId);
  assert.equal(current.sessions.find(session => session.id===newChat.session.id).workspaceRevisionSnapshot,firstEnabled.currentRevisionId);
  record('Disable and re-enable persist across reload without rewriting existing chat snapshots');

  const externalDisable = await post('/api/workspaces/research',{workspaceId:general.id,revision:reenabled.currentRevisionId,enabled:false});
  const stalePending = responseFor('/api/workspaces/research');
  await permission().click();
  assert.equal((await stalePending).status(),409);
  await page.getByRole('alert').filter({hasText:'Workspace changed'}).waitFor();
  assert.equal(await permission().isChecked(),true);
  await page.getByRole('button',{name:'Refresh permissions',exact:true}).click();
  await waitUntil(() => permission().isChecked(),false,'refresh stale checkbox');
  assert.equal((await snapshot()).workspaces.find(workspace=>workspace.id===general.id).currentRevisionId,externalDisable.currentRevisionId);
  await toggle(true);
  record('Stale edits fail closed, retain displayed state, and Refresh permissions loads current state before retry');

  await page.route('**/api/context-sessions/list',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:'Synthetic list refresh failure'})}));
  await toggle(false);
  await page.getByRole('alert').filter({hasText:'Permission saved, but the Workspace list could not refresh'}).waitFor();
  assert.equal(await permission().isChecked(),false);
  await page.unroute('**/api/context-sessions/list');
  await page.getByRole('button',{name:'Refresh permissions',exact:true}).click();
  await waitUntil(() => page.getByRole('button',{name:'Refresh permissions',exact:true}).count(),0,'refresh recovery');
  record('Successful permission save remains confirmed when list refresh fails; explicit refresh recovers');

  await page.getByRole('button',{name:enabledCreation.displayName,exact:true}).click();
  await page.getByRole('heading',{name:enabledCreation.displayName,exact:true}).waitFor();
  await page.screenshot({path:path.join(artifacts,'desktop-settings.png'),fullPage:true});
  await page.setViewportSize({width:390,height:844});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
  const box = await permission().boundingBox();
  assert.ok(box && box.x>=0 && box.x+box.width<=390);
  await permission().focus();
  assert.equal(await permission().evaluate(el=>document.activeElement===el),true);
  await page.screenshot({path:path.join(artifacts,'mobile-settings.png'),fullPage:true});
  record('Workspace research settings fit 390px without horizontal overflow and retain keyboard focus');

  assert.deepEqual(report.pageErrors,[]);
  report.status='passed';
})().catch(async error=>{
  report.status='failed';
  report.failure={name:error.name,message:error.message,stack:error.stack};
  if(page) await page.screenshot({path:path.join(artifacts,'failure.png'),fullPage:true}).catch(()=>{});
  process.exitCode=1;
}).finally(async()=>{
  if(browser) await browser.close();
  if(server && server.exitCode===null) server.kill('SIGUSR1');
  if(serverDone) report.fixtureExit=await serverDone;
  if(report.fixtureExit?.code!==0){report.status='failed';process.exitCode=1;}
  fs.writeFileSync(path.join(artifacts,'browser-report.json'),JSON.stringify(report,null,2));
  console.log(JSON.stringify({artifacts,...report},null,2));
});
