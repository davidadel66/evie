const {chromium}=require('../memory-stage-4/browser-driver/node_modules/playwright-core');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true});
 try{
 const page=await browser.newPage({viewport:{width:1440,height:1000}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const sessions=['A','B'].map(id=>({id,title:'Conversation '+id,status:'active',workspaceId:'w',createdAt:'2026-09-05T00:00:00Z',updatedAt:'2026-09-05T00:00:00Z'}));
 const scope={kind:'workspace',displayName:'General',workspaceId:'w',workspaceRevision:'r'};
 let selected='A',delayA=false,releaseA,failOlder=true,olderRequests=[],sent;
 const item=(id,text)=>({kind:'user',key:id,text});
 await page.route(url=>url.pathname.startsWith('/api/'),async route=>{
  const path=new URL(route.request().url()).pathname;const body=route.request().postDataJSON();
  const json=body=>route.fulfill({json:body});
  if(path==='/api/context-sessions/list') return json({ownerDisplayName:'David',workspaces:[{id:'w',displayName:'General',state:'active',currentRevisionId:'r'}],projects:[],sessions,activeSession:sessions.find(s=>s.id===selected),activeScope:scope});
  if(path==='/api/context-sessions/select'){selected=body.sessionId;return json({session:sessions.find(s=>s.id===selected),scope});}
  if(path==='/api/context-sessions/history'){
   if(body.sessionId==='A'&&delayA){await new Promise(resolve=>releaseA=resolve);return json({sessionId:'A',items:[item('late','STALE A')]});}
   if(body.before){olderRequests.push(body.before);if(failOlder){failOlder=false;return route.fulfill({status:503,json:{error:'Retry older history'}});}return json({sessionId:body.sessionId,items:[item('old','OLDER B')],before:''});}
   return json({sessionId:body.sessionId,items:[item(body.sessionId,'HISTORY '+body.sessionId)],before:body.sessionId==='B'?'100':''});
  }
  if(path==='/api/chat'){sent=body;return route.fulfill({contentType:'text/event-stream',body:'event: turn_done\ndata: {}\n\n'});}
  return json({});
 });
 await page.goto('http://127.0.0.1:5174/');await page.getByText('HISTORY A',{exact:true}).waitFor({timeout:8000}).catch(async e=>{console.log(await page.locator('body').innerText(),errors);throw e;});
 await page.getByRole('button',{name:'Conversation B',exact:true}).click();await page.getByText('HISTORY B',{exact:true}).waitFor();
 delayA=true;await page.getByRole('button',{name:'Conversation A',exact:true}).click();
 await page.getByText('Loading conversation…',{exact:true}).waitFor();
 await page.getByRole('button',{name:'Conversation B',exact:true}).click();await page.getByText('HISTORY B',{exact:true}).waitFor();releaseA();
 await page.getByRole('button',{name:'Earlier messages',exact:true}).click();await page.getByRole('alert').filter({hasText:'Retry older history'}).waitFor();
 await page.getByRole('button',{name:'Retry',exact:true}).click();await page.getByText('OLDER B',{exact:true}).waitFor();
 assert.deepEqual(olderRequests,['100','100']);assert.equal(await page.getByText('STALE A',{exact:true}).count(),0);assert.equal(await page.getByText('HISTORY B',{exact:true}).count(),1);
 await page.getByPlaceholder('Message Evie…').fill('synthetic binding test');await page.getByPlaceholder('Message Evie…').press('Enter');await page.waitForFunction(()=>document.body.innerText.includes('synthetic binding test'));
 assert.equal(sent.sessionId,'B');assert.deepEqual(errors,[]);
 await page.screenshot({path:'.scratch/session-history/browser-fixtures.png'});
 console.log('PASS: delayed session isolation, failed older-page retry, retained current history, session-bound send, no page errors');
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
