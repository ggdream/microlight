// The Go runtime and MyGo UI stay on the main thread. SQLite and its asynchronous
// IndexedDB VFS run in a separate worker; no SharedArrayBuffer or eval is needed.
const worker = new Worker(new URL('./worker.js', import.meta.url), {type: 'module'});
let sequence = 0;
let failed;
const pending = new Map();
worker.onmessage = ({data}) => {
  const request = pending.get(data.id);
  if (!request) return;
  pending.delete(data.id);
  if (data.error) request.reject(Object.assign(new Error(data.error.message), {code: data.error.code}));
  else request.resolve(data.result);
};
function fail(error) {
  failed = error;
  for (const request of pending.values()) request.reject(error);
  pending.clear();
}
worker.onerror = event => fail(new Error(event.message || 'SQLite worker failed to load'));
worker.onmessageerror = () => fail(new Error('SQLite worker returned an unreadable message'));
globalThis.microlightSQLite = {
  call(op, args) {
    if (failed) return Promise.reject(failed);
    return new Promise((resolve, reject) => {
      const id = ++sequence;
      pending.set(id, {resolve, reject});
      try { worker.postMessage({id, op, args}); }
      catch (error) { pending.delete(id); reject(error); }
    });
  }
};
