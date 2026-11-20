// Local browser regression for MyGo's DOM pointer -> Go touch scrolling path.
// Build testdata/main.go to touch.wasm beside wasm_exec.js and index.html first.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile} from 'node:fs/promises';
import path from 'node:path';
import {chromium} from '../sqlitewebcheck/node_modules/playwright/index.mjs';

const site = path.resolve(process.argv[2]);
const server = createServer(async (req,res) => {
  const file=path.resolve(site,'.'+(req.url==='/'?'/index.html':req.url));
  if(!file.startsWith(site+path.sep)){res.writeHead(403).end();return;}
  try {
    res.setHeader('Content-Type',file.endsWith('.wasm')?'application/wasm':file.endsWith('.js')?'text/javascript':'text/html');
    res.end(await readFile(file));
  } catch { res.writeHead(404).end(); }
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
try {
  browser=await chromium.launch({channel:'chrome',headless:true});
  const page=await browser.newPage({viewport:{width:390,height:640},hasTouch:true});
  page.on('pageerror',e=>console.error(e));
  await page.goto(`http://127.0.0.1:${server.address().port}/`);
  await page.waitForFunction(()=>typeof touchScrollY==='function' && document.querySelector('canvas'));
  const client=await page.context().newCDPSession(page);
  const swipe=async()=>{
    await client.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:180,y:480}]});
    for(let y=460;y>=180;y-=20){
      await client.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:180,y}]});
      await new Promise(resolve=>setTimeout(resolve,20));
    }
    await client.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
  };
  await swipe();
  await page.waitForFunction(()=>touchScrollY()>100);
  const first=await page.evaluate(()=>touchScrollY());
  await swipe();
  await page.waitForFunction(first=>touchScrollY()>first+100,first);
  assert.ok(await page.evaluate(()=>touchScrollY())>first);
  console.log('PASS browser touch scrolling and repeated swipe');
} finally {
  if(browser) await browser.close();
  await new Promise(resolve=>server.close(resolve));
}
