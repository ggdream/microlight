import SQLiteFactory from './vendor/dist/wa-sqlite-async.mjs';
import * as SQLite from './vendor/src/sqlite-api.js';
import {IDBBatchAtomicVFS} from './vendor/src/examples/IDBBatchAtomicVFS.js';

const engine = (async () => {
  const module = await SQLiteFactory();
  const sqlite = SQLite.Factory(module);
  sqlite.bind_empty_blob = (stmt, index) => {
    // sqlite3_malloc(0) may return NULL. Use zeroblob(0) so []byte{} does not
    // silently become SQL NULL at the C API boundary.
    const code = module._sqlite3_bind_zeroblob(stmt, index, 0);
    if (code !== SQLite.SQLITE_OK) throw new SQLite.SQLiteError('bind empty blob', code);
  };
  const vfs = await IDBBatchAtomicVFS.create('microlight-idb', module);
  sqlite.vfs_register(vfs, true);
  return sqlite;
})();
const databases = new Map();
const statements = new Map();
let nextStatement = 0;
// Asyncify permits only one suspended operation per Wasm instance. Serialize
// ALL calls, including calls on separate database/sql connections in this tab.
let queue = Promise.resolve();
self.onmessage = ({data}) => {
  queue = queue.then(async () => {
    try { self.postMessage({id: data.id, result: await dispatch(data.op, data.args)}); }
    catch (error) { self.postMessage({id: data.id, error: {message: error.message, code: error.code || 0}}); }
  });
};

function bind(sqlite, stmt, parameters) {
  if (parameters.length !== sqlite.bind_parameter_count(stmt)) throw new Error('SQLite parameter count mismatch');
  parameters.forEach(([kind, value], i) => {
    if (kind === 'integer') sqlite.bind_int64(stmt, i + 1, BigInt(value));
    else if (kind === 'real') sqlite.bind_double(stmt, i + 1, value);
    else if (kind === 'blob') {
      if (value.byteLength === 0) sqlite.bind_empty_blob(stmt, i + 1);
      else sqlite.bind_blob(stmt, i + 1, value);
    }
    else if (kind === 'text') sqlite.bind_text(stmt, i + 1, value);
    else if (kind === 'null') sqlite.bind_null(stmt, i + 1);
    else throw new Error('Unsupported SQLite parameter type');
  });
}

function row(sqlite, stmt) {
  return sqlite.column_names(stmt).map((_, i) => {
    const kind = sqlite.column_type(stmt, i);
    const value = sqlite.column(stmt, i);
    // Preserve INTEGER vs REAL, full int64 precision, NULL vs empty values, and
    // copy blobs before SQLite reuses their Wasm memory on the next step.
    return [kind, kind === SQLite.SQLITE_INTEGER ? String(value) :
      kind === SQLite.SQLITE_BLOB ? value.slice() : value];
  });
}

async function dispatch(op, args) {
  const sqlite = await engine;
  if (op === 'open') {
    const name = args[0];
    if (!/^[a-zA-Z0-9_-]{1,48}$/.test(name)) throw new Error('Invalid browser database name');
    // A second connection to the same file in THIS worker could wait for a
    // lock whose releasing COMMIT is queued behind it. Share the sql.DB pool
    // instead. Separate tabs have separate workers and coordinate via Web Locks.
    if ([...databases.values()].includes(name)) throw new Error('Database already open in this tab; share the existing sql.DB');
    const db = await sqlite.open_v2(name, SQLite.SQLITE_OPEN_READWRITE | SQLite.SQLITE_OPEN_CREATE, 'microlight-idb');
    try {
      await sqlite.exec(db, 'PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON; PRAGMA temp_store=MEMORY;');
    } catch (error) { await sqlite.close(db); throw error; }
    databases.set(db, name);
    return db;
  }
  const db = args[0];
  if (!databases.has(db)) throw new Error('Browser database is closed');
  switch (op) {
    case 'close':
      for (const [id, state] of statements) {
        if (state.db === db) { await state.iterator.return(); statements.delete(id); }
      }
      await sqlite.close(db); databases.delete(db); return null;
    case 'exec': {
      let affected = 0;
      let used = false;
      for await (const stmt of sqlite.statements(db, args[1])) {
        const parameters = used ? [] : args[2];
        bind(sqlite, stmt, parameters); used = true;
        while (await sqlite.step(stmt) === SQLite.SQLITE_ROW) { /* drain RETURNING */ }
        affected += sqlite.changes(db);
      }
      let inserted = '0';
      await sqlite.exec(db, 'SELECT last_insert_rowid()', values => { inserted = String(values[0]); });
      return {affected, inserted};
    }
    case 'prepare': {
      const iterator = sqlite.statements(db, args[1]);
      try {
        const first = await iterator.next();
        if (first.done) throw new Error('Empty SQLite statement');
        const stmt = first.value;
        const id = ++nextStatement;
        statements.set(id, {iterator, stmt, db});
        return {id, count: sqlite.bind_parameter_count(stmt), columns: sqlite.column_names(stmt)};
      } catch (error) { await iterator.return(); throw error; }
    }
    case 'finalize': {
      const state = statements.get(args[1]);
      if (state) { await state.iterator.return(); statements.delete(args[1]); }
      return null;
    }
    case 'query': {
      const state = statements.get(args[1]);
      if (!state || state.db !== db) throw new Error('SQLite statement is closed');
      await sqlite.reset(state.stmt);
      sqlite.clear_bindings(state.stmt);
      bind(sqlite, state.stmt, args[2]);
      return null;
    }
    case 'next': {
      const state = statements.get(args[1]);
      if (!state || state.db !== db) throw new Error('SQLite statement is closed');
      return await sqlite.step(state.stmt) === SQLite.SQLITE_ROW ? row(sqlite, state.stmt) : null;
    }
    case 'reset': {
      const state = statements.get(args[1]);
      if (state) await sqlite.reset(state.stmt);
      return null;
    }
    default: throw new Error('Unknown SQLite operation: ' + op);
  }
}
