// Package sqliteweb provides a browser-only database/sql driver for SQLite.
// It bridges to wa-sqlite in a dedicated worker, with page-level IndexedDB
// persistence through the upstream IDBBatchAtomicVFS. Native apps do not import
// the driver and continue to use their existing SQLite implementation.
package sqliteweb
