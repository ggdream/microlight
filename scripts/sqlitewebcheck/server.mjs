// Exercise the real packaged server, including a deep-link browser load.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {chromium} from 'playwright';
const binary = process.argv[2];
if (!binary) throw new Error('Usage: node server.mjs /absolute/path/to/server');
const server = spawn(binary,['-addr','127.0.0.1:0'],{stdio:['ignore','pipe','pipe']});
let browser;
try {
 const url = await new Promise((resolve,reject)=>{
  const timer=setTimeout(()=>reject(new Error('server startup timed out')),15000);
  server.once('error',reject);
  server.once('exit',code=>{clearTimeout(timer);reject(new Error('server exited '+code));});
  server.stderr.on('data',data=>{
   const match=data.toString().match(/listening on (127\.0\.0\.1:\d+)/);
   if(match){clearTimeout(timer);resolve('http://'+match[1]);}
  });
 });
 for(const [route,status] of [['/',200],['/chat/history/123',200],['/missing.js',404],['/sqlite/worker.js',200]]) {
  const r=await fetch(url+route); assert.equal(r.status,status,route);
 }
 const index=await fetch(url+'/'); const text=await index.text();
 assert(text.includes('<base href="/"'));
 const wasm=text.match(/fetch\("([^"]+\.wasm)"\)/)[1];
 const head=await fetch(url+'/'+wasm,{method:'HEAD'});
 assert.equal(head.headers.get('content-type'),'application/wasm');
 assert.equal((await head.arrayBuffer()).byteLength,0);
 const range=await fetch(url+'/'+wasm,{headers:{Range:'bytes=0-3'}});
 assert.equal(range.status,206);
 assert.deepEqual([...new Uint8Array(await range.arrayBuffer())],[0,97,115,109]);
 browser=await chromium.launch({executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE||undefined,headless:true});
 const page=await browser.newPage(); const errors=[];
 page.on('pageerror',e=>errors.push(e.message));
 await page.goto(url+'/chat/history/123');
 await page.waitForFunction(()=>document.querySelector('canvas'),{},{timeout:120000});
 await page.waitForFunction(()=>typeof globalThis.microlightSQLite==='object',{},{timeout:120000});
 assert.deepEqual(errors,[]);
 console.log('PASS packaged server: History reload, root assets, SQLite worker, Wasm MIME/HEAD/range, real asset 404');
} finally {
 if(browser) await browser.close();
 server.kill();
}
