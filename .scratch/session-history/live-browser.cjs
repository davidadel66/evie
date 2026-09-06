const {chromium}=require('../memory-stage-4/browser-driver/node_modules/playwright-core');
const assert=require('node:assert/strict');
(async()=>{const browser=await chromium.launch({executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true});try{
 const page=await browser.newPage({viewport:{width:1440,height:1000}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://127.0.0.1:6687/');await page.getByRole('button',{name:'hows it going',exact:true}).click();
 const saved='okay yeah lets test it out. I like concise answers and not a lot of data dump. when you answer me, I want you to think about readibility and style.';
 await page.getByText(saved,{exact:true}).waitFor();await page.reload();await page.getByText(saved,{exact:true}).waitFor();
 assert.equal(await page.getByRole('button',{name:'Approve',exact:true}).count(),0);
 await page.screenshot({path:'.scratch/session-history/live-history.png'});
 await page.getByRole('button',{name:'Data',exact:true}).click();
 await page.getByRole('button',{name:/Keep answers concise/}).click();await page.getByRole('region',{name:'Memory detail',exact:true}).waitFor();
 await page.getByRole('button',{name:'David',exact:true}).click();await page.getByRole('heading',{name:'David',exact:true}).waitFor();
 await page.getByRole('button',{name:/Back to memories/}).click();await page.getByRole('button',{name:'Graph',exact:true}).click();await page.getByRole('button',{name:'Open Keep answers concise; avoid data dumps.',exact:true}).click();await page.getByRole('region',{name:'Memory detail',exact:true}).waitFor();
 await page.screenshot({path:'.scratch/session-history/live-memory.png'});
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:'.scratch/session-history/live-mobile.png'});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);assert.deepEqual(errors,[]);
 console.log('PASS: saved General history on reopen and reload, historical approvals inert, memory list/detail/David/graph navigation, mobile no overflow, no page errors');
}finally{await browser.close();}})().catch(e=>{console.error(e);process.exitCode=1;});
