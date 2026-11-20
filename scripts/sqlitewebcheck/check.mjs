// Runs actual Go/Wasm code against SQLite and IndexedDB in an isolated browser.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp, cp, readFile, writeFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {chromium} from 'playwright';

const root = fileURLToPath(new URL('../../', import.meta.url));
const site = await mkdtemp(path.join(tmpdir(), 'microlight-sqliteweb-'));
const env = {...process.env, GOOS: 'js', GOARCH: 'wasm', CGO_ENABLED: '0'};
const go = args => execFileSync('go', args, {cwd: root, env, stdio: 'inherit'});
let server, browser;
try {
  go(['build', '-mod=readonly', '-o', path.join(site, 'check.wasm'), './scripts/sqlitewebcheck']);
  go(['test', '-mod=readonly', '-c', '-o', path.join(site, 'app-store.test.wasm'), './internal/pages/chat']);
  execFileSync('bash',['scripts/build.sh','-platform=web','-o',path.join(site,'product')],{
    cwd:root,env:{...process.env,CGO_ENABLED:'0'},stdio:'inherit'
  });
  const goroot = execFileSync('go', ['env', 'GOROOT'], {encoding: 'utf8'}).trim();
  await cp(path.join(goroot, 'lib/wasm/wasm_exec.js'), path.join(site, 'wasm_exec.js'));
  await cp(path.join(root, 'pkg/sqliteweb/web'), path.join(site, 'sqlite'), {recursive: true});
  await cp(path.join(root,'pkg/webnotify/web/notification-worker.js'),path.join(site,'notification-worker.js'));
  await writeFile(path.join(site, 'index.html'), `<!doctype html><meta charset="utf-8">
<script src="wasm_exec.js"></script><script>
const go = new Go();
const suite = new URL(location.href).searchParams.get('suite');
if (suite === 'app') go.argv = ['store.test', '-test.run=^TestBrowserStoreRoundTrip$', '-test.v'];
go.exit = code => { globalThis.testExit = code; };
WebAssembly.instantiateStreaming(fetch(suite === 'app' ? 'app-store.test.wasm' : 'check.wasm'), go.importObject)
.then(x => go.run(x.instance)).catch(e => { console.error(e); globalThis.testExit = 1; });
</script>`);
  server = createServer(async (req, res) => {
    const name = new URL(req.url, 'http://localhost').pathname;
    if (name === '/notification-model/chat/completions') {
      req.resume();
      await new Promise(resolve=>setTimeout(resolve,1000));
      res.writeHead(200,{'Content-Type':'text/event-stream'});
      for(const delta of [{role:'assistant',content:''},{content:'NOTIFICATION_QA_REPLY'}]) {
        res.write('data: '+JSON.stringify({id:'chatcmpl-notification',object:'chat.completion.chunk',created:1,model:'notification-qa',choices:[{index:0,delta,finish_reason:null}]})+'\n\n');
      }
      res.end('data: '+JSON.stringify({id:'chatcmpl-notification',object:'chat.completion.chunk',created:1,model:'notification-qa',choices:[{index:0,delta:{},finish_reason:'stop'}]})+'\n\ndata: [DONE]\n\n');
      return;
    }
    const file = path.resolve(site, '.' + (name.endsWith('/') ? name+'index.html' : name));
    if (!file.startsWith(site + path.sep)) { res.writeHead(403).end(); return; }
    try {
      const data = await readFile(file);
      res.setHeader('Content-Type', file.endsWith('.wasm') ? 'application/wasm' : /\.(m?js)$/.test(file) ? 'text/javascript' : 'text/html');
      res.end(data);
    } catch { res.writeHead(404).end(); }
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const url = `http://127.0.0.1:${server.address().port}/`;
  browser = await chromium.launch({
    executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined,
    headless: true,
    args: ['--disable-background-timer-throttling', '--disable-renderer-backgrounding', '--disable-backgrounding-occluded-windows']
  });
  const context = await browser.newContext();
  context.setDefaultTimeout(120000);
  async function page(suite = '') {
    const p = await context.newPage();
    p.on('pageerror', error => console.error('Browser error:', error.message));
    await p.goto(url + (suite ? '?suite=' + suite : ''));
    if (!suite) await p.waitForFunction(() => typeof storageCheck === 'function');
    return p;
  }
  const call = (p, op) => p.evaluate(op => storageCheck(op), op);
  // Real browser API and permissions; no fake Notification implementation.
  await context.grantPermissions(['notifications']);
  const note = await page();
  assert.equal(await note.evaluate(()=>notificationCheck('request')),'ok');
  assert.equal(await note.evaluate(()=>notificationCheck('show')),'ok');
  const visibleNotes=()=>note.evaluate(async()=>{
    const reg=await navigator.serviceWorker.ready;
    return (await reg.getNotifications({tag:'notification-qa'})).map(n=>({title:n.title,body:n.body,tag:n.tag,clientID:n.data.clientID,renotify:n.renotify}));
  });
  const notes=await visibleNotes();
  assert.equal(notes.length,1);assert.equal(notes[0].body,'Actual browser notification');
  assert.equal(notes[0].renotify,true);
  assert.equal(await note.evaluate(()=>notificationCheck('show')),'ok');
  const repeated=await visibleNotes();
  assert.equal(repeated.length,1);assert.equal(repeated[0].renotify,true);
  // Confirm the notification belongs to its originating tab.
  const secondTab=await page();
  assert.equal(await secondTab.evaluate(()=>notificationCheck('show')),'ok');
  const replacement=await visibleNotes();
  assert.notEqual(replacement[0].clientID,notes[0].clientID);
  const notificationWorker=context.serviceWorkers().find(worker=>worker.url().endsWith('/notification-worker.js'));
  await notificationWorker.evaluate(async payload=>{
    const target=await self.clients.get(payload.clientID);
    target.postMessage({type:'microlight-notification-click',tag:payload.tag});
  },replacement[0]);
  await secondTab.waitForFunction(()=>notificationCheck('clicked'));
  await secondTab.evaluate(()=>notificationCheck('close'));
  await secondTab.waitForFunction(async()=>{
    const reg=await navigator.serviceWorker.ready;return (await reg.getNotifications({tag:'notification-qa'})).length===0;
  });
  await secondTab.close();
  const fallback=await context.newPage();
  await fallback.addInitScript(()=>Object.defineProperty(navigator,'serviceWorker',{value:undefined}));
  await fallback.goto(url);await fallback.waitForFunction(()=>typeof notificationCheck==='function');
  assert.equal(await fallback.evaluate(()=>notificationCheck('show')),'ok');
  assert.equal(await fallback.evaluate(()=>notificationCheck('close')),'ok');
  await fallback.close();
  await context.clearPermissions();
  const requested=await note.evaluate(()=>Promise.race([notificationCheck('request'),new Promise(resolve=>setTimeout(()=>resolve('pending'),1500))]));
  assert(['ok','denied','pending'].includes(requested));
  const cdp=await context.newCDPSession(note);
  await cdp.send('Browser.setPermission',{permission:{name:'notifications'},setting:'denied',origin:url});
  assert.equal(await note.evaluate(()=>notificationCheck('request')),'denied');
  assert.equal(await note.evaluate(()=>notificationCheck('show')),'denied');
  await cdp.send('Browser.resetPermissions');
  await cdp.detach();
  await note.close();
  console.log('PASS browser notifications: real permission grant/deny, Service Worker delivery, originating tab, close');
  if (!process.env.MICROLIGHT_NOTIFICATION_ONLY) {
  const a = await page();
  await call(a, 'open');
  const start = performance.now();
  await call(a, 'seed');
  console.log(`12 MiB seed: ${(performance.now() - start).toFixed(0)} ms`);
  const original = await call(a, 'verify');
  assert.equal(original.bytes, 12 * 1024 * 1024);
  assert.equal(original.integrity, 'ok');
  await call(a, 'rollback');
  assert.deepEqual(await call(a, 'verify'), original);
  assert.equal(await call(a,'finalize'),'finalized');
  await call(a, 'constraint');
  await call(a, 'duplicate');
  assert.deepEqual(await call(a, 'verify'), original);
  await a.reload();
  await a.waitForFunction(() => typeof storageCheck === 'function');
  await call(a, 'open');
  assert.deepEqual(await call(a, 'verify'), original);
  console.log('PASS int64/REAL/TEXT/NULL/empty BLOB, 12 MiB BLOB, rollback, reload');
  const b = await page();
  await call(b, 'open');
  await Promise.all([call(a, 'increment'), call(b, 'increment')]);
  assert.equal((await call(a, 'verify')).counter, 80);
  console.log('PASS two tabs: 80 concurrent atomic increments, no lost writes');
  const worker = a.workers()[0];
  await worker.evaluate(() => {
    globalThis.blockBytesWritten = 0;
    const put = IDBObjectStore.prototype.put;
    IDBObjectStore.prototype.put = function(value, ...args) {
      if (this.name === 'blocks') globalThis.blockBytesWritten += value.data?.byteLength || 0;
      return put.call(this, value, ...args);
    };
  });
  await call(a, 'increment');
  const written = await worker.evaluate(() => globalThis.blockBytesWritten);
  assert(written > 0 && written < 1024 * 1024, `small updates wrote ${written} bytes`);
  console.log(`PASS incremental I/O: 40 small updates wrote ${written} block bytes, not 40 whole database snapshots`);
  await call(b, 'close');
  await b.close();
  await call(a, 'crash');
  await a.close(); // deliberately omit sql.DB.Close / COMMIT / ROLLBACK
  const c = await page();
  await call(c, 'open');
  const recovered = await call(c, 'verify');
  assert.equal(recovered.counter, 120);
  assert.equal(recovered.sha256, original.sha256);
  assert.equal(recovered.integrity, 'ok');
  await call(c, 'close');
  console.log('PASS abrupt tab close: uncommitted writes recovered, integrity_check=ok');
  await c.evaluate(() => {
    const button=document.createElement('button'); button.textContent='Pick test file';
    button.onclick=()=>{ globalThis.filePickResult=storageCheck('pick'); };
    document.body.appendChild(button);
  });
  const chooserEvent = c.waitForEvent('filechooser');
  await c.getByRole('button',{name:'Pick test file'}).click();
  const chooser = await chooserEvent;
  await chooser.setFiles({name:'storage-check.txt',mimeType:'text/plain',buffer:Buffer.from('browser file bytes')});
  assert.deepEqual(await c.evaluate(()=>filePickResult),{name:'storage-check.txt',text:'browser file bytes'});
  const downloaded = c.waitForEvent('download');
  await call(c,'download');
  const download = await downloaded;
  assert.equal(download.suggestedFilename(),'storage-check.txt');
  assert.deepEqual(await readFile(await download.path()),Buffer.from('original bytes\0'));
  console.log('PASS browser file selection and original-byte download');
  const app = await page('app');
  const output = [];
  app.on('console', message => { output.push(message.text()); console.log(message.text()); });
  await app.waitForFunction(() => typeof testExit === 'number');
  assert.equal(await app.evaluate(() => testExit), 0, output.join('\n'));
  await app.reload();
  await app.waitForFunction(() => typeof testExit === 'number');
  assert.equal(await app.evaluate(() => testExit), 0, output.join('\n'));
  assert(output.some(line => line.includes('checking application data after page reload')));
  console.log('PASS application schema migrations, conversation/messages, 4 MiB attachment, draft and configuration after reload');
  }
  await context.grantPermissions(['notifications']);
  const ui=await context.newPage({viewport:{width:1120,height:760}});
  await ui.setViewportSize({width:1120,height:760});
  const uiErrors=[];
  ui.on('pageerror',error=>uiErrors.push(error.message));
  await ui.goto(url+'product/web/');
  await ui.waitForSelector('canvas');
  await ui.waitForTimeout(500);
  async function click(x,y) {
    await ui.mouse.move(x,y); await ui.waitForTimeout(80);
    await ui.mouse.click(x,y,{delay:80}); await ui.waitForTimeout(150);
  }
  await click(30,726); // actual MyGo settings button
  await click(430,360); // actual send-test-notification button
  await ui.waitForFunction(async()=>{const reg=await navigator.serviceWorker.ready;return (await reg.getNotifications({tag:'microlight-test'})).length===1;});
  await ui.evaluate(async()=>{const reg=await navigator.serviceWorker.ready;for(const n of await reg.getNotifications())n.close();});
  await click(450,600); // actual custom-instruction editor
  await ui.keyboard.type('Web persisted instruction',{delay:60});
  await ui.waitForTimeout(300);
  // Read via a separate DB worker, so this assertion checks durable state seen
  // by another connection, not the editor's in-memory value or a test fixture.
  async function saved(sql) {
    return ui.evaluate(async sql=>{
      const worker=new Worker(new URL('sqlite/worker.js',document.baseURI),{type:'module'});
      let id=0;const pending=new Map();
      worker.onmessage=({data})=>{const p=pending.get(data.id);pending.delete(data.id);data.error?p.reject(new Error(data.error.message)):p.resolve(data.result)};
      const call=(op,...args)=>new Promise((resolve,reject)=>{const key=++id;pending.set(key,{resolve,reject});worker.postMessage({id:key,op,args})});
      try {
        const db=await call('open','microlight');
        const stmt=await call('prepare',db,sql);
        await call('query',db,stmt.id,[]);
        const value=await call('next',db,stmt.id);
        await call('finalize',db,stmt.id);await call('close',db);
        return value?.[0]?.[0]===4?new TextDecoder().decode(value[0][1]):value?.[0]?.[1];
      } finally {worker.terminate()}
    },sql);
  }
  const promptSQL="SELECT value FROM config WHERE key='system_prompt'";
  assert.equal(await saved(promptSQL),'Web persisted instruction');
  await click(70,32); // back to chat
  await click(440,661); // actual composer
  await ui.keyboard.type('Browser draft survives reload',{delay:40});
  await ui.waitForTimeout(300);
  const draftSQL="SELECT value FROM app_data WHERE key='draft:text:'";
  assert.equal(await saved(draftSQL),'Browser draft survives reload');
  await ui.reload();await ui.waitForSelector('canvas');await ui.waitForTimeout(300);
  assert.equal(await saved(promptSQL),'Web persisted instruction');
  assert.equal(await saved(draftSQL),'Browser draft survives reload');
  assert.deepEqual(uiErrors,[]);
  console.log('PASS real production UI: typing settings and draft stays responsive, both survive reload');
  // Local deterministic model fixture exercises the actual app reply pipeline.
  const model={protocol:'openai_chat',base_url:url+'notification-model',model:'notification-qa',api_key:'isolated-notification-test'};
  const literal=JSON.stringify(model).replaceAll("'","''");
  await saved("INSERT OR REPLACE INTO config(key,value) VALUES('settings','"+literal+"')");
  await ui.reload();await ui.waitForSelector('canvas');await ui.waitForTimeout(300);
  await click(440,661);
  await ui.keyboard.press('ControlOrMeta+A');await ui.keyboard.type('Background notification QA');
  await ui.keyboard.press('Enter');
  const front=await context.newPage();await front.goto(url);
  await front.bringToFront();
  await ui.waitForFunction(async()=>{const reg=await navigator.serviceWorker.ready;return (await reg.getNotifications()).some(n=>n.body==='NOTIFICATION_QA_REPLY');});
  await ui.evaluate(async()=>{const reg=await navigator.serviceWorker.ready;for(const n of await reg.getNotifications())n.close();});
  await ui.bringToFront();await front.close();
  console.log('PASS production UI: test notification and actual background reply notification');
  if (process.env.MICROLIGHT_WEB_OUTPUT) await cp(path.join(site,'product/web'),process.env.MICROLIGHT_WEB_OUTPUT,{recursive:true});
} finally {
  if (browser) await browser.close();
  if (server) await new Promise(resolve => server.close(resolve));
  await rm(site, {recursive: true, force: true});
}
