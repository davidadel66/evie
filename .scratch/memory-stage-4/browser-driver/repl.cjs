const {chromium} = require('playwright-core');
const readline = require('node:readline');
(async()=>{
 const browser = await chromium.launch({executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true});
 const context=await browser.newContext({viewport:{width:1440,height:1000}});
 const page=await context.newPage();
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(process.argv[2]);
 const rl=readline.createInterface({input:process.stdin});
 console.log('BROWSER_READY '+page.url());
 for await (const line of rl){try {if(line==='CLOSE'){await browser.close();break;}const value=await eval('(async()=>{'+line+'})()');console.log(JSON.stringify({ok:true,value}));}catch(e){console.log(JSON.stringify({ok:false,error:e.message}));}}
 await browser.close();
})().catch(e=>{console.error(e);process.exitCode=1;});
